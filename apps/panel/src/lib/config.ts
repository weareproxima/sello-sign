/**
 * The panel's configuration, read from its environment. Pure functions over a
 * record so the rules can be tested without one (`config.test.ts`).
 *
 * Server only: it sees the OIDC client secret and the auth secret.
 */

/** An external OpenID Connect provider to sign in with. */
export type OidcConfig = {
	/** The provider's id in better-auth and in `account.provider_id`. */
	providerId: string
	/** Its name on the button: «Continuar con Sello». */
	name: string
	/** The issuer, without a trailing slash: discovery hangs from it. */
	issuer: string
	clientId: string
	clientSecret: string
	/** The only way in: no email and password, no sign-up here. */
	only: boolean
}

export type PanelConfig = {
	/** `BETTER_AUTH_URL`: the panel's own origin, the tokens' `iss`. */
	panelUrl: string
	/** `API_URL`: the API's origin, the tokens' `aud`. */
	apiUrl: string
	databaseUrl: string | null
	secret: string | null
	/** `CLOUD=true` in `sign.sello.tax`: spaces are Sello's organizations. */
	cloud: boolean
	oidc: OidcConfig | null
	/** `PUBLIC_SITE_URL`'s origin: Sello's site, the only one with CORS on
	 * `/sales-inquiry`. `https://sello.tax` unless set. */
	siteOrigin: string
	/** The sales dialog's mail: Resend's key (`null` → the console), where it
	 * goes and who sends it. */
	email: { resendKey: string | null; sales: string; from: string }
}

/** The origin of `value`, or `fallback` if it isn't a URL. */
function originOf(value: string | undefined, fallback: string): string {
	try {
		return new URL(value?.trim() || fallback).origin
	} catch {
		return new URL(fallback).origin
	}
}

const trimSlash = (s: string) => s.trim().replace(/\/+$/, '')

export function readOidcConfig(
	env: Record<string, string | undefined>
): OidcConfig | null {
	const issuer = env.OIDC_ISSUER ? trimSlash(env.OIDC_ISSUER) : ''
	const clientId = env.OIDC_CLIENT_ID?.trim()
	const clientSecret = env.OIDC_CLIENT_SECRET?.trim()
	if (!issuer || !clientId || !clientSecret) return null
	const cloud = env.CLOUD?.trim() === 'true'
	return {
		providerId: env.OIDC_PROVIDER_ID?.trim() || (cloud ? 'sello' : 'oidc'),
		name: env.OIDC_PROVIDER_NAME?.trim() || (cloud ? 'Sello' : 'SSO'),
		issuer,
		clientId,
		clientSecret,
		only: env.OIDC_ONLY?.trim() === 'true',
	}
}

export function readConfig(
	env: Record<string, string | undefined>
): PanelConfig {
	return {
		panelUrl: trimSlash(env.BETTER_AUTH_URL || 'http://127.0.0.1:4440'),
		apiUrl: trimSlash(env.API_URL || 'http://127.0.0.1:4441'),
		databaseUrl: env.DATABASE_URL?.trim() || null,
		secret: env.BETTER_AUTH_SECRET?.trim() || null,
		cloud: env.CLOUD?.trim() === 'true',
		oidc: readOidcConfig(env),
		siteOrigin: originOf(env.PUBLIC_SITE_URL, 'https://sello.tax'),
		email: {
			resendKey: env.RESEND_API_KEY?.trim() || null,
			sales: env.SALES_EMAIL?.trim() || 'sales@sign.sello.tax',
			from: env.EMAIL_FROM?.trim() || 'Sello Sign <noreply@sign.sello.tax>',
		},
	}
}

/** What the pages may know: never a secret. */
export type PublicConfig = {
	cloud: boolean
	oidc: { providerId: string; name: string; only: boolean } | null
	/** Sello's account center (its issuer), only in the cloud with Sello's
	 * account: where a plan chosen on Sello's site is bought (`lib/plan.ts`). */
	accountCenter: string | null
}

export function publicConfig(config: PanelConfig): PublicConfig {
	return {
		cloud: config.cloud,
		oidc: config.oidc
			? {
					providerId: config.oidc.providerId,
					name: config.oidc.name,
					only: config.oidc.only,
				}
			: null,
		accountCenter: config.cloud && config.oidc ? config.oidc.issuer : null,
	}
}

/** The claim of Sello's ID token that lists the person's organizations. */
export const WORKSPACES_CLAIM = 'https://sello.tax/workspaces'

export type SelloWorkspace = {
	id: string
	name: string
	country: string
	role: 'owner' | 'admin' | 'member'
}

const ROLES = ['owner', 'admin', 'member'] as const

/**
 * The organizations an ID token's payload lists (`WORKSPACES_CLAIM`), as
 * `organizationsIn` of ../sign-old read them: anything malformed is dropped.
 */
export function workspacesIn(
	payload: Record<string, unknown>
): SelloWorkspace[] {
	const claim = payload[WORKSPACES_CLAIM]
	if (!Array.isArray(claim)) return []
	const out: SelloWorkspace[] = []
	for (const item of claim) {
		if (!item || typeof item !== 'object') continue
		const r = item as Record<string, unknown>
		if (typeof r.id !== 'string' || typeof r.name !== 'string') continue
		const role = ROLES.find((x) => x === r.role) ?? 'member'
		out.push({
			id: r.id,
			name: r.name,
			country: typeof r.country === 'string' ? r.country : '',
			role,
		})
	}
	return out
}

/** The payload of a JWT, unverified: the token came from the provider over TLS
 * and better-auth already verified it at sign-in. `null` if it isn't one. */
export function decodeJwtPayload(
	token: string
): Record<string, unknown> | null {
	const part = token.split('.')[1]
	if (!part) return null
	try {
		const json = Buffer.from(part, 'base64url').toString('utf8')
		const value = JSON.parse(json) as unknown
		return value && typeof value === 'object'
			? (value as Record<string, unknown>)
			: null
	} catch {
		return null
	}
}
