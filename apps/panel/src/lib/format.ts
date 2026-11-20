/** Dates and numbers in `es-419`, as Sello. */
const LOCALE = 'es-419'

function toDate(value: string | number | Date): Date | null {
	const date = value instanceof Date ? value : new Date(value)
	return Number.isNaN(date.getTime()) ? null : date
}

export function formatDate(value: string | number | Date | null | undefined) {
	if (value == null) return '—'
	const date = toDate(value)
	return date
		? date.toLocaleDateString(LOCALE, {
				year: 'numeric',
				month: 'short',
				day: 'numeric',
			})
		: '—'
}

export function formatDateTime(
	value: string | number | Date | null | undefined
) {
	if (value == null) return '—'
	const date = toDate(value)
	return date
		? date.toLocaleString(LOCALE, {
				year: 'numeric',
				month: 'short',
				day: 'numeric',
				hour: '2-digit',
				minute: '2-digit',
			})
		: '—'
}

export const formatNumber = (n: number) => n.toLocaleString(LOCALE)

/** Days from now until `value` (negative once past). */
export function daysUntil(value: string, now = Date.now()): number {
	return Math.floor((new Date(value).getTime() - now) / 86_400_000)
}

export const ROLE_LABELS = {
	owner: 'Propietario',
	admin: 'Administrador',
	member: 'Miembro',
} as const

export const ENVIRONMENT_LABELS = {
	test: 'Pruebas',
	prod: 'Producción',
} as const
