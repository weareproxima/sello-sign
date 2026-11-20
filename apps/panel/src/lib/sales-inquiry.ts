/**
 * «Hablar con ventas» of the cloud. Its dialog is on Sello's site
 * (`sello.tax/sign`, `scripts/ventas.ts` of Sello's landing) and posts here,
 * from that origin, the same JSON it posted to ../sign-old's
 * `/sales-inquiry`, which answered `{ ok: true, id }` or `{ message }` with
 * 400. This keeps those shapes. The old Sign also stored each inquiry in its
 * database; this panel has no table for it, so the e-mail to sales is the
 * record, and if it can't go, the inquiry is written to the log in full.
 *
 * Pure functions over the request and the configuration, tested in
 * `sales-inquiry.test.ts`; the route is `routes/sales-inquiry.ts`.
 */

import { deliverEmail, escapeHtml } from './email'

export const TIMELINES = ['asap', '1m', '3m', 'exploring'] as const
export type Timeline = (typeof TIMELINES)[number]

export type SalesInquiry = {
	company: string
	contactName: string
	email: string
	country: string | null
	expectedIssuers: string | null
	expectedDocuments: string | null
	timeline: Timeline
	message: string | null
}

/** Who sent it, when they were signed in to the panel. */
export type Sender = { id: string; name: string; email: string }

function parseTimeline(value: unknown): Timeline {
	return (TIMELINES as readonly unknown[]).includes(value)
		? (value as Timeline)
		: 'exploring'
}

function clean(value: unknown, max = 500): string | null {
	if (typeof value !== 'string') return null
	const trimmed = value.trim()
	return trimmed ? trimmed.slice(0, max) : null
}

/** The inquiry in a request's body, or `null` without the required fields. */
export function parseInquiry(body: unknown): SalesInquiry | null {
	if (!body || typeof body !== 'object' || Array.isArray(body)) return null
	const b = body as Record<string, unknown>
	const company = clean(b.company, 200)
	const contactName = clean(b.contactName, 200)
	const email = clean(b.email, 320)
	if (!company || !contactName || !email || !email.includes('@')) return null
	return {
		company,
		contactName,
		email,
		country: clean(b.country, 10),
		expectedIssuers: clean(b.expectedIssuers, 20),
		expectedDocuments: clean(b.expectedDocuments, 20),
		timeline: parseTimeline(b.timeline),
		message: clean(b.message, 4000),
	}
}

/**
 * CORS for Sello's site only, with credentials: it's the same site as the
 * panel (`sello.tax`), so a visitor signed in here is named in the e-mail.
 * JSON isn't a form body, so the preflight is what keeps other sites out.
 */
export function corsHeaders(
	request: Request,
	siteOrigin: string
): Record<string, string> {
	const headers: Record<string, string> = { Vary: 'Origin' }
	if (request.headers.get('origin') === siteOrigin) {
		headers['Access-Control-Allow-Origin'] = siteOrigin
		headers['Access-Control-Allow-Credentials'] = 'true'
	}
	return headers
}

export function preflight(request: Request, siteOrigin: string): Response {
	const headers = corsHeaders(request, siteOrigin)
	if (headers['Access-Control-Allow-Origin']) {
		headers['Access-Control-Allow-Methods'] = 'POST, OPTIONS'
		headers['Access-Control-Allow-Headers'] = 'Content-Type'
		headers['Access-Control-Max-Age'] = '600'
	}
	return new Response(null, { status: 204, headers })
}

const TIMELINE_LABELS: Record<Timeline, string> = {
	asap: 'Lo antes posible',
	'1m': 'En 1 mes',
	'3m': 'En 3 meses',
	exploring: 'Explorando',
}

