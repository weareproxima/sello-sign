import { createServerFn } from '@tanstack/react-start'
import { getRequestHeaders } from '@tanstack/react-start/server'
import { createApi } from '#/lib/api'
import { getAuth } from '#/lib/auth'
import { readConfig } from '#/lib/config'
import { sendEmail } from '#/lib/email'
import { serverEnv } from '#/lib/env'
import {
	inviteAndSend,
	parseInviteInput,
	type InviteResult,
} from '#/lib/invitations'

/**
 * Invites someone to a space and sends them the e-mail, from the panel's
 * server: the token the API answers goes in the link and never through the
 * browser apart from that link. It calls the API as the signed-in person,
 * with the same token the browser would get.
 */
export const inviteMember = createServerFn({ method: 'POST' })
	.validator(parseInviteInput)
	.handler(async ({ data }): Promise<InviteResult> => {
		const headers = new Headers(getRequestHeaders() as unknown as HeadersInit)
		const auth = getAuth()
		const session = await auth.api.getSession({ headers })
		if (!session) return { ok: false, status: 401, problem: null }
		const { token } = await auth.api.getToken({ headers })
		const config = readConfig(serverEnv())
		return inviteAndSend({
			api: createApi({
				base: config.apiUrl,
				fetch: (input, init) => fetch(input, init),
				token: async () => token,
			}),
			send: (message) => sendEmail(message),
			panelUrl: config.panelUrl,
			inviterName: session.user.name || session.user.email,
			...data,
		})
	})
