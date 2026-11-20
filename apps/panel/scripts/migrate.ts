/**
 * Creates or updates better-auth's tables, and the panel's own
 * `password_attempt`, in the `auth` schema of the panel's database
 * (`DATABASE_URL`). `bun run db:migrate`; Bun reads `.env`, and a
 * variable given in the shell wins over it, so the same command migrates
 * Neon (docs/deploy.md):
 *
 *   DATABASE_URL='postgres://…' bun run db:migrate
 */
import { getMigrations } from 'better-auth/db/migration'
import { AUTH_SCHEMA, authOptions } from '../src/lib/auth-options'
import { readConfig } from '../src/lib/config'
import { Database } from '../src/lib/database'
import { passwordAttemptTable } from '../src/lib/password-attempts'

// The tables don't depend on the OIDC provider, and leaving it out spares
// fetching its discovery document from here.
const config = { ...readConfig(process.env), oidc: null }
if (!config.databaseUrl) {
	console.error('DATABASE_URL is not set')
	process.exit(1)
}
const target = new URL(config.databaseUrl)
console.info(`auth: ${target.host}${target.pathname}, schema ${AUTH_SCHEMA}`)

const database = new Database(config.databaseUrl, { perRequest: false })
await database.query(`create schema if not exists ${AUTH_SCHEMA}`)

const options = authOptions({
	config,
	database,
	sendEmail: async () => {},
})
const { toBeCreated, toBeAdded, runMigrations } = await getMigrations(options)
if (toBeCreated.length === 0 && toBeAdded.length === 0) {
	console.info('auth: nothing to migrate')
} else {
	await runMigrations()
	console.info(
		`auth: created ${toBeCreated.map((t) => t.table).join(', ') || 'no tables'}; added columns to ${toBeAdded.map((t) => t.table).join(', ') || 'no tables'}`
	)
}
// The panel's own table, beside better-auth's: the attempts at a password
// (`src/lib/password-attempts.ts`), after `user`, which it references.
const had = await database.query<{ exists: boolean }>(
	`select to_regclass('${AUTH_SCHEMA}.password_attempt') is not null as exists`
)
await database.query(passwordAttemptTable(AUTH_SCHEMA))
if (!had.rows[0]?.exists) console.info('auth: created password_attempt')
await database.end()
