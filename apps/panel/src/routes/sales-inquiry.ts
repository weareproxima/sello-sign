import { createFileRoute } from '@tanstack/react-router'
import { getAuth } from '#/lib/auth'
import { readConfig } from '#/lib/config'
import { serverEnv } from '#/lib/env'
import { handleInquiry, preflight } from '#/lib/sales-inquiry'

/**
 * `POST /sales-inquiry`: the «Hablar con ventas» dialog of Sello's site
 * (`lib/sales-inquiry.ts`). The configuration is read at each request, so it
 * works on Workers.
 */
export const Route = createFileRoute('/sales-inquiry')({
	server: {
		handlers: {
			OPTIONS: ({ request }) =>
				preflight(request, readConfig(serverEnv()).siteOrigin),
			POST: ({ request }) => {
				const config = readConfig(serverEnv())
				return handleInquiry(request, {
					siteOrigin: config.siteOrigin,
					email: config.email,
					sender: async () => {
						if (!config.databaseUrl) return null
						const session = await getAuth().api.getSession({
							headers: request.headers,
						})
						if (!session) return null
						const { id, name, email } = session.user
						return { id, name, email }
					},
				})
			},
		},
	},
})
