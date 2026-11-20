import handler, { createServerEntry } from '@tanstack/react-start/server-entry'
import { readConfig } from '#/lib/config'
import { withRequestDatabase } from '#/lib/database'
import { onWorkers, serverEnv } from '#/lib/env'
import { movedResponse } from '#/lib/moved'
import { planCookieFor } from '#/lib/plan'

/**
 * The server's entry: TanStack Start's own, and on Cloudflare Workers each
 * request with a database pool of its own (`lib/database.ts`). In Node the
 * process's pool serves every request, as before.
 *
 * Before it: the old Sign's addresses, redirected (`lib/moved.ts`), and the
 * plan chosen on Sello's site, kept in its cookie (`lib/plan.ts`).
 */
export default createServerEntry({
	async fetch(request, options) {
		const config = readConfig(serverEnv())
		const cookie = planCookieFor(new URL(request.url), config.cloud)
		const response =
			movedResponse(request, config.apiUrl) ??
			(onWorkers()
				? await withRequestDatabase(async () => handler.fetch(request, options))
				: await handler.fetch(request, options))
		if (!cookie) return response
		const out = new Response(response.body, response)
		out.headers.append('set-cookie', cookie)
		return out
	},
})
