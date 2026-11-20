import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { AUDIT_TARGET_TYPES, type AuditEvent } from './api'
import {
	TARGET_LABELS,
	actorFilter,
	actorOf,
	changeLines,
	describeEvent,
	detailRows,
} from './audit'

const read = (path: string) =>
	readFileSync(fileURLToPath(new URL(path, import.meta.url)), 'utf8')

/** The actions `audit.go` declares: `Name Action = "target.verb"`. */
function actionsOfTheApi(): string[] {
	const source = read('../../../api/internal/audit/audit.go')
	return [...source.matchAll(/\b\w+\s+Action = "([^"]+)"/g)].map((m) => m[1]!)
}

const HERE = 'w-1'

function event(action: string, patch: Partial<AuditEvent> = {}): AuditEvent {
	return {
		id: 'e-1',
		action,
		actor: {
			type: 'user',
			id: 'u-1',
			email: 'ana@example.com',
			name: 'Ana',
			via: null,
		},
		target: { type: 'organization', id: 'o-1', label: 'Empresa' },
		details: {},
		created_at: '2026-11-20T15:00:00Z',
		...patch,
	}
}

describe('describeEvent', () => {
	it('words every action of the API', () => {
		const actions = actionsOfTheApi()
		expect(actions.length).toBeGreaterThan(25)
		for (const action of actions) {
			const sentence = describeEvent(event(action), HERE)
			expect(sentence, action).not.toBe(action)
			expect(sentence, action).toMatch(/^[A-ZÁÉÍÓÚ]/)
		}
	})

	it('names what changed', () => {
		const e = event('workspace.updated', {
			target: { type: 'workspace', id: HERE, label: 'Proveedor' },
			details: {
				changes: {
					name: { from: 'A', to: 'B' },
					requests_per_minute: { from: null, to: 30 },
				},
			},
		})
		expect(describeEvent(e, HERE)).toBe(
			'Cambió el nombre y las firmas por minuto del espacio'
		)
		expect(changeLines(e)).toEqual([
			{ field: 'name', label: 'Nombre', from: '«A»', to: '«B»' },
			{
				field: 'requests_per_minute',
				label: 'Firmas por minuto',
				from: 'Las del plan',
				to: '30',
			},
		])
	})

	it('reads a plan and its limits', () => {
		const e = event('workspace.plan_changed', {
			actor: { type: 'billing', id: null, email: null, name: null, via: null },
			details: {
				changes: {
					plan: { from: 'pro_starter', to: 'pro_growth' },
					prod_signatures: { from: 1000, to: 5000 },
					test_signatures: { from: 100, to: null },
				},
			},
		})
		expect(describeEvent(e, HERE)).toBe('Cambió el plan del espacio')
		expect(
			changeLines(e).map((l) => `${l.label}: ${l.from} → ${l.to}`)
		).toEqual([
			'Plan: Pro Esencial → Pro Profesional',
			'Firmas de producción al mes: 1,000 → 5,000',
			'Firmas de prueba al mes: 100 → Sin límite',
		])
		expect(actorOf(e)).toEqual({
			name: 'Sello',
			detail: 'El plan de su organización',
		})
	})

	it('reads a delegation from each side', () => {
		const granted = event('delegation.granted', {
			target: { type: 'delegation', id: 'd-1', label: 'Empresa' },
			details: {
				workspace_id: 'w-2',
				workspace_name: 'Otro',
				owner_workspace_id: HERE,
			},
		})
		expect(describeEvent(granted, HERE)).toBe('Delegó «Empresa» a «Otro»')
		expect(describeEvent(granted, 'w-2')).toBe(
			'Delegó «Empresa» a este espacio'
		)
		const removed = { ...granted, action: 'delegation.removed' }
		expect(describeEvent(removed, HERE)).toBe(
			'Quitó la delegación de «Empresa» a «Otro»'
		)
		expect(describeEvent(removed, 'w-2')).toBe(
			'Quitó a este espacio la delegación de «Empresa»'
		)
		expect(
			describeEvent(
				{ ...removed, details: { ...removed.details, renounced: true } },
				HERE
			)
		).toBe('Renunció a la delegación de «Empresa»')
	})

	it('reads certificates, members and keys', () => {
		expect(
			describeEvent(
				event('certificate.uploaded', {
					details: {
						environment: 'test',
						active: true,
						replaced_certificate_id: 'c-0',
					},
				}),
				HERE
			)
		).toBe(
			'Subió un certificado de prueba a «Empresa», que reemplaza al activo'
		)
		expect(
			describeEvent(
				event('member.role_changed', {
					target: { type: 'member', id: 'u-2', label: 'beto@example.com' },
					details: { from: 'member', to: 'admin' },
				}),
				HERE
			)
		).toBe('Cambió el rol de beto@example.com de «Miembro» a «Administrador»')
		expect(
			describeEvent(
				event('member.left', { details: { account_deleted: true } }),
				HERE
			)
		).toBe('Salió del espacio al borrar su cuenta')
		expect(
			describeEvent(
				event('api_key.revoked', {
					target: { type: 'api_key', id: 'k-1', label: 'Producción' },
					details: { reason: 'organization_deleted' },
				}),
				HERE
			)
		).toBe('Revocó la clave de API «Producción» al borrar su organización')
	})
})

