import type { ApiKeyScope, DelegationScope } from './api'

export const DELEGATION_SCOPE_LABELS: Record<
	DelegationScope,
	{ label: string; description: string }
> = {
	sign: {
		label: 'Firmar',
		description: 'Firmar documentos con esta organización.',
	},
	'certificates:read': {
		label: 'Ver certificados',
		description: 'Consultar sus certificados.',
	},
	'certificates:write': {
		label: 'Administrar certificados',
		description: 'Subir, activar y desactivar sus certificados.',
	},
}

export const API_KEY_SCOPE_LABELS: Record<
	ApiKeyScope,
	{ label: string; description: string }
> = {
	sign: { label: 'Firmar', description: 'Firmar y verificar documentos.' },
	'organizations:read': {
		label: 'Ver organizaciones',
		description: 'Consultar las organizaciones del espacio.',
	},
	'organizations:write': {
		label: 'Administrar organizaciones',
		description: 'Crear, cambiar y borrar organizaciones.',
	},
	'certificates:read': {
		label: 'Ver certificados',
		description: 'Consultar los certificados de las organizaciones.',
	},
	'certificates:write': {
		label: 'Administrar certificados',
		description: 'Subir, activar y desactivar certificados.',
	},
	'delegations:read': {
		label: 'Ver delegaciones',
		description: 'Consultar las delegaciones de las organizaciones.',
	},
	'delegations:write': {
		label: 'Administrar delegaciones',
		description: 'Dar y quitar delegaciones.',
	},
	'keys:read': {
		label: 'Ver claves',
		description: 'Consultar las claves de API del espacio.',
	},
	'keys:write': {
		label: 'Administrar claves',
		description: 'Crear y revocar claves de API.',
	},
	'logs:read': {
		label: 'Ver el registro',
		description: 'Consultar el registro de firmas.',
	},
}
