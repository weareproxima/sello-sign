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
	parseResendInput,
	resendAndSend,
	type InviteResult,
} from '#/lib/invitations'

/** The signed-in person and an API client with their token; null without a
 * session. */
async function asPerson() {
	const headers = new Headers(getRequestHeaders() as unknown as HeadersInit)
	const auth = getAuth()
	const session = await auth.api.getSession({ headers })
	if (!session) return null
	const { token } = await auth.api.getToken({ headers })
	const config = readConfig(serverEnv())
	return {
		session,
		config,
		api: createApi({
			base: config.apiUrl,
			fetch: (input, init) => fetch(input, init),
			token: async () => token,
		}),
	}
}

/**
 * Invites someone to a space and sends them the e-mail, from the panel's
 * server: the token the API answers goes in the link and never through the
 * browser apart from that link. It calls the API as the signed-in person,
 * with the same token the browser would get.
 */
export const inviteMember = createServerFn({ method: 'POST' })
	.validator(parseInviteInput)
	.handler(async ({ data }): Promise<InviteResult> => {
		const person = await asPerson()
		if (!person) return { ok: false, status: 401, problem: null }
		return inviteAndSend({
			api: person.api,
			send: (message) => sendEmail(message),
			panelUrl: person.config.panelUrl,
			inviterName: person.session.user.name || person.session.user.email,
			...data,
		})
	})

/**
 * Sends an invitation again, from the panel's server, as `inviteMember`: the
 * API renews it (a new token, 7 more days) and the e-mail carries the new
 * link.
 */
export const resendInvitation = createServerFn({ method: 'POST' })
	.validator(parseResendInput)
	.handler(async ({ data }): Promise<InviteResult> => {
		const person = await asPerson()
		if (!person) return { ok: false, status: 401, problem: null }
		return resendAndSend({
			api: person.api,
			send: (message) => sendEmail(message),
			panelUrl: person.config.panelUrl,
			inviterName: person.session.user.name || person.session.user.email,
			...data,
		})
	})
