/**
 * The API client: the contract of contracts/README.md, typed.
 *
 * Every call carries the panel's token (`Authorization: Bearer <JWT>`), which
 * better-auth signs with EdDSA for the API (`/api/auth/token`, 15 minutes).
 * It goes to `/api/sign/*`, which the panel's server forwards to `API_URL`
 * unchanged (`routes/api/sign/$.ts`).
 */

export type Environment = 'test' | 'prod'
export type Role = 'owner' | 'admin' | 'member'

export type Me = {
	user: { id: string; email: string; name: string }
	workspaces: WorkspaceSummary[]
}

export type WorkspaceSummary = {
	id: string
	name: string
	role: Role
	plan: string | null
	default_country: string | null
}

export type Limits = {
	organizations: number | null
	prod_signatures: number | null
	test_signatures: number | null
	members: number | null
	requests_per_minute: number | null
	retention_days: number | null
	prod_hard_cap: number | null
	test_hard_cap: number | null
}

export type Workspace = Omit<WorkspaceSummary, 'role'> & {
	role: Role | null
	sello_workspace_id: string | null
	closed: boolean
	limits: Limits
	created_at: string
}

export type SelloOrganizationStatus =
	'open' | 'available' | 'ask' | 'full' | 'closed'

export type SelloOrganization = {
	id: string
	name: string
	country: string
	role: Role
	status: SelloOrganizationStatus
	/** The space here, when it has one (`open`). */
	workspace_id: string | null
	max_members: number | null
}

export type Member = {
	user_id: string
	email: string | null
	name: string | null
	role: Role
}

/** Who sent an invitation. */
export type InvitedBy = {
	user_id: string
	email: string | null
	name: string | null
}

export type Invitation = {
	id: string
	email: string
	role: Role
	status: 'pending' | 'expired'
	invited_by: InvitedBy
	expires_at: string
	created_at: string
}

/** With its token, only when it's created. */
export type CreatedInvitation = Invitation & { token: string }

/** What an invitation says to whoever holds its link. */
export type InvitationLookup = {
	workspace_id: string
	workspace_name: string
	email: string
	role: Role
	status: 'pending' | 'expired' | 'accepted'
	invited_by: InvitedBy
	expires_at: string
}

export type UsageEntry = {
	month: string
	environment: Environment
	signatures: number
}

export type Usage = { plan: string | null; limits: Limits; items: UsageEntry[] }

export type Certificate = {
	id: string
	environment: Environment
	tax_id: string | null
	subject: string
	issuer: string
	serial: string
	not_before: string | null
	not_after: string | null
	active: boolean
	created_at: string
}

export type Relation = 'owned' | 'delegated'

export type Organization = {
	id: string
	name: string
	tax_id: string
	country: string
	relation: Relation
	sello_workspace_id: string | null
	contact_email: string | null
	/** For a delegated one: what the delegation lets this space do. */
	delegation: { id: string; scopes: DelegationScope[]; locked: boolean } | null
	certificates: { test: Certificate | null; prod: Certificate | null }
	created_at: string
}

export const DELEGATION_SCOPES = [
	'sign',
	'certificates:read',
	'certificates:write',
] as const
export type DelegationScope = (typeof DELEGATION_SCOPES)[number]

export type Delegation = {
	id: string
	workspace_id: string
	workspace_name: string
	scopes: DelegationScope[]
	locked: boolean
	created_at: string
}

export const API_KEY_SCOPES = [
	'sign',
	'organizations:read',
	'organizations:write',
	'certificates:read',
	'certificates:write',
	'delegations:read',
	'delegations:write',
	'keys:read',
	'keys:write',
	'logs:read',
] as const
export type ApiKeyScope = (typeof API_KEY_SCOPES)[number]

export type ApiKey = {
	id: string
	name: string
	environment: Environment
	scopes: ApiKeyScope[]
	organization_id: string | null
	prefix: string
	last_used_at: string | null
	created_at: string
}

