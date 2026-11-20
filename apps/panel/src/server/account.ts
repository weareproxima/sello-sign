import { createServerFn } from '@tanstack/react-start'
import { getRequestHeaders } from '@tanstack/react-start/server'
import {
	accountDeletable,
	deleteAccountFlow,
	parseDeleteAccountInput,
	type DeleteAccountResult,
} from '#/lib/account-deletion'
import { createApi } from '#/lib/api'
import { getAuth } from '#/lib/auth'
import { readConfig } from '#/lib/config'
import { serverEnv } from '#/lib/env'

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
		return result
	})
