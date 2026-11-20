import { describe, expect, it, vi } from 'vitest'
import {
	FRESH_SESSION_MS,
	accountDeletable,
	deleteAccountFlow,
	parseDeleteAccountInput,
} from './account-deletion'
import { ApiError } from './api'

const oidc = (only: boolean) => ({
	providerId: 'oidc',
	name: 'SSO',
	issuer: 'https://idp.example',
	clientId: 'c',
	clientSecret: 's',
	only,
})

describe('accountDeletable', () => {
	it('is self-hosted only, and not when a provider is the only way in', () => {
		expect(accountDeletable({ cloud: false, oidc: null })).toBe(true)
		expect(accountDeletable({ cloud: false, oidc: oidc(false) })).toBe(true)
		expect(accountDeletable({ cloud: false, oidc: oidc(true) })).toBe(false)
		expect(accountDeletable({ cloud: true, oidc: null })).toBe(false)
	})
})

describe('deleteAccountFlow', () => {
	const base = () => ({
		deletable: true,
		hasPassword: true,
		password: 'secreto123',
		verifyPassword: vi.fn(async (p: string) => p === 'secreto123'),
		sessionStartedAt: 0,
		now: 0,
		api: { deleteMe: vi.fn(async () => {}) },
		deleteUser: vi.fn(async () => {}),
	})

	it('asks the API first, then deletes the account', async () => {
		const deps = base()
		const order: string[] = []
		deps.api.deleteMe.mockImplementation(async () => {
			order.push('api')
		})
		deps.deleteUser.mockImplementation(async () => {
			order.push('user')
		})
		expect(await deleteAccountFlow(deps)).toEqual({ ok: true })
		expect(order).toEqual(['api', 'user'])
	})

	it('touches nothing with a wrong password', async () => {
		const deps = { ...base(), password: 'otra' }
		expect(await deleteAccountFlow(deps)).toEqual({
			ok: false,
			reason: 'invalid-password',
		})
		expect(deps.api.deleteMe).not.toHaveBeenCalled()
		expect(deps.deleteUser).not.toHaveBeenCalled()
	})

	it('without a password, wants a recent session', async () => {
		const stale = {
			...base(),
			hasPassword: false,
			password: null,
			now: FRESH_SESSION_MS + 1,
		}
		expect(await deleteAccountFlow(stale)).toMatchObject({
			reason: 'session-expired',
		})
		const fresh = { ...base(), hasPassword: false, password: null, now: 1000 }
		expect(await deleteAccountFlow(fresh)).toEqual({ ok: true })
	})

	it('keeps the account when the API refuses', async () => {
		const deps = base()
		const problem = {
			type: 'account-has-workspaces',
			title: 'Conflict',
			status: 409,
		}
		deps.api.deleteMe.mockRejectedValue(new ApiError(409, problem))
		expect(await deleteAccountFlow(deps)).toEqual({
			ok: false,
			reason: 'blocked',
			status: 409,
			problem,
		})
		expect(deps.deleteUser).not.toHaveBeenCalled()
	})

	it('does nothing where it is not offered', async () => {
		const deps = { ...base(), deletable: false }
		expect(await deleteAccountFlow(deps)).toEqual({
			ok: false,
			reason: 'disabled',
		})
		expect(deps.verifyPassword).not.toHaveBeenCalled()
	})
})

describe('parseDeleteAccountInput', () => {
	it('takes an optional password', () => {
		expect(parseDeleteAccountInput({ password: 'x' })).toEqual({
			password: 'x',
		})
		expect(parseDeleteAccountInput({})).toEqual({ password: null })
		expect(() => parseDeleteAccountInput({ password: 1 })).toThrow()
	})
})
