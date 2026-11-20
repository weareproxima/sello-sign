import { createFileRoute } from '@tanstack/react-router'
import { readConfig } from '#/lib/config'
import { serverEnv } from '#/lib/env'

/**
 * `/api/sign/*` → the API (`API_URL`), unchanged: method, path, query, body
 * and the headers the contract uses. The browser gets its token from
 * `/api/auth/token` and sends it in `Authorization`; this route only spares
 * the API from answering CORS for the panel. It adds no credentials.
 */
const FORWARDED = [
	'authorization',
	'content-type',
	'accept',
	'x-organization-id',
	'idempotency-key',
]

async function forward(request: Request): Promise<Response> {
	const { apiUrl } = readConfig(serverEnv())
	const url = new URL(request.url)
	const path = url.pathname.replace(/^\/api\/sign/, '') || '/'
	const headers = new Headers()
	for (const name of FORWARDED) {
		const value = request.headers.get(name)
		if (value) headers.set(name, value)
	}
	const hasBody = request.method !== 'GET' && request.method !== 'HEAD'
	let upstream: Response
	try {
		upstream = await fetch(`${apiUrl}${path}${url.search}`, {
			method: request.method,
			headers,
			body: hasBody ? await request.arrayBuffer() : undefined,
			redirect: 'manual',
		})
	} catch {
		return new Response(
			JSON.stringify({
				type: 'api-unavailable',
				title: 'API unavailable',
				status: 502,
				detail: 'The panel could not reach the API.',
			}),
			{ status: 502, headers: { 'content-type': 'application/problem+json' } }
		)
	}
	const out = new Headers()
	for (const name of [
		'content-type',
		'content-disposition',
		'location',
		'retry-after',
	]) {
		const value = upstream.headers.get(name)
		if (value) out.set(name, value)
	}
	return new Response(upstream.body, { status: upstream.status, headers: out })
}

export const Route = createFileRoute('/api/sign/$')({
	server: {
		handlers: {
			GET: ({ request }) => forward(request),
			POST: ({ request }) => forward(request),
			PATCH: ({ request }) => forward(request),
			PUT: ({ request }) => forward(request),
			DELETE: ({ request }) => forward(request),
		},
	},
})
