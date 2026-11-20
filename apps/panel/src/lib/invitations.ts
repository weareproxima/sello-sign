/**
 * Invitations to a space. The API creates them and answers their token
 * once (`POST /workspaces/{w}/invitations`); the panel's server puts it in
 * the link of the e-mail it sends (`server/invitations.ts`), so the token
 * never needs to be stored here. Pure over the API and the e-mail, tested in
 * `invitations.test.ts`.
 */

import {
	ApiError,
	type Api,
	type CreatedInvitation,
	type Invitation,
	type Problem,
	type Role,
} from './api'
import { escapeHtml, type EmailMessage } from './email'
import { ROLE_LABELS, formatDate } from './format'

/** The page that accepts it: `/invitation?token=…` of the panel. */
export function invitationLink(panelUrl: string, token: string): string {
	return `${panelUrl}/invitation?token=${encodeURIComponent(token)}`
}

/** The e-mail to the invited address, from whoever invited. */
export function invitationEmail(input: {
	to: string
	workspaceName: string
	inviterName: string
	role: Role
	link: string
	expiresAt: string
}): EmailMessage {
	const role = ROLE_LABELS[input.role]
	const expires = formatDate(input.expiresAt)
	const subject = `${input.inviterName} le invitó a ${input.workspaceName} en Sello Sign`
	const text = [
		`${input.inviterName} le invitó a unirse al espacio «${input.workspaceName}» de Sello Sign con el rol de ${role}.`,
		'',
		`Para aceptar la invitación, abra este enlace e inicie sesión o cree una cuenta con este correo (${input.to}):`,
		input.link,
		'',
		`La invitación vence el ${expires}. Si no la esperaba, puede ignorar este correo.`,
	].join('\n')
	const html = [
		`<p>${escapeHtml(input.inviterName)} le invitó a unirse al espacio «<strong>${escapeHtml(input.workspaceName)}</strong>» de Sello Sign con el rol de ${escapeHtml(role)}.</p>`,
		`<p>Para aceptar la invitación, inicie sesión o cree una cuenta con este correo (${escapeHtml(input.to)}).</p>`,
		`<p><a href="${escapeHtml(input.link)}">Aceptar la invitación</a></p>`,
		`<p>La invitación vence el ${escapeHtml(expires)}. Si no la esperaba, puede ignorar este correo.</p>`,
	].join('\n')
	return { to: input.to, subject, text, html }
}

/** What inviting answers to the page: never the token alone, but the link,
 * shown once so it can be passed on if the e-mail doesn't arrive. */
export type InviteResult =
	| { ok: true; invitation: Invitation; link: string; emailed: boolean }
	| { ok: false; status: number; problem: Problem | null }

type SendDeps = {
	api: Pick<Api, 'workspace'>
	send: (message: EmailMessage) => Promise<void>
	panelUrl: string
	inviterName: string
	workspaceId: string
}

/**
 * Calls the API for an invitation with its token (creating it or renewing
 * it) and sends its e-mail. The invitation stands even if the e-mail fails
 * (`emailed: false`): the page offers the link instead.
 */
async function withEmail(
	deps: SendDeps,
	call: () => Promise<CreatedInvitation>
): Promise<InviteResult> {
	let created: CreatedInvitation
	let workspaceName: string
	try {
		created = await call()
		workspaceName = (await deps.api.workspace(deps.workspaceId)).name
	} catch (error) {
		if (error instanceof ApiError)
			return { ok: false, status: error.status, problem: error.problem }
		throw error
	}
	const { token, ...invitation } = created
	const link = invitationLink(deps.panelUrl, token)
	let emailed = true
	try {
		await deps.send(
			invitationEmail({
				to: invitation.email,
				workspaceName,
				inviterName: deps.inviterName,
				role: invitation.role,
				link,
				expiresAt: invitation.expires_at,
			})
		)
	} catch (error) {
		emailed = false
		console.error(
			JSON.stringify({
				msg: 'Failed to send invitation email',
				invitationId: invitation.id,
				error: error instanceof Error ? error.message : String(error),
			})
		)
	}
	return { ok: true, invitation, link, emailed }
}

/** Creates the invitation with the inviter's token and sends its e-mail. */
export async function inviteAndSend(
	deps: SendDeps & {
		api: Pick<Api, 'createInvitation' | 'workspace'>
		email: string
		role: Role
	}
): Promise<InviteResult> {
	return withEmail(deps, () =>
		deps.api.createInvitation(deps.workspaceId, {
			email: deps.email,
			role: deps.role,
		})
	)
}

/**
 * Sends an invitation again: the API can't give its old token back (it keeps
 * only its hash), so it renews it (a new token, 7 more days, the old link no
 * longer works) and the e-mail goes with the new link.
 */
export async function resendAndSend(
	deps: SendDeps & {
		api: Pick<Api, 'renewInvitation' | 'workspace'>
		invitationId: string
	}
): Promise<InviteResult> {
	return withEmail(deps, () =>
		deps.api.renewInvitation(deps.workspaceId, deps.invitationId)
	)
}

/** Parses the page's input to `resendInvitation`; throws on anything else. */
export function parseResendInput(data: unknown): {
	workspaceId: string
	invitationId: string
} {
	const d = (data ?? {}) as Record<string, unknown>
	if (typeof d.workspaceId !== 'string' || typeof d.invitationId !== 'string')
		throw new Error('invalid input')
	return { workspaceId: d.workspaceId, invitationId: d.invitationId }
}

/** Parses the page's input to `inviteMember`; throws on anything else. */
export function parseInviteInput(data: unknown): {
	workspaceId: string
	email: string
	role: Role
} {
	const d = (data ?? {}) as Record<string, unknown>
	if (
		typeof d.workspaceId !== 'string' ||
		typeof d.email !== 'string' ||
		(d.role !== 'owner' && d.role !== 'admin' && d.role !== 'member')
	)
		throw new Error('invalid input')
	return { workspaceId: d.workspaceId, email: d.email, role: d.role }
}
