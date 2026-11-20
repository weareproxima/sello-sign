/**
 * The server's environment, read when it's needed and never at module load.
 *
 * In Node (`bun run dev`, `node .output/server/index.mjs`) it's
 * `process.env`. On Cloudflare Workers the variables and secrets are the
 * Worker's bindings: workerd also copies them to `process.env` (nodejs_compat
 * with a compatibility date from 2025-04-01 on), and Nitro leaves them in
 * `globalThis.__env__` at every request; this reads both, so it doesn't depend
 * on either alone. Only strings: a binding to a resource isn't a variable.
 */
export function serverEnv(): Record<string, string | undefined> {
	const out: Record<string, string | undefined> = {}
	const bindings = (globalThis as { __env__?: Record<string, unknown> }).__env__
	if (bindings) {
		for (const [key, value] of Object.entries(bindings)) {
			if (typeof value === 'string') out[key] = value
		}
	}
	if (typeof process !== 'undefined' && process.env) {
		for (const [key, value] of Object.entries(process.env)) {
			if (value !== undefined) out[key] ??= value
		}
	}
	return out
}

/** Whether this runs on Cloudflare Workers (workerd), not in Node or Bun. */
export function onWorkers(): boolean {
	return (
		typeof navigator !== 'undefined' &&
		navigator.userAgent === 'Cloudflare-Workers'
	)
}
