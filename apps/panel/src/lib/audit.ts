/**
 * «Actividad»: the space's audit log (`GET /workspaces/{w}/audit-events`,
 * contracts/README.md) in Spanish, for its owners and admins. Each action
 * of the API (`apps/api/internal/audit/audit.go`) is worded here, as a
 * sentence whose subject is who acted; `audit.test.ts` compares the two
 * lists, so an action the API adds breaks that test until the panel words
 * it. Pure, so the page (`routes/_app.w.$workspaceId.activity.tsx`) only
 * lays it out.
 */

import type { AuditActorType, AuditEvent, AuditTargetType } from './api'
import { countryName } from './countries'
import { ROLE_LABELS, formatDateTime, formatNumber } from './format'
import { planLabel } from './limits'

/** How long the API keeps an event (`audit.RetentionDays`). */
export const AUDIT_RETENTION_DAYS = 365

/** The filter «Qué»: what the events are about. */
export const TARGET_LABELS: Record<AuditTargetType, string> = {
	workspace: 'El espacio',
	member: 'Miembros',
	invitation: 'Invitaciones',
	organization: 'Organizaciones',
	certificate: 'Certificados',
	delegation: 'Delegaciones',
	api_key: 'Claves de API',
}

/** What one event is about, in the dialog. */
export const TARGET_NAMES: Record<AuditTargetType, string> = {
	workspace: 'Espacio',
	member: 'Miembro',
	invitation: 'Invitación',
	organization: 'Organización',
	certificate: 'Certificado',
	delegation: 'Delegación',
	api_key: 'Clave de API',
}

/** The filter «Quién» besides each member: `user:<id>` or one of these. */
export const ACTOR_FILTERS: { value: AuditActorType; label: string }[] = [
	{ value: 'api_key', label: 'Claves de API' },
	{ value: 'workspace', label: 'Otros espacios' },
	{ value: 'billing', label: 'Sello (el plan)' },
	{ value: 'console', label: 'Consola del servidor' },
]

/** The API's filters for the value of «Quién». */
export function actorFilter(value: string | undefined): {
	actor_type?: AuditActorType
	actor_id?: string
} {
	if (!value) return {}
	if (value.startsWith('user:'))
		return { actor_type: 'user', actor_id: value.slice('user:'.length) }
	const known = ACTOR_FILTERS.find((f) => f.value === value)
	return known ? { actor_type: known.value } : {}
}

const ENVIRONMENT_WORDS: Record<string, string> = {
	test: 'prueba',
	prod: 'producción',
}

const text = (v: unknown): string => (typeof v === 'string' ? v : '')
const quoted = (v: string) => (v ? `«${v}»` : '')
const role = (v: unknown): string =>
	ROLE_LABELS[v as keyof typeof ROLE_LABELS] ?? text(v)
const environment = (v: unknown): string =>
	ENVIRONMENT_WORDS[text(v)] ?? text(v)

/** «a, b y c». */
function listed(items: string[]): string {
	if (items.length <= 1) return items.join('')
	return `${items.slice(0, -1).join(', ')} y ${items.at(-1)}`
}

/** What each field of `changes` is called, inside a sentence and as a
 * label, and how its values read. */
const FIELDS: Record<
	string,
	{ word: string; label: string; value?: (v: unknown) => string }
> = {
	name: { word: 'el nombre', label: 'Nombre', value: (v) => quoted(text(v)) },
	default_country: {
		word: 'el país por defecto',
		label: 'País por defecto',
		value: (v) => (v ? countryName(text(v)) : 'Ninguno'),
	},
	requests_per_minute: {
		word: 'las firmas por minuto',
		label: 'Firmas por minuto',
	},
	contact_email: {
		word: 'el correo de contacto',
		label: 'Correo de contacto',
		value: (v) => text(v) || 'Ninguno',
	},
	sello_workspace_id: {
		word: 'su organización en Sello',
		label: 'Organización en Sello',
		value: (v) => text(v) || 'Ninguna',
	},
	plan: {
		word: 'el plan',
		label: 'Plan',
		value: (v) => planLabel(text(v) || null) ?? 'Sin plan',
	},
	organizations: { word: 'las organizaciones', label: 'Organizaciones' },
	prod_signatures: {
		word: 'las firmas de producción',
		label: 'Firmas de producción al mes',
	},
	test_signatures: {
		word: 'las firmas de prueba',
		label: 'Firmas de prueba al mes',
	},
	members: { word: 'los miembros', label: 'Miembros' },
	retention_days: {
		word: 'los días del registro de firmas',
		label: 'Días del registro de firmas',
	},
	prod_hard_cap: {
		word: 'el tope de producción',
		label: 'Tope de producción al mes',
	},
	test_hard_cap: {
		word: 'el tope de prueba',
		label: 'Tope de prueba al mes',
	},
}

