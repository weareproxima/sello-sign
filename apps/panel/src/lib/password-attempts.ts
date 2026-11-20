/**
 * The limit on guessing a password where the panel's server checks it
 * itself: changing the e-mail (`lib/account-email-change.ts`) and deleting
 * the account (`lib/account-deletion.ts`), which better-auth's own rate
 * limit doesn't cover (it limits its HTTP routes, and these call its API
 * from the server). At most `PASSWORD_ATTEMPTS` failed attempts per account
 * in a window of `PASSWORD_WINDOW_MS` that starts at the first one; then
 * none until the window ends.
 *
 * Kept in the panel's Postgres (`auth.password_attempt`, which
 * `scripts/migrate.ts` creates), not in the process's memory: on
 * Cloudflare Workers each request may land on another isolate, and a
 * self-hosted install may run several. One row per account that failed:
 * its id, how many failures and when the window started. Never the
 * password, nor where the request came from. The row goes when the
 * password is right, with the account, or, once its window is over, at
 * anyone's next check.
 *
 * An attempt is taken before the password is checked, in one statement, so
 * that requests at once can't check more than the limit allows.
 */

/** Failed attempts an account may make in a window. */
export const PASSWORD_ATTEMPTS = 5

/** How long a window lasts, from its first failed attempt. */
export const PASSWORD_WINDOW_MS = 15 * 60_000

/** One account's attempts, as the server holds them. */
export type PasswordAttempts = {
	/** Takes one attempt; if none is left, when the next one will be. */
	take(): Promise<{ ok: true } | { ok: false; retryAt: Date }>
	/** The password was right: the failures are forgotten. */
	clear(): Promise<void>
}

export type PasswordCheck =
	| { ok: true }
	| { ok: false; reason: 'invalid-password' }
	| { ok: false; reason: 'too-many-attempts'; retryAt: string }

/**
 * Checks a password with an account's attempts: an empty one is wrong
 * without taking any; past the limit, it isn't checked.
 */
export async function checkPassword(
	password: string | null,
	verify: (password: string) => Promise<boolean>,
	attempts: PasswordAttempts
): Promise<PasswordCheck> {
	if (!password) return { ok: false, reason: 'invalid-password' }
	const taken = await attempts.take()
	if (!taken.ok)
		return {
			ok: false,
			reason: 'too-many-attempts',
			retryAt: taken.retryAt.toISOString(),
		}
	if (!(await verify(password)))
		return { ok: false, reason: 'invalid-password' }
	await attempts.clear()
	return { ok: true }
}

/** The table, in better-auth's schema (`auth`, or a test's); it goes with
 * its account. */
export function passwordAttemptTable(schema = 'auth'): string {
	return `create table if not exists ${schema}.password_attempt (
	user_id text primary key references ${schema}."user" (id) on delete cascade,
	failures integer not null,
	window_start timestamptz not null
)`
}

/** What runs the queries: the panel's `Database`, or a test's pool. */
export type Queryable = {
	query<R extends Record<string, unknown>>(
		text: string,
		values?: unknown[]
	): Promise<{ rows: R[] }>
}

/**
 * An account's attempts in the panel's Postgres. `table` is
 * `auth.password_attempt`, or a test's.
 */
export function postgresPasswordAttempts(
	db: Queryable,
	userId: string,
	options: {
		now?: () => Date
		table?: string
		limit?: number
		windowMs?: number
	} = {}
): PasswordAttempts {
	const table = options.table ?? 'auth.password_attempt'
	const limit = options.limit ?? PASSWORD_ATTEMPTS
	const windowMs = options.windowMs ?? PASSWORD_WINDOW_MS
	const now = options.now ?? (() => new Date())
	return {
		async take() {
			const at = now()
			// Windows that started before this are over.
			const cutoff = new Date(at.getTime() - windowMs)
			await db.query(
				`delete from ${table} where window_start <= $1 and user_id <> $2`,
				[cutoff, userId]
			)
			// A new window, or one more attempt in the current one while
			// it has any left; nothing comes back when it has none.
			const taken = await db.query(
				`insert into ${table} as a (user_id, failures, window_start) values ($1, 1, $2)
				on conflict (user_id) do update set
					failures = case when a.window_start <= $3 then 1 else a.failures + 1 end,
					window_start = case when a.window_start <= $3 then $2 else a.window_start end
				where a.window_start <= $3 or a.failures < $4
				returning failures`,
				[userId, at, cutoff, limit]
			)
			if (taken.rows.length > 0) return { ok: true }
			const held = await db.query<{ window_start: Date }>(
				`select window_start from ${table} where user_id = $1`,
				[userId]
			)
			const start = held.rows[0]?.window_start ?? at
			return {
				ok: false,
				retryAt: new Date(new Date(start).getTime() + windowMs),
			}
		},
		async clear() {
			await db.query(`delete from ${table} where user_id = $1`, [userId])
		},
	}
}

/** The sentence for an account past the limit, with when to try again in
 * the reader's time («después de las 15:42»). */
export function passwordAttemptsMessage(retryAt: string | undefined): string {
	const at = retryAt ? new Date(retryAt) : null
	if (!at || Number.isNaN(at.getTime()))
		return 'Alcanzó el límite de intentos con la contraseña. Vuelva a intentarlo en 15 minutos.'
	const time = at.toLocaleTimeString('es-419', {
		hour: '2-digit',
		minute: '2-digit',
	})
	// «a.m.» already ends in a period, which also ends the sentence.
	const end = time.endsWith('.') ? '' : '.'
	return `Alcanzó el límite de intentos con la contraseña. Vuelva a intentarlo después de las ${time}${end}`
}