/** The e-mail to sales: subject, plain text and HTML, as ../sign-old wrote it. */
export function inquiryEmail(
	inquiry: SalesInquiry,
	sender: Sender | null
): { subject: string; text: string; html: string } {
	const sections: { title: string; rows: [string, string][] }[] = [
		{
			title: 'Contacto',
			rows: [
				['Empresa', inquiry.company],
				['Nombre', inquiry.contactName],
				['Email', inquiry.email],
				...(inquiry.country
					? [['País', inquiry.country] as [string, string]]
					: []),
				['Timeline', TIMELINE_LABELS[inquiry.timeline]],
			],
		},
	]
	const volume: [string, string][] = []
	if (inquiry.expectedIssuers)
		volume.push(['Emisores', inquiry.expectedIssuers])
	if (inquiry.expectedDocuments)
		volume.push(['Firmas/mes', inquiry.expectedDocuments])
	if (volume.length) sections.push({ title: 'Volumen esperado', rows: volume })
	if (sender)
		sections.push({
			title: 'Cuenta en Sello Sign (auto)',
			rows: [['Usuario', `${sender.name} <${sender.email}> · ${sender.id}`]],
		})

	const text = [
		`${inquiry.company} quiere hablar con ventas`,
		...sections.map(
			(s) => `\n${s.title}\n${s.rows.map(([k, v]) => `${k}: ${v}`).join('\n')}`
		),
		...(inquiry.message ? [`\nMensaje\n${inquiry.message}`] : []),
		`\nResponder: mailto:${inquiry.email}`,
	].join('\n')

	const html = [
		`<h1>${escapeHtml(inquiry.company)} quiere hablar con ventas</h1>`,
		...sections.map(
			(s) =>
				`<h2>${escapeHtml(s.title)}</h2><table>${s.rows
					.map(
						([k, v]) =>
							`<tr><td><strong>${escapeHtml(k)}</strong></td><td>${escapeHtml(v)}</td></tr>`
					)
					.join('')}</table>`
		),
		...(inquiry.message
			? [`<blockquote>${escapeHtml(inquiry.message)}</blockquote>`]
			: []),
		`<p><a href="mailto:${escapeHtml(inquiry.email)}">Responder</a></p>`,
	].join('\n')

	return {
		subject: `[Sales] ${inquiry.company} — ${inquiry.timeline}`,
		text,
		html,
	}
}

/**
 * Sends the inquiry to sales (`lib/email.ts`: Resend, or the console without
 * `RESEND_API_KEY`), with the visitor as `reply_to`. Throws if Resend
 * refuses it.
 */
export async function sendInquiry(
	inquiry: SalesInquiry,
	sender: Sender | null,
	email: { resendKey: string | null; sales: string; from: string },
	fetcher: typeof fetch = fetch
): Promise<void> {
	const message = inquiryEmail(inquiry, sender)
	await deliverEmail(
		{ to: email.sales, replyTo: inquiry.email, ...message },
		email,
		fetcher
	)
}

/**
 * A limit per client address: `max` inquiries in `windowMs`. In memory, so on
 * Workers it holds per isolate only: it stops a loop from one address, not a
 * determined sender (../sign-old had none).
 */
export class InquiryLimiter {
	private seen = new Map<string, number[]>()
	constructor(
		private max = 5,
		private windowMs = 10 * 60 * 1000
	) {}

	allow(key: string, now = Date.now()): boolean {
		const recent = (this.seen.get(key) ?? []).filter(
			(t) => now - t < this.windowMs
		)
		if (recent.length >= this.max) {
			this.seen.set(key, recent)
			return false
		}
		recent.push(now)
		this.seen.set(key, recent)
		if (this.seen.size > 10_000) this.seen.clear()
		return true
	}
}

const limiter = new InquiryLimiter()

/** The client's address: Cloudflare's header, or the first forwarded one. */
export function clientAddress(request: Request): string {
	return (
		request.headers.get('cf-connecting-ip') ??
		request.headers.get('x-forwarded-for')?.split(',')[0]?.trim() ??
		'unknown'
	)
}

/**
 * The whole exchange, apart from the session: `sender` resolves who is signed
 * in, if anyone. Never fails for the e-mail: like ../sign-old, the visitor
 * gets `ok` and the failure goes to the log, with the inquiry in it.
 */
export async function handleInquiry(
	request: Request,
	deps: {
		siteOrigin: string
		email: { resendKey: string | null; sales: string; from: string }
		sender: () => Promise<Sender | null>
		fetcher?: typeof fetch
		newId?: () => string
		limiter?: InquiryLimiter
	}
): Promise<Response> {
	const cors = corsHeaders(request, deps.siteOrigin)
	const json = (body: unknown, status = 200) =>
		Response.json(body, { status, headers: cors })
	if (!(deps.limiter ?? limiter).allow(clientAddress(request))) {
		return json({ message: 'Too many requests' }, 429)
	}
	let body: unknown
	try {
		body = await request.json()
	} catch {
		return json({ message: 'Invalid JSON body' }, 400)
	}
	const inquiry = parseInquiry(body)
	if (!inquiry) return json({ message: 'Missing required fields' }, 400)

	const id = (deps.newId ?? (() => crypto.randomUUID()))()
	let sender: Sender | null = null
	try {
		sender = await deps.sender()
	} catch {
		sender = null
	}
	try {
		await sendInquiry(inquiry, sender, deps.email, deps.fetcher)
	} catch (err) {
		console.error(
			JSON.stringify({
				msg: 'Failed to send sales inquiry email',
				salesInquiryId: id,
				error: err instanceof Error ? err.message : String(err),
				inquiry,
				sender,
			})
		)
	}
	console.info(
		JSON.stringify({
			msg: 'Sales inquiry received',
			salesInquiryId: id,
			userId: sender?.id ?? null,
			timeline: inquiry.timeline,
		})
	)
	return json({ ok: true, id })
}