/** A value of a change: a limit or a number, or what its field says. */
function valueOf(field: string, v: unknown, action: string): string {
	const custom = FIELDS[field]?.value
	if (custom) return custom(v)
	if (typeof v === 'number') return formatNumber(v)
	if (v === null || v === undefined) {
		// The space's own signing rate, cleared, is its plan's.
		return action === 'workspace.updated' ? 'Las del plan' : 'Sin límite'
	}
	return String(v)
}

export type ChangeLine = {
	field: string
	label: string
	from: string
	to: string
}

function changesOf(
	e: AuditEvent
): Record<string, { from: unknown; to: unknown }> {
	const c = e.details.changes
	return c && typeof c === 'object'
		? (c as Record<string, { from: unknown; to: unknown }>)
		: {}
}

/** What an event changed, a line each: «Nombre: «A» → «B»». */
export function changeLines(e: AuditEvent): ChangeLine[] {
	const out: ChangeLine[] = Object.entries(changesOf(e)).map(
		([field, { from, to }]) => ({
			field,
			label: FIELDS[field]?.label ?? field,
			from: valueOf(field, from, e.action),
			to: valueOf(field, to, e.action),
		})
	)
	if (e.action === 'member.role_changed')
		out.push({
			field: 'role',
			label: 'Rol',
			from: role(e.details.from),
			to: role(e.details.to),
		})
	return out
}

/** «el nombre y las firmas por minuto», for the sentence of a change. */
function changedWords(e: AuditEvent, fallback: string): string {
	const words = Object.keys(changesOf(e)).map((f) => FIELDS[f]?.word ?? f)
	return words.length ? listed(words) : fallback
}

/**
 * What an event did, as a sentence whose subject is its actor (who goes
 * apart, `actorOf`): «Subió un certificado de prueba a «Empresa»».
 * `workspaceId` is the space whose log it is: a delegation or an
 * organization that moved reads from its side.
 */
export function describeEvent(e: AuditEvent, workspaceId: string): string {
	const d = e.details
	const label = quoted(e.target.label)
	switch (e.action) {
		case 'workspace.created':
			return `Creó el espacio ${label}`
		case 'workspace.updated':
			return `Cambió ${changedWords(e, 'los datos')} del espacio`
		case 'workspace.limits_changed':
			return `Cambió ${changedWords(e, 'los límites')} del espacio`
		case 'workspace.plan_changed':
			return 'plan' in changesOf(e)
				? 'Cambió el plan del espacio'
				: `Cambió ${changedWords(e, 'los límites')} del plan`
		case 'workspace.closed':
			return 'Cerró el espacio: su organización se eliminó en Sello'
		case 'workspace.reopened':
			return 'Volvió a abrir el espacio'
		case 'workspace.exported':
			return 'Exportó los datos del espacio'

		case 'member.added':
			return `Agregó a ${e.target.label || 'una persona'} con el rol «${role(d.role)}»`
		case 'member.role_changed':
			return `Cambió el rol de ${e.target.label || 'una persona'} de «${role(d.from)}» a «${role(d.to)}»`
		case 'member.removed':
			return `Quitó a ${e.target.label || 'una persona'} del espacio`
		case 'member.left':
			return d.account_deleted
				? 'Salió del espacio al borrar su cuenta'
				: 'Salió del espacio'

		case 'invitation.created':
			return `Invitó a ${e.target.label} con el rol «${role(d.role)}»`
		case 'invitation.renewed':
			return `Reenvió la invitación a ${e.target.label}`
		case 'invitation.revoked':
			return `Revocó la invitación a ${e.target.label}`
		case 'invitation.accepted':
			return d.joined === false
				? `Aceptó la invitación a ${e.target.label}, y ya era miembro`
				: `Aceptó la invitación a ${e.target.label} y entró con el rol «${role(d.role)}»`

		case 'organization.created':
			return `Creó la organización ${label}`
		case 'organization.updated':
			return `Cambió ${changedWords(e, 'los datos')} de la organización ${label}`
		case 'organization.deleted':
			return `Borró la organización ${label}`
		case 'organization.claimed':
			return d.to_workspace_id === workspaceId
				? `Trajo la organización ${label} desde ${quoted(text(d.from_workspace_name)) || 'la plataforma de Sello'}`
				: `Se llevó la organización ${label} a su espacio`
		case 'organization.returned':
			return d.to_workspace_id === workspaceId
				? `Devolvió la organización ${label} a este espacio`
				: `Devolvió la organización ${label} a la plataforma de Sello`

		case 'certificate.uploaded': {
			const what = `Subió un certificado de ${environment(d.environment)} a ${label}`
			if (d.active === false) return `${what}, sin activarlo`
			return d.replaced_certificate_id
				? `${what}, que reemplaza al activo`
				: what
		}
		case 'certificate.activated':
			return `Activó un certificado de ${environment(d.environment)} de ${label}`
		case 'certificate.deactivated':
			return `Desactivó un certificado de ${environment(d.environment)} de ${label}`
		case 'certificate.deleted':
			return `Borró un certificado de ${environment(d.environment)} de ${label}`

		case 'delegation.granted':
			return d.workspace_id === workspaceId
				? `Delegó ${label} a este espacio`
				: `Delegó ${label} a ${quoted(text(d.workspace_name)) || 'otro espacio'}`
		case 'delegation.removed':
			if (d.renounced) return `Renunció a la delegación de ${label}`
			return d.workspace_id === workspaceId
				? `Quitó a este espacio la delegación de ${label}`
				: `Quitó la delegación de ${label} a ${quoted(text(d.workspace_name)) || 'otro espacio'}`

		case 'api_key.created':
			return `Creó la clave de API ${label}`
		case 'api_key.revoked':
			switch (d.reason) {
				case 'organization_deleted':
					return `Revocó la clave de API ${label} al borrar su organización`
				case 'organization_returned':
					return `Revocó la clave de API ${label} al devolver su organización a la plataforma de Sello`
				default:
					return `Revocó la clave de API ${label}`
			}
	}
	return e.action
}

