import { QueryClient } from '@tanstack/react-query'
import { createRouter as createTanStackRouter } from '@tanstack/react-router'
import { setupRouterSsrQueryIntegration } from '@tanstack/react-router-ssr-query'
import { ApiError } from './lib/api'
import { routeTree } from './routeTree.gen'

export function getRouter() {
	const queryClient = new QueryClient({
		defaultOptions: {
			queries: {
				staleTime: 15_000,
				// A 4xx won't change by asking again.
				retry: (count, error) =>
					!(error instanceof ApiError && error.status < 500) && count < 2,
			},
		},
	})

	const router = createTanStackRouter({
		routeTree,
		context: { queryClient },
		scrollRestoration: true,
		defaultPreload: 'intent',
		defaultPreloadStaleTime: 0,
	})

	setupRouterSsrQueryIntegration({ router, queryClient })

	return router
}

declare module '@tanstack/react-router' {
	interface Register {
		router: ReturnType<typeof getRouter>
	}
}