export type CreatedApiKey = ApiKey & { key: string }

export type SignatureEntry = {
	id: string
	organization_id: string
	organization_name: string
	environment: Environment
	document_id: string | null
	document_type: string | null
	api_key_id: string | null
	/** Who signed from the panel; `null` for an API key. */
	user_id: string | null
	signed_by_workspace_id: string
	duration_ms: number
	created_at: string
}

export type Page<T> = { items: T[]; next_cursor: string | null }

export type SignResult = {
	signature: string
	document_id: string | null
	document_type: string | null
}

/** A problem+json answer (RFC 9457). */
export type Problem = {
	type: string
	title: string
	status: number
	detail?: string
}

export class ApiError extends Error {
	readonly status: number
	readonly problem: Problem | null

	constructor(status: number, problem: Problem | null) {
		super(problem?.detail || problem?.title || `HTTP ${status}`)
		this.name = 'ApiError'
		this.status = status
		this.problem = problem
	}

	/** The stable slug of the problem (`organization-not-found`…). */
	get type(): string | null {
		return this.problem?.type ?? null
	}
}

/** What the client needs from its surroundings; replaced in tests. */
export type Transport = {
	/** Where `/workspaces/…` hangs from: the panel's `/api/sign`. */
	base: string
	fetch: typeof fetch
	/** A fresh token for the API. */
	token: () => Promise<string>
}

/** Keeps the token until a minute before it expires. */
export function tokenCache(
	load: () => Promise<string>,
	now: () => number = Date.now
): { get: () => Promise<string>; clear: () => void } {
	let cached: { token: string; expires: number } | null = null
	let pending: Promise<string> | null = null
	return {
		async get() {
			if (cached && cached.expires - 60_000 > now()) return cached.token
			pending ??= load()
				.then((token) => {
					cached = { token, expires: expiryOf(token) ?? now() + 5 * 60_000 }
					return token
				})
				.finally(() => {
					pending = null
				})
			return pending
		},
		clear() {
			cached = null
		},
	}
}

/** `exp` of a JWT, in milliseconds; `null` if it can't be read. */
export function expiryOf(token: string): number | null {
	const part = token.split('.')[1]
	if (!part) return null
	try {
		const base64 = part.replace(/-/g, '+').replace(/_/g, '/')
		const json = JSON.parse(atob(base64)) as { exp?: unknown }
		return typeof json.exp === 'number' ? json.exp * 1000 : null
	} catch {
		return null
	}
}

type RequestOptions = {
	method?: string
	query?: Record<string, string | number | undefined | null>
	body?: unknown
	/** Sent as is, with this content type, instead of JSON. */
	raw?: { body: string; contentType: string }
	headers?: Record<string, string>
}