/** Who acted, as a name and a second line. */
export function actorOf(e: AuditEvent): {
	name: string
	detail: string | null
} {
	const a = e.actor
	switch (a.type) {
		case 'user':
			return a.name
				? { name: a.name, detail: a.email !== a.name ? a.email : null }
				: { name: a.email ?? a.id ?? 'Una persona', detail: null }
		case 'api_key':
			return {
				name: a.name ? `Clave ${quoted(a.name)}` : 'Una clave de API',
				detail: a.via === 'mcp' ? 'Por el servidor MCP' : 'Por el API',
			}
		case 'workspace':
			return {
				name: a.name ? `Espacio ${quoted(a.name)}` : 'Otro espacio',
				detail: 'Otro espacio',
			}
		case 'billing':
			return { name: 'Sello', detail: 'El plan de su organización' }
		case 'console':
			return { name: 'Consola del servidor', detail: null }
		default:
			return { name: 'Sello Sign', detail: null }
	}
}

/** The labels of the details the dialog shows, besides the changes. */
const DETAIL_LABELS: Record<string, string> = {
	role: 'Rol',
	tax_id: 'Identificación tributaria',
	country: 'País',
	default_country: 'País por defecto',
	contact_email: 'Correo de contacto',
	sello_workspace_id: 'Organización en Sello',
	plan: 'Plan',
	environment: 'Ambiente',
	serial: 'Número de serie',
	not_after: 'Vence',
	active: 'Activo',
	replaced_certificate_id: 'Reemplazó al certificado',
	organization_id: 'Organización',
	owner_workspace_name: 'Espacio dueño',
	workspace_name: 'Espacio delegado',
	scopes: 'Permisos',
	locked: 'Bloqueada por Sello',
	renounced: 'Renunció el espacio delegado',
	from_workspace_name: 'Desde el espacio',
	certificates: 'Certificados que tenía',
	requests_per_minute: 'Firmas por minuto',
	reason: 'Motivo',
	joined: 'Entró al espacio',
	account_deleted: 'Al borrar su cuenta',
}

const REASONS: Record<string, string> = {
	organization_deleted: 'Se borró su organización',
	organization_returned: 'Su organización volvió a la plataforma de Sello',
}

/** What the dialog says of an event besides its changes: a label and a
 * value each, in a stable order. */
export function detailRows(e: AuditEvent): [string, string][] {
	const out: [string, string][] = []
	for (const [key, label] of Object.entries(DETAIL_LABELS)) {
		if (!(key in e.details)) continue
		if (e.action === 'member.role_changed') continue
		const v = e.details[key]
		if (v === null || v === undefined || v === '') continue
		let value: string
		switch (key) {
			case 'role':
				value = role(v)
				break
			case 'environment':
				value = environment(v)
				break
			case 'country':
			case 'default_country':
				value = countryName(text(v))
				break
			case 'plan':
				value = planLabel(text(v)) ?? text(v)
				break
			case 'not_after':
				value = formatDateTime(text(v))
				break
			case 'reason':
				value = REASONS[text(v)] ?? text(v)
				break
			default:
				value =
					typeof v === 'boolean'
						? v
							? 'Sí'
							: 'No'
						: typeof v === 'number'
							? formatNumber(v)
							: Array.isArray(v)
								? v.join(', ')
								: String(v)
		}
		out.push([label, value])
	}
	return out
}

/** A date of the filter (`YYYY-MM-DD`, the person's day) as an instant. */
export const startOfDay = (day?: string) =>
	day ? new Date(`${day}T00:00:00`).toISOString() : undefined
export const endOfDay = (day?: string) =>
	day ? new Date(`${day}T23:59:59.999`).toISOString() : undefined
