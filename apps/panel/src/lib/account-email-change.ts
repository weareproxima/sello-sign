/**
 * Changing one's e-mail in a self-hosted install, with better-auth's
 * change-email flow: the panel's server checks the current password, asks
 * better-auth to e-mail a link to the new address (`lib/email.ts`, with the
 * template of `lib/account-email.ts`) and tells the current one. The e-mail
 * changes when the link is opened (better-auth's `/verify-email`), which
 * comes back to «Su cuenta» (`EMAIL_CHANGED_PATH`).
 *
 * Not in Sello's cloud (`CLOUD`) nor with `OIDC_ONLY`, nor for an account
 * that signs in with the install's provider: there the e-mail is the
 * provider's, and better-auth writes it again at each sign-in. In the cloud,
 * «Su cuenta» links to Sello's account center instead.
 *
 * The API keeps no e-mail of its own: it reads the token's, which carries the
 * new one from the next token on, and writes it in its `users` at the next
 * `GET /me` (contracts/README.md › Espacios). Pure over its dependencies,
 * tested in `account-email-change.test.ts`; the panel's server runs it
 * (`server/account.ts`).
 */

import type { PanelConfig } from './config'

/** Whether this install lets a person change their e-mail. */
export function emailChangeAllowed(
	config: Pick<PanelConfig, 'cloud' | 'oidc'>
): boolean {
	return !config.cloud && config.oidc?.only !== true
}

/** Where the link of the e-mail lands; better-auth adds `&error=…` when
 * the link no longer serves. */
export const EMAIL_CHANGED_PATH = '/account?email=changed'

export type EmailChangeResult =
	| { ok: true; email: string }
	| {
			ok: false
			reason:
				| 'unauthorized'
				| 'disabled'
				| 'provider'
				| 'invalid-email'
				| 'same-email'
				| 'invalid-password'
				| 'failed'
	  }

/** An address as better-auth keeps it: trimmed, in lower case. */
export function normalizeEmail(email: string): string {
	return email.trim().toLowerCase()
}

/** Rough on purpose: the link is what proves the address. */
const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

export async function emailChangeFlow(deps: {
	allowed: boolean
	currentEmail: string
	newEmail: string
	password: string
	/** Whether the account signs in with the install's provider, which
	 * writes its e-mail at each sign-in. */
	linkedToProvider: boolean
	/** Whether the account has a password (a credential account). */
	hasPassword: boolean
	verifyPassword: (password: string) => Promise<boolean>
	/** better-auth's change: e-mails the link to the new address. */
	requestChange: (email: string) => Promise<void>
	/** Tells the current address; its failure doesn't stop the change. */
	notifyCurrent: (email: string) => Promise<void>
}): Promise<EmailChangeResult> {
	if (!deps.allowed) return { ok: false, reason: 'disabled' }
	if (deps.linkedToProvider || !deps.hasPassword)
		return { ok: false, reason: 'provider' }
	const email = normalizeEmail(deps.newEmail)
	if (!EMAIL.test(email) || email.length > 254)
		return { ok: false, reason: 'invalid-email' }
	if (email === normalizeEmail(deps.currentEmail))
		return { ok: false, reason: 'same-email' }
	if (!deps.password || !(await deps.verifyPassword(deps.password)))
		return { ok: false, reason: 'invalid-password' }
	try {
		await deps.requestChange(email)
	} catch {
		return { ok: false, reason: 'failed' }
	}
	try {
		await deps.notifyCurrent(email)
	} catch {
		// The link went out; the notice is a courtesy.
	}
	return { ok: true, email }
}

/** Parses the page's input to `requestEmailChange`; throws on anything else. */
export function parseEmailChangeInput(data: unknown): {
	email: string
	password: string
} {
	const d = (data ?? {}) as Record<string, unknown>
	if (typeof d.email !== 'string' || typeof d.password !== 'string')
		throw new Error('invalid input')
	return { email: d.email, password: d.password }
}

/** The sentence for a change that didn't start. */
export function emailChangeMessage(
	result: Extract<EmailChangeResult, { ok: false }>
): string {
	switch (result.reason) {
		case 'invalid-email':
			return 'Escriba un correo electrónico válido.'
		case 'same-email':
			return 'Ese ya es el correo de su cuenta.'
		case 'invalid-password':
			return 'La contraseña no es correcta.'
		case 'provider':
			return 'Su correo viene de su proveedor de inicio de sesión, y se cambia allí.'
		case 'disabled':
			return 'Esta instalación no permite cambiar el correo desde el panel.'
		case 'unauthorized':
			return 'Su sesión terminó. Vuelva a iniciar sesión.'
		default:
			return 'No se pudo enviar el enlace. Vuelva a intentarlo.'
	}
}

/**
 * What the link of the e-mail said, back on «Su cuenta»: `changed` once the
 * e-mail changed, or the sentence for better-auth's `error` (the link
 * expired, isn't valid, or is another account's). `null` when the address
 * carries neither.
 */
export function emailLinkOutcome(search: {
	email?: string
	error?: string
}): { changed: true } | { changed: false; message: string } | null {
	if (search.error) {
		switch (search.error) {
			case 'TOKEN_EXPIRED':
				return {
					changed: false,
					message: 'El enlace para cambiar el correo venció. Pida otro.',
				}
			case 'INVALID_TOKEN':
				return {
					changed: false,
					message: 'El enlace para cambiar el correo no es válido. Pida otro.',
				}
			case 'INVALID_USER':
				return {
					changed: false,
					message:
						'El enlace es de otra cuenta. Cierre la sesión, inicie con esa cuenta y ábralo otra vez.',
				}
			default:
				return {
					changed: false,
					message: 'No se pudo cambiar el correo. Vuelva a intentarlo.',
				}
		}
	}
	return search.email === 'changed' ? { changed: true } : null
}
