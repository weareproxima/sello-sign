/**
 * The panel's e-mail: Resend's HTTP API (no SDK: plain `fetch`, as on
 * Workers), or, without `RESEND_API_KEY`, the server's console. Every e-mail
 * the panel sends goes through here: the sales inquiry
 * (`lib/sales-inquiry.ts`), the invitations (`lib/invitations.ts`) and
 * better-auth's.
 *
 * Server only: it reads the key.
 */

import { readConfig } from './config'
import { serverEnv } from './env'

export type EmailMessage = {
	to: string
	subject: string
	text: string
	html?: string
	/** Where a reply goes, when it isn't `EMAIL_FROM`. */
	replyTo?: string
}

/** Resend's key (`null` → the console) and the sender, `EMAIL_FROM`. */
export type EmailSettings = { resendKey: string | null; from: string }

/**
 * Sends `message` with `settings`. Without a key it writes it to the console
 * and returns. Throws if Resend refuses it: the caller decides whether that
 * fails what it was doing.
 */
export async function deliverEmail(
	message: EmailMessage,
	settings: EmailSettings,
	fetcher: typeof fetch = fetch
): Promise<void> {
	if (!settings.resendKey) {
		console.info(
			`[email] to=${message.to} subject=${message.subject}\n${message.text}`
		)
		return
	}
	const response = await fetcher('https://api.resend.com/emails', {
		method: 'POST',
		headers: {
			authorization: `Bearer ${settings.resendKey}`,
			'content-type': 'application/json',
		},
		body: JSON.stringify({
			from: settings.from,
			to: message.to,
			...(message.replyTo ? { reply_to: message.replyTo } : {}),
			subject: message.subject,
			text: message.text,
			...(message.html ? { html: message.html } : {}),
		}),
	})
	if (!response.ok) {
		throw new Error(`Resend answered ${response.status}`)
	}
}

/** `deliverEmail` with the environment's settings (`RESEND_API_KEY`,
 * `EMAIL_FROM`), read at each call, so it works on Workers. */
export function sendEmail(
	message: EmailMessage,
	fetcher: typeof fetch = fetch
): Promise<void> {
	return deliverEmail(message, readConfig(serverEnv()).email, fetcher)
}

/** Escapes text for an e-mail's HTML. */
export const escapeHtml = (s: string) =>
	s.replace(
		/[&<>"']/g,
		(c) =>
			({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[
				c
			]!
	)