export function createApi(transport: Transport) {
	async function request<T>(
		path: string,
		options: RequestOptions = {}
	): Promise<T> {
		const url = new URL(`${transport.base}${path}`, 'http://panel.invalid')
		for (const [key, value] of Object.entries(options.query ?? {})) {
			if (value !== undefined && value !== null && value !== '')
				url.searchParams.set(key, String(value))
		}
		const headers: Record<string, string> = {
			accept: 'application/json',
			authorization: `Bearer ${await transport.token()}`,
			...options.headers,
		}
		let body: string | undefined
		if (options.raw) {
			body = options.raw.body
			headers['content-type'] = options.raw.contentType
		} else if (options.body !== undefined) {
			body = JSON.stringify(options.body)
			headers['content-type'] = 'application/json'
		}
		const target = transport.base.startsWith('http')
			? url.toString()
			: `${url.pathname}${url.search}`
		const response = await transport.fetch(target, {
			method: options.method ?? 'GET',
			headers,
			body,
		})
		if (!response.ok) {
			let problem: Problem | null = null
			try {
				const parsed = (await response.json()) as Partial<Problem>
				if (parsed && typeof parsed === 'object' && 'type' in parsed)
					problem = {
						type: String(parsed.type),
						title: String(parsed.title ?? ''),
						status: Number(parsed.status ?? response.status),
						detail: parsed.detail ? String(parsed.detail) : undefined,
					}
			} catch {
				problem = null
			}
			throw new ApiError(response.status, problem)
		}
		if (response.status === 204) return undefined as T
		const text = await response.text()
		return (text ? JSON.parse(text) : undefined) as T
	}

	const w = (workspace: string) =>
		`/workspaces/${encodeURIComponent(workspace)}`
	const o = (workspace: string, organization: string) =>
		`${w(workspace)}/organizations/${encodeURIComponent(organization)}`

	return {
		request,
		me: () => request<Me>('/me'),
		selloOrganizations: () =>
			request<SelloOrganization[] | { items: SelloOrganization[] }>(
				'/me/sello-organizations'
			).then(itemsOf),
		createWorkspace: (
			body:
				| { name: string; default_country: string }
				| { sello_workspace_id: string }
		) => request<Workspace>('/workspaces', { method: 'POST', body }),
		workspace: (id: string) => request<Workspace>(w(id)),
		updateWorkspace: (
			id: string,
			body: { name?: string; default_country?: string }
		) => request<Workspace>(w(id), { method: 'PATCH', body }),
		members: (id: string) =>
			request<Member[] | { items: Member[] }>(`${w(id)}/members`).then(itemsOf),
		updateMemberRole: (id: string, userId: string, role: Role) =>
			request<Member>(`${w(id)}/members/${encodeURIComponent(userId)}`, {
				method: 'PATCH',
				body: { role },
			}),
		invitations: (id: string) =>
			request<{ items: Invitation[] }>(`${w(id)}/invitations`).then(itemsOf),
		/** Answers the token once; the panel's server sends it by e-mail
		 * (`server/invitations.ts`). */
		createInvitation: (id: string, body: { email: string; role: Role }) =>
			request<CreatedInvitation>(`${w(id)}/invitations`, {
				method: 'POST',
				body,
			}),
		revokeInvitation: (id: string, invitationId: string) =>
			request<void>(
				`${w(id)}/invitations/${encodeURIComponent(invitationId)}`,
				{ method: 'DELETE' }
			),
		lookupInvitation: (token: string) =>
			request<InvitationLookup>('/invitation-lookups', {
				method: 'POST',
				body: { token },
			}),
		acceptInvitation: (token: string) =>
			request<Workspace>('/invitation-acceptances', {
				method: 'POST',
				body: { token },
			}),
		usage: (id: string) => request<Usage>(`${w(id)}/usage`),

		organizations: (workspace: string, relation?: Relation) =>
			request<Organization[] | Page<Organization>>(
				`${w(workspace)}/organizations`,
				{ query: { relation } }
			).then(itemsOf),
		organization: (workspace: string, id: string) =>
			request<Organization>(o(workspace, id)),
		createOrganization: (
			workspace: string,
			body: {
				name: string
				tax_id: string
				country: string
				contact_email?: string
				sello_workspace_id?: string
			}
		) =>
			request<Organization>(`${w(workspace)}/organizations`, {
				method: 'POST',
				body,
			}),
		updateOrganization: (
			workspace: string,
			id: string,
			body: { name?: string; contact_email?: string | null }
		) => request<Organization>(o(workspace, id), { method: 'PATCH', body }),
		deleteOrganization: (workspace: string, id: string) =>
			request<void>(o(workspace, id), { method: 'DELETE' }),

		certificates: (workspace: string, organization: string) =>
			request<Certificate[] | { items: Certificate[] }>(
				`${o(workspace, organization)}/certificates`
			).then(itemsOf),
		uploadCertificate: (
			workspace: string,
			organization: string,
			body: {
				environment: Environment
				file: string
				password?: string
				activate?: boolean
			}
		) =>
			request<Certificate>(`${o(workspace, organization)}/certificates`, {
				method: 'POST',
				body,
			}),
		setCertificateActive: (
			workspace: string,
			organization: string,
			id: string,
			active: boolean
		) =>
			request<Certificate>(
				`${o(workspace, organization)}/certificates/${encodeURIComponent(id)}`,
				{ method: 'PATCH', body: { active } }
			),
		deleteCertificate: (workspace: string, organization: string, id: string) =>
			request<void>(
				`${o(workspace, organization)}/certificates/${encodeURIComponent(id)}`,
				{ method: 'DELETE' }
			),

		delegations: (workspace: string, organization: string) =>
			request<Delegation[] | { items: Delegation[] }>(
				`${o(workspace, organization)}/delegations`
			).then(itemsOf),
		createDelegation: (
			workspace: string,
			organization: string,
			body: { workspace_id: string; scopes: DelegationScope[] }
		) =>
			request<Delegation>(`${o(workspace, organization)}/delegations`, {
				method: 'POST',
				body,
			}),
		deleteDelegation: (workspace: string, organization: string, id: string) =>
			request<void>(
				`${o(workspace, organization)}/delegations/${encodeURIComponent(id)}`,
				{ method: 'DELETE' }
			),

		/** Signs from the panel: JSON in El Salvador, XML elsewhere. */
		sign: (
			workspace: string,
			organization: string,
			environment: Environment,
			document: string,
			contentType: string
		) =>
			request<SignResult>(`${o(workspace, organization)}/signatures`, {
				method: 'POST',
				query: { environment },
				raw: { body: document, contentType },
			}),

		claimableOrganizations: (workspace: string) =>
			request<Organization[] | { items: Organization[] }>(
				`${w(workspace)}/claimable-organizations`
			).then(itemsOf),
		claimOrganizations: (workspace: string, organizationIds: string[]) =>
			request<unknown>(`${w(workspace)}/organization-claims`, {
				method: 'POST',
				body: { organization_ids: organizationIds },
			}),

		apiKeys: (workspace: string) =>
			request<ApiKey[] | { items: ApiKey[] }>(`${w(workspace)}/api-keys`).then(
				itemsOf
			),
		createApiKey: (
			workspace: string,
			body: {
				name: string
				environment: Environment
				scopes: ApiKeyScope[]
				organization_id?: string
			}
		) =>
			request<CreatedApiKey>(`${w(workspace)}/api-keys`, {
				method: 'POST',
				body,
			}),
		revokeApiKey: (workspace: string, id: string) =>
			request<void>(`${w(workspace)}/api-keys/${encodeURIComponent(id)}`, {
				method: 'DELETE',
			}),

		signatures: (
			workspace: string,
			filters: {
				organization_id?: string
				environment?: Environment
				from?: string
				to?: string
				limit?: number
				cursor?: string
			}
		) =>
			request<Page<SignatureEntry>>(`${w(workspace)}/signatures`, {
				query: filters,
			}),
		signature: (workspace: string, id: string) =>
			request<SignatureEntry>(
				`${w(workspace)}/signatures/${encodeURIComponent(id)}`
			),
	}
}

