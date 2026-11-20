import { describe, expect, it } from 'vitest'
import { movedResponse, movedTarget } from './moved'
import { planCookieFor, planIn, planPageUrl } from './plan'

const API = 'https://api.sign.sello.tax'
const at = (path: string) => new URL(`https://sign.sello.tax${path}`)

describe('the old addresses', () => {
	it('move with their query', () => {
		expect(movedTarget(at('/register?plan=pro_growth'), API)).toBe(
			'/sign-up?plan=pro_growth'
		)
		expect(movedTarget(at('/login?redirect=%2Fdashboard'), API)).toBe(
			'/sign-in?redirect=%2Fdashboard'
		)
		expect(movedTarget(at('/login/two-factor'), API)).toBe('/sign-in')
		expect(movedTarget(at('/dashboard'), API)).toBe('/')
		expect(movedTarget(at('/dashboard/'), API)).toBe('/')
		expect(movedTarget(at('/organizations/new'), API)).toBe('/workspaces/new')
		expect(movedTarget(at('/settings/organization'), API)).toBe('/')
		expect(movedTarget(at('/settings/workspace/signing'), API)).toBe('/')
		expect(movedTarget(at('/issuers/abc/certs'), API)).toBe('/')
		expect(movedTarget(at('/invite/tok'), API)).toBe('/')
	})

	it('send the specification to the API', () => {
		expect(movedTarget(at('/api/openapi'), API)).toBe(
			'https://api.sign.sello.tax/openapi.yaml'
		)
	})

	it('leave the new ones alone', () => {
		for (const path of [
			'/',
			'/sign-in',
			'/sign-up',
			'/workspaces/new',
			'/forgot-password',
			'/reset-password?token=x',
			'/w/1/settings',
			'/registered',
			'/api/sign/me',
		])
			expect(movedTarget(at(path), API)).toBeNull()
	})

	it('answer 308 to GET only', () => {
		const get = movedResponse(new Request(at('/register?plan=x')), API)
		expect(get?.status).toBe(308)
		expect(get?.headers.get('location')).toBe('/sign-up?plan=x')
		expect(
			movedResponse(new Request(at('/login'), { method: 'POST' }), API)
		).toBeNull()
	})
})

describe('the plan of Sello’s site', () => {
	it('waits in a cookie only in the cloud and for a known plan', () => {
		expect(planCookieFor(at('/sign-up?plan=pro_scale'), true)).toBe(
			'selected_plan=pro_scale; Path=/; Max-Age=3600; SameSite=Lax; Secure'
		)
		expect(planCookieFor(at('/register?plan=pro_scale'), true)).not.toBeNull()
		expect(planCookieFor(at('/sign-up?plan=pro_scale'), false)).toBeNull()
		expect(planCookieFor(at('/sign-up?plan=free'), true)).toBeNull()
		expect(planCookieFor(at('/w/1?plan=pro_scale'), true)).toBeNull()
		expect(
			planCookieFor(
				new URL('http://127.0.0.1:4440/sign-in?plan=pro_starter'),
				true
			)
		).not.toContain('Secure')
	})

	it('is read back from the cookies', () => {
		expect(planIn('a=1; selected_plan=pro_growth; b=2')).toBe('pro_growth')
		expect(planIn('selected_plan=enterprise')).toBeNull()
		expect(planIn('')).toBeNull()
	})

	it('is bought in the account center, for the organization', () => {
		expect(planPageUrl('https://account.sello.tax', 'org 1')).toBe(
			'https://account.sello.tax/u/0/sign/plan?organization=org+1'
		)
	})
})
