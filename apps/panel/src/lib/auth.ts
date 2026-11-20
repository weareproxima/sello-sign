import { betterAuth } from 'better-auth'
import { tanstackStartCookies } from 'better-auth/tanstack-start'
import { authOptions } from './auth-options'
import { readConfig } from './config'
import { Database } from './database'
import { deliverEmail } from './email'
import { onWorkers, serverEnv } from './env'

/**
 * The panel's better-auth: accounts, sessions and the EdDSA token the panel
 * sends to the API (`/api/auth/token`, verified there with `/api/auth/jwks`).
 * Server only.
 */
function createAuth() {
	const env = serverEnv()
	const workers = onWorkers()
	// On Workers the defaults (this machine's ports) would be a panel that
	// issues tokens no API accepts: the deployment has to say where it is.
	if (workers && !env.BETTER_AUTH_URL?.trim()) {
		throw new Error('BETTER_AUTH_URL is not set (wrangler.jsonc, vars)')
	}
	const config = readConfig(env)
	if (!config.databaseUrl) throw new Error('DATABASE_URL is not set')
	if (!config.secret && workers)
		throw new Error('BETTER_AUTH_SECRET is not set')
	const options = authOptions({
		config,
		database: new Database(config.databaseUrl, { perRequest: workers }),
		// Resend with RESEND_API_KEY and EMAIL_FROM; without the key, the
		// server's console (`lib/email.ts`).
		sendEmail: (message) => deliverEmail(message, config.email),
	})
	return betterAuth({
		...options,
		plugins: [...options.plugins, tanstackStartCookies()],
	})
}

type Auth = ReturnType<typeof createAuth>

let instance: Auth | null = null

/**
 * Built on first use, from the environment of that moment (on Workers it
 * exists only inside a request), so a build without the environment still
 * works. One per process or isolate: its database finds each request's pool
 * by itself (`lib/database.ts`).
 */
export function getAuth(): Auth {
	instance ??= createAuth()
	return instance
}