/** Lists come as an array or as a page; the pages want the array. */
function itemsOf<T>(value: T[] | { items: T[] }): T[] {
	return Array.isArray(value) ? value : (value?.items ?? [])
}

export type Api = ReturnType<typeof createApi>

/** The browser's token: from better-auth's `/api/auth/token`, with the
 * session cookie. */
const browserToken = tokenCache(async () => {
	const response = await fetch('/api/auth/token', { credentials: 'include' })
	if (!response.ok) throw new ApiError(401, null)
	const { token } = (await response.json()) as { token: string }
	return token
})

export const api = createApi({
	base: '/api/sign',
	fetch: (input, init) => fetch(input, init),
	token: browserToken.get,
})

/** Forget the token (signing out). */
export const clearToken = browserToken.clear

/**
 * The sentence for a failed action, with Sello's molds: «No se pudo
 * {acción}. Vuelva a intentarlo.», or what the problem's type says better.
 */
export function errorMessage(error: unknown, action: string): string {
	if (error instanceof ApiError) {
		const known = error.type ? PROBLEMS[error.type] : undefined
		if (known) return known
		if (error.status === 401)
			return 'Su sesión terminó. Vuelva a iniciar sesión.'
		if (error.status === 403) return 'No tiene permiso para hacer esto.'
		if (error.status === 502 || error.status === 503)
			return 'No se pudo contactar con el API. Vuelva a intentarlo en unos minutos.'
		if (error.status === 422 && error.problem?.detail)
			return error.problem.detail
	}
	if (error instanceof TypeError)
		return 'Revise su conexión y vuelva a intentarlo.'
	return `No se pudo ${action}. Vuelva a intentarlo.`
}

