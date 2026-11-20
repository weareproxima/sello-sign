import type { BetterAuthOptions } from 'better-auth'
import { genericOAuth, jwt } from 'better-auth/plugins'
import { PostgresDialect } from 'kysely'
import { resetPasswordEmail } from './account-email'
import type { Database } from './database'
import {
	decodeJwtPayload,
	workspacesIn,
	type OidcConfig,
	type PanelConfig,
} from './config'

/**
 * The schema of database `sign` where better-auth keeps its tables. `public`
 * belongs to the API and its migrations; nothing of the panel goes there.
 */
export const AUTH_SCHEMA = 'auth'

/** How long a token for the API lasts (contracts/README.md › Credenciales). */
export const TOKEN_LIFETIME = '15m'

/**
 * better-auth's database: Postgres through Kysely, with every table named
 * with its schema (`auth."user"`). Not with `search_path` as a connection
 * option: Neon's pooled endpoint (PgBouncer) doesn't pass startup options to
 * Postgres, and the tables would be looked for in `public`.
 */
export function authDatabase(database: Database) {
	return {
		dialect: new PostgresDialect({ pool: database }),
		type: 'postgres' as const,
		schemaName: AUTH_SCHEMA,
		transaction: true,
	}
}

/**
 * The external provider for better-auth's `genericOAuth`: authorization code
 * with PKCE, the rest from the issuer's discovery document. In Sello's cloud
 * it also asks for `workspaces`, so Sello's ID token lists the person's
 * organizations (`https://sello.tax/workspaces`).
 */
export function oidcProvider(oidc: OidcConfig, cloud: boolean) {
	return {
		providerId: oidc.providerId,
		discoveryUrl: `${oidc.issuer}/.well-known/openid-configuration`,
		requireIdTokenVerification: true,
		clientId: oidc.clientId,
		clientSecret: oidc.clientSecret,
		tokenEndpointAuth: { method: 'client_secret_basic' as const },
		scopes: cloud
			? ['openid', 'profile', 'email', 'workspaces']
			: ['openid', 'profile', 'email'],
		pkce: true,
		overrideUserInfo: true,
		postLogoutRedirectURI: '/sign-in',
		mapProfileToUser: (profile: Record<string, unknown>) => ({
			name:
				(typeof profile.name === 'string' && profile.name) ||
				(typeof profile.email === 'string' ? profile.email : undefined),
			email: typeof profile.email === 'string' ? profile.email : undefined,
			emailVerified: profile.email_verified === true,
			image: typeof profile.picture === 'string' ? profile.picture : undefined,
		}),
	}
}

/** What a stored account says about the person at Sello, from its ID token. */
export function selloFieldsOf(idToken: string | null | undefined): {
	selloSub: string
	selloWorkspaces: string
} | null {
	if (!idToken) return null
	const payload = decodeJwtPayload(idToken)
	if (!payload || typeof payload.sub !== 'string') return null
	return {
		selloSub: payload.sub,
		selloWorkspaces: JSON.stringify(workspacesIn(payload)),
	}
}

/**
 * The claims of a token for the API besides `sub`, `iss`, `aud`, `iat` and
 * `exp`: `email` and `name`, and, for someone who came in with Sello's
 * account, `sello_sub` and `sello_workspaces` (contracts/README.md).
 */
export function tokenPayload(user: {
	email: string
	name: string
	selloSub?: string | null
	selloWorkspaces?: string | null
}): Record<string, unknown> {
	const payload: Record<string, unknown> = {
		email: user.email,
		name: user.name,
	}
	if (user.selloSub) {
		payload.sello_sub = user.selloSub
		let workspaces: unknown = []
		try {
			workspaces = JSON.parse(user.selloWorkspaces || '[]')
		} catch {
			workspaces = []
		}
		payload.sello_workspaces = Array.isArray(workspaces) ? workspaces : []
	}
	return payload
}

type Deps = {
	config: PanelConfig
	database: Database
	/** Sends an e-mail (`lib/email.ts`). */
	sendEmail: (message: {
		to: string
		subject: string
		text: string
		html?: string
	}) => Promise<void>
}

/**
 * better-auth's options for the panel. Plugins that only touch cookies
 * (`tanstackStartCookies`) are added by `auth.ts`; this is what the
 * migrations need too (`scripts/migrate.ts`).
 */
export function authOptions({ config, database, sendEmail }: Deps) {
	const oidc = config.oidc
	const oidcOnly = oidc?.only === true

	/** Keeps on the user what Sello's ID token says, at every sign-in. */
	async function rememberSello(account: {
		providerId: string
		userId: string
		idToken?: string | null
	}) {
		if (!oidc || account.providerId !== oidc.providerId) return
		const fields = selloFieldsOf(account.idToken)
		if (!fields) return
		await database.query(
			`update ${AUTH_SCHEMA}."user" set "selloSub" = $1, "selloWorkspaces" = $2, "updatedAt" = now() where id = $3`,
			[fields.selloSub, fields.selloWorkspaces, account.userId]
		)
	}

	return {
		appName: 'Sello Sign',
		baseURL: config.panelUrl,
		secret: config.secret ?? undefined,
		database: authDatabase(database),
		trustedOrigins: [config.panelUrl],
		emailAndPassword: {
			enabled: !oidcOnly,
			disableSignUp: oidcOnly,
			minPasswordLength: 8,
			sendResetPassword: async ({ user, url }) => {
				await sendEmail({ to: user.email, ...resetPasswordEmail(url) })
			},
		},
		user: {
			additionalFields: {
				// The person at Sello (`sub` of its ID token) and their
				// organizations there, as the last sign-in with Sello said. Only
				// `rememberSello` writes them.
				selloSub: { type: 'string', required: false, input: false },
				selloWorkspaces: { type: 'string', required: false, input: false },
			},
		},
		account: {
			accountLinking: {
				enabled: true,
				trustedProviders: oidc ? [oidc.providerId] : [],
			},
		},
		databaseHooks: {
			account: {
				create: { after: (account) => rememberSello(account) },
				update: { after: (account) => rememberSello(account) },
			},
		},
		plugins: [
			jwt({
				jwks: { keyPairConfig: { alg: 'EdDSA', crv: 'Ed25519' } },
				jwt: {
					issuer: config.panelUrl,
					audience: config.apiUrl,
					expirationTime: TOKEN_LIFETIME,
					definePayload: ({ user }) =>
						tokenPayload(
							user as typeof user & {
								selloSub?: string | null
								selloWorkspaces?: string | null
							}
						),
				},
				disableSettingJwtHeader: true,
			}),
			// Always mounted, so the routes don't depend on the environment.
			genericOAuth({
				config: oidc ? [oidcProvider(oidc, config.cloud)] : [],
			}),
		],
	} satisfies BetterAuthOptions
}
