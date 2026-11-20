import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, type CreatedInvitation } from './api'
import {
	invitationEmail,
	invitationLink,
	inviteAndSend,
	parseInviteInput,
} from './invitations'
import { canChangeRole, membersPageUrl, rolesToGive } from './members'

const created: CreatedInvitation = {
	id: 'i1',
	email: 'beto@example.com',
	role: 'admin',
	status: 'pending',
	invited_by: { user_id: 'u1', email: 'ana@example.com', name: 'Ana' },
	expires_at: '2026-11-27T12:00:00Z',
	created_at: '2026-11-20T12:00:00Z',
	token: 'sign_inv_ABC',
}

const fakeApi = (create = async () => created) => ({
	createInvitation: vi.fn(create),
	workspace: vi.fn(async () => ({ name: 'Proveedor <A>' }) as never),
})

afterEach(() => vi.restoreAllMocks())

describe('invitationLink', () => {
	it('is the panel page with the token in the query', () => {
		expect(invitationLink('https://sign.sello.tax', 'sign_inv_a+b')).toBe(
			'https://sign.sello.tax/invitation?token=sign_inv_a%2Bb'
		)
	})
})

describe('invitationEmail', () => {
	it('names who invited, the space, the role and the address, escaped in HTML', () => {
		const m = invitationEmail({
			to: 'beto@example.com',
			workspaceName: 'Proveedor <A>',
			inviterName: 'Ana & Co',
			role: 'admin',
			link: 'https://x/invitation?token=t&a=1',
			expiresAt: '2026-11-27T12:00:00Z',
		})
		expect(m.to).toBe('beto@example.com')
		expect(m.subject).toBe('Ana & Co le invitó a Proveedor <A> en Sello Sign')
		expect(m.text).toContain('con el rol de Administrador')
		expect(m.text).toContain('https://x/invitation?token=t&a=1')
		expect(m.text).toContain('(beto@example.com)')
		expect(m.html).toContain('Proveedor &lt;A&gt;')
		expect(m.html).toContain('Ana &amp; Co')
		expect(m.html).toContain('href="https://x/invitation?token=t&amp;a=1"')
	})
})

describe('inviteAndSend', () => {
	const base = {
		panelUrl: 'https://sign.sello.tax',
		inviterName: 'Ana',
		workspaceId: 'w1',
		email: 'Beto@example.com',
		role: 'admin' as const,
	}

	it('creates it, sends the link and never hands back the token alone', async () => {
		const api = fakeApi()
		const send = vi.fn(async () => {})
		const result = await inviteAndSend({ ...base, api, send })
		expect(api.createInvitation).toHaveBeenCalledWith('w1', {
			email: 'Beto@example.com',
			role: 'admin',
		})
		expect(result).toEqual({
			ok: true,
			invitation: expect.not.objectContaining({ token: expect.anything() }),
			link: 'https://sign.sello.tax/invitation?token=sign_inv_ABC',
			emailed: true,
		})
		const message = (
			send.mock.calls[0] as unknown as [{ to: string; text: string }]
		)[0]
		expect(message.to).toBe('beto@example.com')
		expect(message.text).toContain('sign_inv_ABC')
	})

	it('keeps the invitation when the e-mail fails', async () => {
		vi.spyOn(console, 'error').mockImplementation(() => {})
		const result = await inviteAndSend({
			...base,
			api: fakeApi(),
			send: async () => {
				throw new Error('Resend answered 422')
			},
		})
		expect(result).toMatchObject({ ok: true, emailed: false })
	})

	it('answers the API problem without sending', async () => {
		const send = vi.fn(async () => {})
		const problem = {
			type: 'invitation-exists',
			title: 'Conflict',
			status: 409,
		}
		const result = await inviteAndSend({
			...base,
			api: fakeApi(async () => {
				throw new ApiError(409, problem)
			}),
			send,
		})
		expect(result).toEqual({ ok: false, status: 409, problem })
		expect(send).not.toHaveBeenCalled()
	})
})

describe('parseInviteInput', () => {
	it('takes a space, an address and a role, and nothing else', () => {
		expect(
			parseInviteInput({ workspaceId: 'w', email: 'a@b.c', role: 'member' })
		).toEqual({ workspaceId: 'w', email: 'a@b.c', role: 'member' })
		expect(() =>
			parseInviteInput({ workspaceId: 'w', email: 'a@b.c', role: 'boss' })
		).toThrow()
		expect(() => parseInviteInput(null)).toThrow()
	})
})

describe('members', () => {
	it('offers only the roles the API lets give', () => {
		expect(rolesToGive('owner')).toEqual(['owner', 'admin', 'member'])
		expect(rolesToGive('admin')).toEqual(['admin', 'member'])
		expect(rolesToGive('member')).toEqual([])
		expect(canChangeRole('admin', 'member')).toBe(true)
		expect(canChangeRole('admin', 'owner')).toBe(false)
		expect(canChangeRole('owner', 'owner')).toBe(true)
		expect(canChangeRole('member', 'member')).toBe(false)
	})

	it('links to the members of the organization at Sello', () => {
		expect(membersPageUrl('https://account.sello.tax', 'org 1')).toBe(
			'https://account.sello.tax/u/0/sign/members?organization=org+1'
		)
	})
})
