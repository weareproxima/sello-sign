/**
 * The plan's limits, as the API applies them (`apps/api/internal/httpapi/
 * limits.go`), and what the panel says when one stops a signature.
 *
 * - Signatures a minute: the key's (`requests_per_minute` of the key), then
 *   the space's: its plan's ceiling (`limits.requests_per_minute`) or a lower
 *   one the space set for itself (`requests_per_minute` of the space). Past
 *   it, `429 rate-limited` with `applies_to` and `Retry-After`.
 * - Signatures a month, per environment: up to the hard cap billing pushes
 *   (in a paid plan, the overage past the quota) or, without one, the quota.
 *   Past it, `429 monthly-limit-reached` until the first day of the next
 *   month (UTC).
 *
 * Pure: the pages and `errorMessage` call it, and `limits.test.ts` tests it.
 */
import type { Environment, Limits, Problem } from './api'

/** The most a space or a key may set (`maxRequestsPerMinute` of the API). */
export const MAX_REQUESTS_PER_MINUTE = 1_000_000

const LOCALE = 'es-419'
const number = (n: number) => n.toLocaleString(LOCALE)

/** The cloud's plans, as Sello's site names them (`../sello/apps/marketing`,
 * Sign's pricing). */
const PLAN_LABELS: Record<string, string> = {
	free: 'Gratis',
	pro_starter: 'Pro Esencial',
	pro_growth: 'Pro Profesional',
	pro_scale: 'Pro Escala',
	pro_business: 'Pro Business',
	enterprise: 'Enterprise',
}

/** A plan's name; `null` without a plan (self-hosted). */
export function planLabel(plan: string | null): string | null {
	if (!plan) return null
	return PLAN_LABELS[plan] ?? plan
}

/** The environment inside a sentence: «firmas de producción». */
const ENVIRONMENT_WORDS: Record<Environment, string> = {
	prod: 'producción',
	test: 'prueba',
}

/** Where signing stops in `env` this month: the hard cap or, without one,
 * the quota; `null` is none (`monthlyCap` of the API). */
export function monthlyCap(limits: Limits, env: Environment): number | null {
	return env === 'prod'
		? (limits.prod_hard_cap ?? limits.prod_signatures)
		: (limits.test_hard_cap ?? limits.test_signatures)
}

/** What the plan includes in `env` a month; `null` is no quota. */
export function monthlyQuota(limits: Limits, env: Environment): number | null {
	return env === 'prod' ? limits.prod_signatures : limits.test_signatures
}

/** The space's signatures a minute: its plan's ceiling, lowered by its own;
 * `null` is none (`requestsPerMinuteOf` of the API). */
export function effectiveRate(
	ceiling: number | null,
	own: number | null
): number | null {
	if (ceiling === null) return own
	if (own === null || own > ceiling) return ceiling
	return own
}

/** «12 segundos», «1 segundo». */
function seconds(n: number): string {
	return n === 1 ? '1 segundo' : `${number(n)} segundos`
}

/** The first day of the next month, in UTC as the API counts:
 * «1 de diciembre de 2026». */
export function formatReset(value: string): string | null {
	const date = new Date(value)
	if (Number.isNaN(date.getTime())) return null
	return date.toLocaleDateString(LOCALE, {
		timeZone: 'UTC',
		day: 'numeric',
		month: 'long',
		year: 'numeric',
	})
}

export type LimitProblem = 'rate' | 'monthly' | 'organizations'

/** Which of the plan's limits a problem is, or `null`. */
export function limitKind(problem: Problem | null): LimitProblem | null {
	switch (problem?.type) {
		case 'rate-limited':
			return 'rate'
		case 'monthly-limit-reached':
			return 'monthly'
		case 'organization-limit-reached':
			return 'organizations'
	}
	return null
}

