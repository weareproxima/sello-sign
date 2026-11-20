import { Outlet, createFileRoute, redirect } from '@tanstack/react-router'
import { getPublicConfig, getSession } from '#/server/session'

/** Everything behind a session. */
export const Route = createFileRoute('/_app')({
	beforeLoad: async ({ location }) => {
		const session = await getSession()
		if (!session)
			throw redirect({
				to: '/sign-in',
				search: { redirect: location.href },
			})
		return { session, config: await getPublicConfig() }
	},
	component: Outlet,
})
