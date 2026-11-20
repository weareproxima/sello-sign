import { describe, expect, it, vi } from 'vitest'
import { ApiError, createApi, errorMessage, expiryOf, tokenCache } from './api'

function jwt(payload: Record<string, unknown>) {
	const part = (o: unknown) =>
		Buffer.from(JSON.stringify(o)).toString('base64url')
	return `${part({ alg: 'EdDSA' })}.${part(payload)}.sig`
}

function fakeFetch(
	status: number,
	body: unknown,
	contentType = 'application/json'
) {
	return vi.fn(
		async (_url: string | URL | Request, _init?: RequestInit) =>
			new Response(body === undefined ? null : JSON.stringify(body), {
				status,
				headers: { 'content-type': contentType },
			})
	)
}

describe('createApi', () => {
	it('sends the token and asks the panel route', async () => {
		const fetch = fakeFetch(200, { user: { id: 'u' }, workspaces: [] })
		const api = createApi({ base: '/api/sign', fetch, token: async () => 'T' })
		await api.me()
		const [url, init] = fetch.mock.calls[0]!
		expect(url).toBe('/api/sign/me')
		expect((init!.headers as Record<string, string>).authorization).toBe(
			'Bearer T'
		)
	})

	it('builds paths, queries and JSON bodies', async () => {
		const fetch = fakeFetch(200, { items: [], next_cursor: null })
		const api = createApi({ base: '/api/sign', fetch, token: async () => 'T' })
		await api.signatures('w 1', {
			environment: 'prod',
			cursor: undefined,
			limit: 10,
		})
		expect(fetch.mock.calls[0]![0]).toBe(
			'/api/sign/workspaces/w%201/signatures?environment=prod&limit=10'
		)

		await api.createApiKey('w', {
			name: 'ERP',
			environment: 'test',
			scopes: ['sign'],
		})
		const [, init] = fetch.mock.calls[1]!
		expect(init!.method).toBe('POST')
		expect(JSON.parse(String(init!.body))).toEqual({
			name: 'ERP',
			environment: 'test',
			scopes: ['sign'],
		})
	})

	it('signs the document as is, with its content type', async () => {
		const fetch = fakeFetch(200, {
			signature: 's',
			document_id: 'd',
			document_type: '01',
		})
		const api = createApi({ base: '/api/sign', fetch, token: async () => 'T' })
		await api.sign('w', 'o', 'test', '<a/>', 'application/xml')
		const [url, init] = fetch.mock.calls[0]!
		expect(url).toBe(
			'/api/sign/workspaces/w/organizations/o/signatures?environment=test'
		)
		expect(init!.body).toBe('<a/>')
		expect((init!.headers as Record<string, string>)['content-type']).toBe(
			'application/xml'
		)
	})

	it('unwraps lists that come as a page', async () => {
		const fetch = fakeFetch(200, { items: [{ id: 'k' }], next_cursor: null })
		const api = createApi({ base: '/api/sign', fetch, token: async () => 'T' })
		expect(await api.apiKeys('w')).toEqual([{ id: 'k' }])
	})

	it('answers nothing for a 204', async () => {
		const fetch = vi.fn(async () => new Response(null, { status: 204 }))
		const api = createApi({ base: '/api/sign', fetch, token: async () => 'T' })
		await expect(api.revokeApiKey('w', 'k')).resolves.toBeUndefined()
	})

	it('throws the problem of a failed call', async () => {
		const fetch = fakeFetch(
			409,
			{ type: 'delegation-locked', title: 'Locked', status: 409, detail: 'x' },
			'application/problem+json'
		)
		const api = createApi({ base: '/api/sign', fetch, token: async () => 'T' })
		const error = await api
			.deleteDelegation('w', 'o', 'd')
			.catch((e: unknown) => e)
		expect(error).toBeInstanceOf(ApiError)
		expect((error as ApiError).status).toBe(409)
		expect((error as ApiError).type).toBe('delegation-locked')
	})
})

describe('errorMessage', () => {
	it('words the problems it knows', () => {
		const error = new ApiError(409, {
			type: 'delegation-locked',
			title: '',
			status: 409,
		})
		expect(errorMessage(error, 'quitar la delegación')).toBe(
			'Esta delegación es de la plataforma de Sello y solo ella puede quitarla.'
		)
	})

	it('falls back to the mold', () => {
		const error = new ApiError(500, {
			type: 'internal',
			title: '',
			status: 500,
		})
		expect(errorMessage(error, 'crear la clave')).toBe(
			'No se pudo crear la clave. Vuelva a intentarlo.'
		)
		expect(errorMessage(new Error('x'), 'firmar')).toBe(
			'No se pudo firmar. Vuelva a intentarlo.'
		)
	})

	it('says when there is no network', () => {
		expect(errorMessage(new TypeError('Failed to fetch'), 'firmar')).toBe(
			'Revise su conexión y vuelva a intentarlo.'
		)
	})
})

describe('tokenCache', () => {
	it('keeps a token until a minute before it expires', async () => {
		let now = 1_000_000_000_000
		const exp = Math.floor(now / 1000) + 15 * 60
		const load = vi.fn(async () => jwt({ exp }))
		const cache = tokenCache(load, () => now)
		await cache.get()
		await cache.get()
		expect(load).toHaveBeenCalledTimes(1)
		now += 14 * 60_000 + 1
		await cache.get()
		expect(load).toHaveBeenCalledTimes(2)
	})

	it('asks once for concurrent callers', async () => {
		const load = vi.fn(async () => jwt({ exp: 9_999_999_999 }))
		const cache = tokenCache(load)
		await Promise.all([cache.get(), cache.get(), cache.get()])
		expect(load).toHaveBeenCalledTimes(1)
	})

	it('reads exp, or nothing', () => {
		expect(expiryOf(jwt({ exp: 10 }))).toBe(10_000)
		expect(expiryOf('nope')).toBeNull()
	})
})