/**
 * The sentence for a limit that stopped an action, with Sello's mold
 * «Alcanzó el límite de…»: whose limit it is and when to try again (from
 * `Retry-After`, in seconds), or when the month's count starts again.
 * `null` if the problem isn't one of the plan's limits.
 */
export function limitMessage(
	problem: Problem | null,
	retryAfter: number | null
): string | null {
	if (!problem) return null
	const limit = typeof problem.limit === 'number' ? problem.limit : null
	switch (limitKind(problem)) {
		case 'rate': {
			const whose =
				problem.applies_to === 'api_key' ? 'de la clave' : 'del espacio'
			const what =
				limit === null
					? `Alcanzó el límite de firmas por minuto ${whose}.`
					: `Alcanzó el límite de ${number(limit)} firmas por minuto ${whose}.`
			const when =
				retryAfter !== null && retryAfter > 0
					? `Vuelva a intentarlo en ${seconds(retryAfter)}.`
					: 'Vuelva a intentarlo en un minuto.'
			return `${what} ${when}`
		}
		case 'monthly': {
			const env = problem.environment
				? ` de ${ENVIRONMENT_WORDS[problem.environment]}`
				: ''
			const what =
				limit === null
					? `Alcanzó el límite de firmas${env} de este mes del plan del espacio.`
					: `Alcanzó el límite de ${number(limit)} firmas${env} de este mes del plan del espacio.`
			const reset = problem.resets_at ? formatReset(problem.resets_at) : null
			const when = reset
				? `La cuenta empieza de nuevo el ${reset}.`
				: 'La cuenta empieza de nuevo el primer día del mes siguiente.'
			return `${what} ${when}`
		}
		case 'organizations':
			return limit === null
				? 'Alcanzó el límite de organizaciones del plan del espacio.'
				: `Alcanzó el límite de ${number(limit)} organizaciones del plan del espacio.`
	}
	return null
}

/**
 * Reads a signatures-a-minute field: only digits go in (DESIGN.md › Campos),
 * up to as many as the largest value has.
 */
export function rateInput(value: string): string {
	return value
		.replace(/\D/g, '')
		.slice(0, String(MAX_REQUESTS_PER_MINUTE).length)
}

/**
 * What's wrong with a signatures-a-minute field, or `null`: a whole number
 * from 1 to `max` (the plan's ceiling for the space, or the API's maximum).
 * Empty is `null` here; whether it may be empty is the form's to say.
 */
export function rateError(
	value: string,
	max: number | null = null
): string | null {
	const text = value.trim()
	if (text === '') return null
	if (!/^\d+$/.test(text)) return 'Escriba un número entero de firmas.'
	const n = Number(text)
	if (n < 1) return 'Escriba al menos 1 firma por minuto.'
	const top = Math.min(max ?? MAX_REQUESTS_PER_MINUTE, MAX_REQUESTS_PER_MINUTE)
	if (n > top)
		return max !== null && max < MAX_REQUESTS_PER_MINUTE
			? `No puede pasar el límite del plan, de ${number(max)} firmas por minuto.`
			: `Escriba hasta ${number(MAX_REQUESTS_PER_MINUTE)} firmas por minuto.`
	return null
}

/**
 * The field error for the API's `400` when the space's own rate passes its
 * plan's ceiling (which billing may have lowered since the page loaded). The
 * API says the ceiling in its detail («…del plan del espacio, 120 por
 * minuto.»); without it, the sentence goes without the number.
 */
export function ownRateProblem(problem: Problem | null): string | null {
	if (!problem || problem.status !== 400) return null
	const ceiling = /(\d+) por minuto/.exec(problem.detail ?? '')?.[1]
	if (/plan/.test(problem.detail ?? ''))
		return ceiling
			? `No puede pasar el límite del plan, de ${number(Number(ceiling))} firmas por minuto.`
			: 'No puede pasar el límite del plan del espacio.'
	return `Escriba un número de 1 a ${number(MAX_REQUESTS_PER_MINUTE)}.`
}
