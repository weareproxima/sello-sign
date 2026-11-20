import { createServerFn } from '@tanstack/react-start'
import { getRequestHeaders } from '@tanstack/react-start/server'
import {
	accountDeletable,
	deleteAccountFlow,
	parseDeleteAccountInput,
	type DeleteAccountResult,
} from '#/lib/account-deletion'
import { emailChangeNoticeEmail } from '#/lib/account-email'
import {
	EMAIL_CHANGED_PATH,
	emailChangeAllowed,
	emailChangeFlow,
	parseEmailChangeInput,
	type EmailChangeResult,
} from '#/lib/account-email-change'
import { createApi } from '#/lib/api'
import { getAuth, getDatabase } from '#/lib/auth'
import { readConfig } from '#/lib/config'
import { sendEmail } from '#/lib/email'
import { serverEnv } from '#/lib/env'
import { postgresPasswordAttempts } from '#/lib/password-attempts'

/**
 * Deletes the signed-in person's account (self-hosted; `lib/account-deletion.ts`):
 * the API first, as the person, then better-auth. better-auth's own
 * `/delete-user` stays off, so nothing deletes an account without the API
 * hearing of it first.
 */
export const deleteAccount = createServerFn({ method: 'POST' })
	.validator(parseDeleteAccountInput)
	.handler(async ({ data }): Promise<DeleteAccountResult> => {
		const headers = new Headers(getRequestHeaders() as unknown as HeadersInit)
		const auth = getAuth()
		const session = await auth.api.getSession({ headers })
		if (!session) return { ok: false, reason: 'unauthorized' }
		const config = readConfig(serverEnv())
		const context = await auth.$context
		const credential = await context.internalAdapter.findCredentialAccount(
			session.user.id
		)
		const hash = credential?.password ?? null
		const { token } = await auth.api.getToken({ headers })
		const result = await deleteAccountFlow({
			deletable: accountDeletable(config),
			hasPassword: hash !== null,
			password: data.password,
			verifyPassword: (password) =>
				context.password.verify({ hash: hash!, password }),
			attempts: postgresPasswordAttempts(getDatabase(), session.user.id),
			sessionStartedAt: new Date(session.session.createdAt).getTime(),
			now: Date.now(),
			api: createApi({
				base: config.apiUrl,
				fetch: (input, init) => fetch(input, init),
				token: async () => token,
			}),
			deleteUser: async () => {
				await auth.api.signOut({ headers })
				await context.internalAdapter.deleteUser(session.user.id)
			},
		})
		if (result.ok)
			console.info(
				JSON.stringify({ msg: 'Account deleted', userId: session.user.id })
			)
		else if (result.reason === 'too-many-attempts')
			console.warn(
				JSON.stringify({
					msg: 'Password attempts exhausted',
					action: 'delete-account',
					userId: session.user.id,
				})
			)
		return result
	})

/**
 * Starts changing the signed-in person's e-mail (self-hosted;
 * `lib/account-email-change.ts`): with their current password, better-auth
 * e-mails a link to the new address, and the current one is told. The e-mail
 * changes when the link is opened. better-auth's own `/change-email` stays
 * off over HTTP (`lib/auth-options.ts`), so nothing changes it without the
 * password.
 */
export const requestEmailChange = createServerFn({ method: 'POST' })
	.validator(parseEmailChangeInput)
	.handler(async ({ data }): Promise<EmailChangeResult> => {
		const headers = new Headers(getRequestHeaders() as unknown as HeadersInit)
		const auth = getAuth()
		const session = await auth.api.getSession({ headers })
		if (!session) return { ok: false, reason: 'unauthorized' }
		const config = readConfig(serverEnv())
		const context = await auth.$context
		const accounts = await context.internalAdapter.findAccounts(session.user.id)
		const hash =
			accounts.find((a) => a.providerId === 'credential')?.password ?? null
		const result = await emailChangeFlow({
			allowed: emailChangeAllowed(config),
			currentEmail: session.user.email,
			newEmail: data.email,
			password: data.password,
			linkedToProvider:
				config.oidc !== null &&
				accounts.some((a) => a.providerId === config.oidc?.providerId),
			hasPassword: hash !== null,
			verifyPassword: (password) =>
				context.password.verify({ hash: hash!, password }),
			attempts: postgresPasswordAttempts(getDatabase(), session.user.id),
			requestChange: async (email) => {
				try {
					await auth.api.changeEmail({
						body: { newEmail: email, callbackURL: EMAIL_CHANGED_PATH },
						headers,
					})
				} catch (error) {
					console.warn(
						JSON.stringify({
							msg: 'Could not request an e-mail change',
							userId: session.user.id,
							error: String(error),
						})
					)
					throw error
				}
			},
			notifyCurrent: async (email) => {
				try {
					await sendEmail({
						to: session.user.email,
						...emailChangeNoticeEmail(email, `${config.panelUrl}/account`),
					})
				} catch (error) {
					console.warn(
						JSON.stringify({
							msg: 'Could not send the e-mail change notice',
							userId: session.user.id,
							error: String(error),
						})
					)
					throw error
				}
			},
		})
		if (result.ok)
			console.info(
				JSON.stringify({
					msg: 'E-mail change requested',
					userId: session.user.id,
				})
			)
		else if (result.reason === 'too-many-attempts')
			console.warn(
				JSON.stringify({
					msg: 'Password attempts exhausted',
					action: 'change-email',
					userId: session.user.id,
				})
			)
		return result
	})
