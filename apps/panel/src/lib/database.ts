import { AsyncLocalStorage } from 'node:async_hooks'
import pg from 'pg'
import type { PoolClient, QueryResult, QueryResultRow } from 'pg'

/**
 * The panel's Postgres, with `pg` (over TCP; on Workers, through
 * `cloudflare:sockets`, which `pg` picks itself under nodejs_compat).
 *
 * In Node there's one pool for the process. On Workers a socket belongs to
 * the request that opened it: another request can't use it ("Cannot perform
 * I/O on behalf of a different request"), so each request gets its own pool,
 * opened on its first query and closed when its response ends
 * (`withRequestDatabase`, from `src/server.ts`). Against Neon that request
 * pool connects to the pooled endpoint (`-pooler`), where opening a
 * connection is cheap.
 *
 * It has what better-auth's Kysely adapter takes for a pg pool (`connect`)
 * and what the panel's own queries use (`query`), so the adapter is built
 * once and each query lands on the right pool.
 */
export class Database {
	readonly #url: string
	readonly #perRequest: boolean
	#shared: pg.Pool | null = null

	constructor(url: string, options: { perRequest: boolean }) {
		this.#url = url
		this.#perRequest = options.perRequest
	}

	#pool(): pg.Pool {
		if (!this.#perRequest) return (this.#shared ??= newPool(this.#url))
		const scope = requestScope.getStore()
		if (!scope) {
			throw new Error(
				'database: used outside a request (withRequestDatabase in src/server.ts)'
			)
		}
		return (scope.pool ??= newPool(this.#url))
	}

	/** What Kysely uses to cancel a running query from another connection. */
	readonly Client = pg.Client
	get options(): object {
		return { connectionString: this.#url }
	}

	connect(): Promise<PoolClient> {
		return this.#pool().connect()
	}

	query<R extends QueryResultRow = QueryResultRow>(
		text: string,
		values?: unknown[]
	): Promise<QueryResult<R>> {
		return this.#pool().query<R>(text, values)
	}

	/** Closes the process's pool (scripts); a request's closes on its own. */
	async end(): Promise<void> {
		await this.#shared?.end()
		this.#shared = null
	}
}

function newPool(url: string): pg.Pool {
	const pool = new pg.Pool({ connectionString: url })
	// An idle connection that the server closes (a restart, Neon suspending
	// the compute) emits `error` on the pool; unhandled, it would end the
	// process. The next query opens another.
	pool.on('error', (error) => {
		console.warn(`[database] idle connection closed: ${error.message}`)
	})
	return pool
}

type RequestScope = { pool?: pg.Pool }

const requestScope = new AsyncLocalStorage<RequestScope>()

/**
 * Runs a request's handler with a pool of its own and closes that pool when
 * the response ends: right away if it has no body, or when its body has been
 * sent (a streamed page may still query while it streams). If the client
 * goes away first, the runtime drops the request's sockets with it.
 */
export async function withRequestDatabase(
	handle: () => Promise<Response>
): Promise<Response> {
	const scope: RequestScope = {}
	const close = async () => {
		const pool = scope.pool
		scope.pool = undefined
		await pool?.end().catch(() => {})
	}
	let response: Response
	try {
		response = await requestScope.run(scope, handle)
	} catch (error) {
		await close()
		throw error
	}
	if (!response.body) {
		await close()
		return response
	}
	const body = response.body.pipeThrough(
		new TransformStream<Uint8Array, Uint8Array>({ flush: close })
	)
	return new Response(body, response)
}
