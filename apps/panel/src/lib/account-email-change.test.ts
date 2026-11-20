import { describe, expect, it, vi } from 'vitest'
import {
	emailChangeAllowed,
	emailChangeFlow,
	emailLinkOutcome,
	parseEmailChangeInput,
} from './account-email-change'
import { changeEmailEmail, emailChangeNoticeEmail } from './account-email'

/** Attempts that are never exhausted. */
const openAttempts = () => ({
	take: vi.fn(async () => ({ ok: true as const })),
	clear: vi.fn(async () => {}),
})

const oidc = (only: boolean) => ({
	providerId: 'oidc',
	name: 'SSO',
	issuer: 'https://idp.example',
	clientId: 'c',
	clientSecret: 's',
	only,
})

describe('emailChangeAllowed', () => {
	it('is self-hosted only, and not when a provider is the only way in', () => {
		expect(emailChangeAllowed({ cloud: false, oidc: null })).toBe(true)
		expect(emailChangeAllowed({ cloud: false, oidc: oidc(false) })).toBe(true)
		expect(emailChangeAllowed({ cloud: false, oidc: oidc(true) })).toBe(false)
		expect(emailChangeAllowed({ cloud: true, oidc: oidc(true) })).toBe(false)
		expect(emailChangeAllowed({ cloud: true, oidc: null })).toBe(false)
	})
})

describe('emailChangeFlow', () => {
	const base = () => ({
		allowed: true,
		currentEmail: 'ana@example.com',
		newEmail: '  Ana@Nuevo.example ',
		password: 'secreto123',
		linkedToProvider: false,
		hasPassword: true,
		verifyPassword: vi.fn(async (p: string) => p === 'secreto123'),
		attempts: openAttempts(),
		requestChange: vi.fn(async (_email: string) => {}),
		notifyCurrent: vi.fn(async (_email: string) => {}),
	})

	it('stops at the limit of attempts, without checking the password', async () => {
		const retryAt = new Date('2026-11-20T15:42:00Z')
		const deps = {
			...base(),
			attempts: {
				take: vi.fn(async () => ({ ok: false as const, retryAt })),
				clear: vi.fn(async () => {}),
			},
		}
		expect(await emailChangeFlow(deps)).toEqual({
			ok: false,
			reason: 'too-many-attempts',
			retryAt: retryAt.toISOString(),
		})
		expect(deps.verifyPassword).not.toHaveBeenCalled()
		expect(deps.requestChange).not.toHaveBeenCalled()
	})

	it('counts a wrong password, and forgets the failures with the right one', async () => {
		const wrong = { ...base(), password: 'otra' }
		expect(await emailChangeFlow(wrong)).toMatchObject({
			reason: 'invalid-password',
		})
		expect(wrong.attempts.take).toHaveBeenCalledTimes(1)
		expect(wrong.attempts.clear).not.toHaveBeenCalled()
		const right = base()
		expect(await emailChangeFlow(right)).toMatchObject({ ok: true })
		expect(right.attempts.clear).toHaveBeenCalledTimes(1)
		// An invalid address isn't an attempt at the password.
		const invalid = { ...base(), newEmail: 'no' }
		await emailChangeFlow(invalid)
		expect(invalid.attempts.take).not.toHaveBeenCalled()
	})

	it('checks the password, sends the link, then tells the current address', async () => {
		const deps = base()
		const order: string[] = []
		deps.requestChange.mockImplementation(async (email) => {
			order.push(`link ${email}`)
		})
		deps.notifyCurrent.mockImplementation(async (email) => {
			order.push(`notice ${email}`)
		})
		expect(await emailChangeFlow(deps)).toEqual({
			ok: true,
			email: 'ana@nuevo.example',
		})
		expect(order).toEqual([
			'link ana@nuevo.example',
			'notice ana@nuevo.example',
		])
	})

	it('sends nothing with a wrong password', async () => {
		const deps = { ...base(), password: 'otra' }
		expect(await emailChangeFlow(deps)).toEqual({
			ok: false,
			reason: 'invalid-password',
		})
		expect(deps.requestChange).not.toHaveBeenCalled()
		expect(deps.notifyCurrent).not.toHaveBeenCalled()
	})

	it('refuses the same address, in any case, and one that is not one', async () => {
		expect(
			await emailChangeFlow({ ...base(), newEmail: 'ANA@example.com' })
		).toEqual({ ok: false, reason: 'same-email' })
		expect(await emailChangeFlow({ ...base(), newEmail: 'ana' })).toEqual({
			ok: false,
			reason: 'invalid-email',
		})
	})

	it('leaves the provider’s e-mail alone', async () => {
		for (const deps of [
			{ ...base(), linkedToProvider: true },
			{ ...base(), hasPassword: false },
		]) {
			expect(await emailChangeFlow(deps)).toEqual({
				ok: false,
				reason: 'provider',
			})
			expect(deps.verifyPassword).not.toHaveBeenCalled()
		}
	})

	it('does nothing where it is not offered', async () => {
		const deps = { ...base(), allowed: false }
		expect(await emailChangeFlow(deps)).toEqual({
			ok: false,
			reason: 'disabled',
		})
		expect(deps.verifyPassword).not.toHaveBeenCalled()
	})

	it('fails when the link does not go out, not when the notice does not', async () => {
		const failing = base()
		failing.requestChange.mockRejectedValue(new Error('Resend answered 500'))
		expect(await emailChangeFlow(failing)).toEqual({
			ok: false,
			reason: 'failed',
		})
		expect(failing.notifyCurrent).not.toHaveBeenCalled()
		const notice = base()
		notice.notifyCurrent.mockRejectedValue(new Error('Resend answered 500'))
		expect(await emailChangeFlow(notice)).toMatchObject({ ok: true })
	})
})

describe('parseEmailChangeInput', () => {
	it('takes the address and the password', () => {
		expect(parseEmailChangeInput({ email: 'a@b.c', password: 'x' })).toEqual({
			email: 'a@b.c',
			password: 'x',
		})
		expect(() => parseEmailChangeInput({ email: 'a@b.c' })).toThrow()
		expect(() => parseEmailChangeInput(null)).toThrow()
	})
})

describe('emailLinkOutcome', () => {
	it('reads where the link came back', () => {
		expect(emailLinkOutcome({ email: 'changed' })).toEqual({ changed: true })
		expect(emailLinkOutcome({})).toBeNull()
		expect(
			emailLinkOutcome({ email: 'changed', error: 'TOKEN_EXPIRED' })
		).toMatchObject({
			changed: false,
			message: expect.stringContaining('venció'),
		})
		expect(emailLinkOutcome({ error: 'INVALID_USER' })).toMatchObject({
			changed: false,
			message: expect.stringContaining('otra cuenta'),
		})
	})
})

describe('the e-mails', () => {
	it('send the link to the new address and tell the current one', () => {
		const link = changeEmailEmail(
			'https://sign.example/api/auth/verify-email?token=t&callbackURL=%2Faccount'
		)
		expect(link.text).toContain('verify-email?token=t')
		expect(link.html).toContain('verify-email?token=t&amp;callbackURL')
		const notice = emailChangeNoticeEmail(
			'nuevo@example.com',
			'https://sign.example/account'
		)
		expect(notice.text).toContain('nuevo@example.com')
		expect(notice.text).toContain('https://sign.example/account')
	})
})
