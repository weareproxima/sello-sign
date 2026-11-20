import { afterEach, describe, expect, it, vi } from 'vitest'
import {
	InquiryLimiter,
	handleInquiry,
	inquiryEmail,
	parseInquiry,
	preflight,
} from './sales-inquiry'

const SITE = 'https://sello.tax'
const valid = {
	company: '  Acme, S.A. ',
	contactName: 'Ana',
	email: 'ana@acme.example',
	country: 'SV',
	expectedIssuers: '10-50',
	expectedDocuments: '10k',
	timeline: 'asap',
	message: null,
}

function post(body: unknown, origin = SITE, ip = '203.0.113.1') {
	return new Request('https://sign.sello.tax/sales-inquiry', {
		method: 'POST',
		headers: {
			origin,
			'content-type': 'application/json',
			'cf-connecting-ip': ip,
		},
		body: typeof body === 'string' ? body : JSON.stringify(body),
	})
}

const deps = (over: Partial<Parameters<typeof handleInquiry>[1]> = {}) => ({
	siteOrigin: SITE,
	email: { resendKey: null, sales: 'sales@x', from: 'Sign <no@x>' },
	sender: async () => null,
	newId: () => 'id-1',
	limiter: new InquiryLimiter(),
	...over,
})

afterEach(() => vi.restoreAllMocks())

describe('parseInquiry', () => {
	it('takes what the dialog sends, trimmed', () => {
		expect(parseInquiry(valid)).toEqual({
			...valid,
			company: 'Acme, S.A.',
			message: null,
		})
	})

	it('needs a company, a name and an e-mail with @', () => {
		expect(parseInquiry({ ...valid, company: ' ' })).toBeNull()
		expect(parseInquiry({ ...valid, contactName: undefined })).toBeNull()
		expect(parseInquiry({ ...valid, email: 'ana' })).toBeNull()
		expect(parseInquiry(null)).toBeNull()
		expect(parseInquiry([valid])).toBeNull()
	})

	it('falls back to «exploring» and cuts what is too long', () => {
		const parsed = parseInquiry({
			...valid,
			timeline: 'tomorrow',
			country: 'El Salvador y más',
			message: 'x'.repeat(5000),
		})
		expect(parsed?.timeline).toBe('exploring')
		expect(parsed?.country).toHaveLength(10)
		expect(parsed?.message).toHaveLength(4000)
	})
})

describe('CORS', () => {
	it('answers the preflight of Sello’s site, with credentials', () => {
		const res = preflight(
			new Request('https://sign.sello.tax/sales-inquiry', {
				method: 'OPTIONS',
				headers: { origin: SITE },
			}),
			SITE
		)
		expect(res.status).toBe(204)
		expect(res.headers.get('access-control-allow-origin')).toBe(SITE)
		expect(res.headers.get('access-control-allow-credentials')).toBe('true')
		expect(res.headers.get('access-control-allow-methods')).toBe(
			'POST, OPTIONS'
		)
		expect(res.headers.get('access-control-allow-headers')).toBe('Content-Type')
	})

	it('gives nothing to another origin', async () => {
		const res = preflight(
			new Request('https://sign.sello.tax/sales-inquiry', {
				method: 'OPTIONS',
				headers: { origin: 'https://evil.example' },
			}),
			SITE
		)
		expect(res.headers.get('access-control-allow-origin')).toBeNull()
		expect(res.headers.get('vary')).toBe('Origin')
		vi.spyOn(console, 'info').mockImplementation(() => {})
		const posted = await handleInquiry(
			post(valid, 'https://evil.example'),
			deps()
		)
		expect(posted.headers.get('access-control-allow-origin')).toBeNull()
	})
})

describe('handleInquiry', () => {
	it('answers { ok, id } and sends to the console without Resend', async () => {
		const info = vi.spyOn(console, 'info').mockImplementation(() => {})
		const res = await handleInquiry(post(valid), deps())
		expect(res.status).toBe(200)
		expect(await res.json()).toEqual({ ok: true, id: 'id-1' })
		expect(res.headers.get('access-control-allow-origin')).toBe(SITE)
		expect(info.mock.calls[0]?.[0]).toContain('[email] to=sales@x')
	})

	it('answers 400 with a message for a bad body', async () => {
		let res = await handleInquiry(post('{nope'), deps())
		expect(res.status).toBe(400)
		expect(await res.json()).toEqual({ message: 'Invalid JSON body' })
		expect(res.headers.get('access-control-allow-origin')).toBe(SITE)
		res = await handleInquiry(post({ ...valid, email: 'x' }), deps())
		expect(res.status).toBe(400)
		expect(await res.json()).toEqual({ message: 'Missing required fields' })
	})

	it('sends through Resend, with reply-to the visitor and who is signed in', async () => {
		vi.spyOn(console, 'info').mockImplementation(() => {})
		const fetcher = vi.fn(async () => new Response('{}', { status: 200 }))
		const res = await handleInquiry(
			post(valid),
			deps({
				email: { resendKey: 're_x', sales: 'sales@x', from: 'Sign <no@x>' },
				sender: async () => ({ id: 'u1', name: 'Ana', email: 'ana@x' }),
				fetcher: fetcher as unknown as typeof fetch,
			})
		)
		expect(res.status).toBe(200)
		const [url, init] = fetcher.mock.calls[0] as unknown as [
			string,
			RequestInit,
		]
		expect(url).toBe('https://api.resend.com/emails')
		expect((init.headers as Record<string, string>).authorization).toBe(
			'Bearer re_x'
		)
		const sent = JSON.parse(init.body as string)
		expect(sent).toMatchObject({
			from: 'Sign <no@x>',
			to: 'sales@x',
			reply_to: 'ana@acme.example',
			subject: '[Sales] Acme, S.A. — asap',
		})
		expect(sent.text).toContain('ana@x')
	})

	it('still answers ok when Resend fails, and logs the inquiry', async () => {
		vi.spyOn(console, 'info').mockImplementation(() => {})
		const error = vi.spyOn(console, 'error').mockImplementation(() => {})
		const res = await handleInquiry(
			post(valid),
			deps({
				email: { resendKey: 're_x', sales: 's@x', from: 'f@x' },
				fetcher: (async () =>
					new Response('no', { status: 422 })) as unknown as typeof fetch,
			})
		)
		expect(res.status).toBe(200)
		expect(error.mock.calls[0]?.[0]).toContain('ana@acme.example')
	})

	it('limits each address', async () => {
		vi.spyOn(console, 'info').mockImplementation(() => {})
		const limiter = new InquiryLimiter(2, 60_000)
		const d = deps({ limiter })
		expect((await handleInquiry(post(valid), d)).status).toBe(200)
		expect((await handleInquiry(post(valid), d)).status).toBe(200)
		const third = await handleInquiry(post(valid), d)
		expect(third.status).toBe(429)
		expect(third.headers.get('access-control-allow-origin')).toBe(SITE)
		expect(
			(await handleInquiry(post(valid, SITE, '203.0.113.2'), d)).status
		).toBe(200)
	})
})

describe('inquiryEmail', () => {
	it('escapes the HTML', () => {
		const parsed = parseInquiry({ ...valid, company: '<b>Acme</b>' })!
		const { html, text } = inquiryEmail(parsed, null)
		expect(html).toContain('&lt;b&gt;Acme&lt;/b&gt;')
		expect(text).toContain('Timeline: Lo antes posible')
		expect(text).toContain('Firmas/mes: 10k')
	})
})