describe('actorOf', () => {
	it('says who acted', () => {
		expect(actorOf(event('workspace.exported'))).toEqual({
			name: 'Ana',
			detail: 'ana@example.com',
		})
		expect(
			actorOf(
				event('api_key.revoked', {
					actor: {
						type: 'api_key',
						id: 'k-1',
						email: null,
						name: 'Agente',
						via: 'mcp',
					},
				})
			)
		).toEqual({ name: 'Clave «Agente»', detail: 'Por el servidor MCP' })
		expect(
			actorOf(
				event('certificate.uploaded', {
					actor: {
						type: 'workspace',
						id: 'w-2',
						email: null,
						name: 'Sello',
						via: null,
					},
				})
			).name
		).toBe('Espacio «Sello»')
		expect(
			actorOf(
				event('member.added', {
					actor: {
						type: 'console',
						id: null,
						email: null,
						name: null,
						via: null,
					},
				})
			).name
		).toBe('Consola del servidor')
		expect(
			actorOf(
				event('workspace.created', {
					actor: {
						type: 'user',
						id: 'u-9',
						email: 'x@example.com',
						name: null,
						via: null,
					},
				})
			)
		).toEqual({ name: 'x@example.com', detail: null })
	})
})

describe('detailRows', () => {
	it('labels what an event says, in order', () => {
		expect(
			detailRows(
				event('organization.created', {
					details: {
						tax_id: '0614-1',
						country: 'SV',
						contact_email: null,
						sello_workspace_id: null,
					},
				})
			)
		).toEqual([
			['Identificación tributaria', '0614-1'],
			['País', 'El Salvador'],
		])
		expect(
			detailRows(
				event('certificate.deleted', {
					details: { environment: 'prod', serial: '01', active: false },
				})
			)
		).toEqual([
			['Ambiente', 'producción'],
			['Número de serie', '01'],
			['Activo', 'No'],
		])
	})
})

describe('filters', () => {
	it('cover every target type', () => {
		expect(Object.keys(TARGET_LABELS).sort()).toEqual(
			[...AUDIT_TARGET_TYPES].sort()
		)
	})

	it('read «Quién»', () => {
		expect(actorFilter(undefined)).toEqual({})
		expect(actorFilter('user:u-1')).toEqual({
			actor_type: 'user',
			actor_id: 'u-1',
		})
		expect(actorFilter('api_key')).toEqual({ actor_type: 'api_key' })
		expect(actorFilter('nope')).toEqual({})
	})
})