/** The problems the panel words itself; the rest use the mold. */
const PROBLEMS: Record<string, string> = {
	'delegation-locked':
		'Esta delegación es de la plataforma de Sello y solo ella puede quitarla.',
	'delegation-exists':
		'Ese espacio ya tiene una delegación de esta organización.',
	'delegation-not-found': 'La delegación ya no existe.',
	'organization-has-delegations':
		'La organización tiene delegaciones. Quítelas antes de borrarla.',
	'organization-not-found':
		'La organización no existe o ya no tiene acceso a ella.',
	'organization-not-claimable':
		'Alguna de esas organizaciones ya no se puede traer. Actualice la lista.',
	'not-organization-owner':
		'Solo el espacio dueño de la organización puede hacer esto.',
	'workspace-not-found': 'El espacio no existe o ya no tiene acceso a él.',
	'workspace-closed': 'Este espacio está cerrado.',
	'workspace-full': 'El plan de esta organización ya no admite más miembros.',
	'members-managed-by-sello':
		'Los miembros de este espacio se invitan y cambian de rol en su cuenta de Sello.',
	'member-not-found': 'Esa persona ya no es miembro del espacio.',
	'already-member': 'Esa persona ya es miembro del espacio.',
	'last-owner':
		'El espacio se quedaría sin propietario. Haga propietario a otra persona antes.',
	'invitation-not-found': 'La invitación no existe o se revocó.',
	'invitation-exists':
		'Ese correo ya tiene una invitación pendiente. Revóquela antes de enviar otra.',
	'invitation-expired':
		'La invitación venció. Pida otra a quien administra el espacio.',
	'invitation-used': 'Otra cuenta ya aceptó esta invitación.',
	'invitation-email-mismatch':
		'La invitación es para otro correo. Inicie sesión con la cuenta de ese correo.',
	'ask-admin':
		'Solo quien administra la organización puede empezar a usar Sello Sign con ella.',
	'sello-workspace-linked': 'Esa organización de Sello ya está vinculada.',
	'sello-organization-unknown':
		'Sello todavía no termina de crear esta organización. Vuelva a intentarlo en un momento.',
	'sello-organization-closed': 'Esta organización se eliminó en Sello.',
	'billing-unavailable':
		'No se pudo contactar con Sello. Vuelva a intentarlo en unos minutos.',
	'certificate-tax-id-mismatch':
		'La identificación del certificado no es la de la organización.',
	'invalid-certificate':
		'No se pudo leer el certificado. Revise el archivo y su PIN.',
	'certificate-not-active':
		'Esta organización no tiene un certificado activo en ese ambiente.',
	'certificate-expired': 'El certificado venció.',
	'country-not-enabled': 'Este país todavía no se ofrece en esta instalación.',
	'country-not-implemented':
		'Sello Sign todavía no firma documentos de este país.',
	'unsupported-content-type':
		'El tipo de documento no corresponde al país de la organización.',
	'invalid-document': 'El documento no es válido.',
	'payload-too-large': 'El documento es demasiado grande.',
	'api-unavailable':
		'No se pudo contactar con el API. Vuelva a intentarlo en unos minutos.',
}
