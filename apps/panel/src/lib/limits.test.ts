import { describe, expect, it } from 'vitest'
import type { Limits, Problem } from './api'
import {
	effectiveRate,
	formatReset,
	limitKind,
	limitMessage,
	monthlyCap,
	monthlyQuota,
	ownRateProblem,
	rateError,
	rateInput,
} from './limits'

const none: Limits = {
	organizations: null,
	prod_signatures: null,
	test_signatures: null,
	members: null,
	requests_per_minute: null,
	retention_days: null,
	prod_hard_cap: null,
	test_hard_cap: null,
}

const problem = (p: Partial<Problem>): Problem => ({
	type: 'internal',
	title: '',
	status: 429,
	...p,
})

describe('the monthly limits', () => {
	it('stop at the hard cap, or at the quota without one', () => {
		const paid = {
			...none,
			prod_signatures: 1000,
			prod_hard_cap: 2000,
			test_signatures: 100,
			test_hard_cap: 110,
		}
		expect(monthlyCap(paid, 'prod')).toBe(2000)
		expect(monthlyCap(paid, 'test')).toBe(110)
		expect(monthlyQuota(paid, 'prod')).toBe(1000)
		const free = { ...none, prod_signatures: 50 }
		expect(monthlyCap(free, 'prod')).toBe(50)
		expect(monthlyCap(free, 'test')).toBeNull()
	})
})

describe('effectiveRate', () => {
	it("is the plan's ceiling lowered by the space's own", () => {
		expect(effectiveRate(120, null)).toBe(120)
		expect(effectiveRate(120, 30)).toBe(30)
		expect(effectiveRate(120, 500)).toBe(120)
		expect(effectiveRate(null, 30)).toBe(30)
		expect(effectiveRate(null, null)).toBeNull()
	})
})

describe('limitMessage', () => {
	it('says whose per-minute limit it is and when to retry', () => {
		expect(
			limitMessage(
				problem({ type: 'rate-limited', limit: 1200, applies_to: 'workspace' }),
				42
			)
		).toBe(
			'Alcanzó el límite de 1,200 firmas por minuto del espacio. Vuelva a intentarlo en 42 segundos.'
		)
		expect(
			limitMessage(
				problem({ type: 'rate-limited', limit: 10, applies_to: 'api_key' }),
				1
			)
		).toBe(
			'Alcanzó el límite de 10 firmas por minuto de la clave. Vuelva a intentarlo en 1 segundo.'
		)
		expect(limitMessage(problem({ type: 'rate-limited' }), null)).toBe(
			'Alcanzó el límite de firmas por minuto del espacio. Vuelva a intentarlo en un minuto.'
		)
	})

	it("says the month's limit of the environment and when it starts again", () => {
		expect(
			limitMessage(
				problem({
					type: 'monthly-limit-reached',
					limit: 2000,
					environment: 'prod',
					resets_at: '2026-12-01T00:00:00Z',
				}),
				864_000
			)
		).toBe(
			'Alcanzó el límite de 2,000 firmas de producción de este mes del plan del espacio. La cuenta empieza de nuevo el 1 de diciembre de 2026.'
		)
		expect(
			limitMessage(
				problem({ type: 'monthly-limit-reached', environment: 'test' }),
				null
			)
		).toBe(
			'Alcanzó el límite de firmas de prueba de este mes del plan del espacio. La cuenta empieza de nuevo el primer día del mes siguiente.'
		)
	})

	it('says the organizations of the plan', () => {
		expect(
			limitMessage(
				problem({ type: 'organization-limit-reached', status: 409, limit: 3 }),
				null
			)
		).toBe('Alcanzó el límite de 3 organizaciones del plan del espacio.')
	})

	it('leaves the other problems alone', () => {
		expect(limitMessage(problem({ type: 'certificate-expired' }), 5)).toBeNull()
		expect(limitMessage(null, null)).toBeNull()
		expect(limitKind(problem({ type: 'rate-limited' }))).toBe('rate')
		expect(limitKind(null)).toBeNull()
	})
})

describe('formatReset', () => {
	it('is the day in UTC, whatever the zone of the browser', () => {
		// The first instant of December in UTC is still November in the
		// Americas: the API counts in UTC.
		expect(formatReset('2027-01-01T00:00:00Z')).toBe('1 de enero de 2027')
		expect(formatReset('nope')).toBeNull()
	})
})

describe('the signatures-a-minute field', () => {
	it('lets only digits in', () => {
		expect(rateInput('1,2a0')).toBe('120')
		expect(rateInput('123456789')).toBe('1234567')
	})

	it("checks the range and the plan's ceiling", () => {
		expect(rateError('')).toBeNull()
		expect(rateError('60', 120)).toBeNull()
		expect(rateError('0', 120)).toBe('Escriba al menos 1 firma por minuto.')
		expect(rateError('121', 120)).toBe(
			'No puede pasar el límite del plan, de 120 firmas por minuto.'
		)
		expect(rateError('2000000')).toBe(
			'Escriba hasta 1,000,000 firmas por minuto.'
		)
	})

	it("words the API's 400", () => {
		expect(
			ownRateProblem(
				problem({
					type: 'invalid-request',
					status: 400,
					detail:
						'«requests_per_minute» no puede pasar el límite del plan del espacio, 1200 por minuto.',
				})
			)
		).toBe('No puede pasar el límite del plan, de 1,200 firmas por minuto.')
		expect(
			ownRateProblem(
				problem({
					type: 'invalid-request',
					status: 400,
					detail: '«requests_per_minute» es un número de 1 a 1000000, o null.',
				})
			)
		).toBe('Escriba un número de 1 a 1,000,000.')
		expect(ownRateProblem(problem({ status: 403 }))).toBeNull()
	})
})
