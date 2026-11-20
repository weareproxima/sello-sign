/**
 * Deleting one's account in a self-hosted install. In Sello's cloud
 * (`CLOUD`) or where an external provider is the only way in (`OIDC_ONLY`)
 * it isn't offered: the cloud deletes it on request by e-mail, as its
 * privacy policy says, and a provider would bring the account back at the
 * next sign-in.
 *
 * The API goes first (`DELETE /me`): the person leaves their spaces, the
 * spaces they were the only member of are deleted, and it refuses, changing
 * nothing, while they're the only owner of a space with other members
 * (`account-has-workspaces`). Only then does better-auth delete the account,
 * its sessions and its sign-in methods. Pure over its dependencies, tested in
 * `account-deletion.test.ts`; the panel's server runs it
 * (`server/account.ts`).
 */

import { ApiError, type Api, type Problem } from './api'
import type { PanelConfig } from './config'

/** Whether this install lets a person delete their own account. */
export function accountDeletable(config: Pick<PanelConfig, 'cloud' | 'oidc'>) {
	return !config.cloud && config.oidc?.only !== true
}

/** How recent a session must be to delete an account that has no password
 * to ask for (one that only signs in with the provider). */
export const FRESH_SESSION_MS = 10 * 60_000

export type DeleteAccountResult =
	| { ok: true }
	| {
			ok: false
			reason:
				| 'unauthorized'
				| 'disabled'
				| 'invalid-password'
				| 'session-expired'
				| 'blocked'
				| 'api'
			status?: number
			problem?: Problem | null
	  }

export async function deleteAccountFlow(deps: {
	deletable: boolean
	/** Whether the account has a password (a credential account). */
	hasPassword: boolean
	password: string | null
	verifyPassword: (password: string) => Promise<boolean>
	/** When its session started, and now, in milliseconds. */
	sessionStartedAt: number
	now: number
	api: Pick<Api, 'deleteMe'>
	/** Signs out and deletes the account in better-auth. */
	deleteUser: () => Promise<void>
}): Promise<DeleteAccountResult> {
	if (!deps.deletable) return { ok: false, reason: 'disabled' }
	if (deps.hasPassword) {
		if (!deps.password || !(await deps.verifyPassword(deps.password)))
			return { ok: false, reason: 'invalid-password' }
	} else if (deps.now - deps.sessionStartedAt > FRESH_SESSION_MS) {
		return { ok: false, reason: 'session-expired' }
	}
	try {
		await deps.api.deleteMe()
	} catch (error) {
		if (!(error instanceof ApiError)) throw error
		return {
			ok: false,
			reason: error.type === 'account-has-workspaces' ? 'blocked' : 'api',
			status: error.status,
			problem: error.problem,
		}
	}
	await deps.deleteUser()
	return { ok: true }
}

/** Parses the page's input to `deleteAccount`; throws on anything else. */
export function parseDeleteAccountInput(data: unknown): {
	password: string | null
} {
	const d = (data ?? {}) as Record<string, unknown>
	if (
		d.password !== null &&
		d.password !== undefined &&
		typeof d.password !== 'string'
	)
		throw new Error('invalid input')
	return { password: typeof d.password === 'string' ? d.password : null }
}

/** The sentence for a failed deletion. */
export function deleteAccountMessage(
	result: Extract<DeleteAccountResult, { ok: false }>
): string {
	switch (result.reason) {
		case 'invalid-password':
			return 'La contraseña no es correcta.'
		case 'session-expired':
			return 'Por seguridad, vuelva a iniciar sesión y vuelva a intentarlo.'
		case 'blocked':
			return 'Antes de borrar la cuenta, resuelva los espacios que lo impiden.'
		case 'unauthorized':
			return 'Su sesión terminó. Vuelva a iniciar sesión.'
		case 'disabled':
			return 'Esta instalación no permite borrar la cuenta desde el panel.'
		default:
			return 'No se pudo borrar la cuenta. Vuelva a intentarlo.'
	}
}
