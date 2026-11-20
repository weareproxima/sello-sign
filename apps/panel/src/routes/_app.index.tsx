import * as React from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { Spinner } from '#/components/ui/spinner'
import { FormError } from '#/components/page'
import { api, errorMessage } from '#/lib/api'
import { clearPlan, pendingPlan, takePlanFor } from '#/lib/plan'
import { lastWorkspace, useMe } from '#/lib/workspace'

/**
 * The way in: the last space opened, the first one, or creating one. In the
 * cloud, a plan chosen on Sello's site (`lib/plan.ts`) sends whoever manages
 * that space to buy it in Sello's account center instead.
 */
export const Route = createFileRoute('/_app/')({
	component: Home,
})

function Home() {
	const { config } = Route.useRouteContext()
	const me = useMe()
	const navigate = useNavigate()

	React.useEffect(() => {
		if (!me.data) return
		const spaces = me.data.workspaces
		if (spaces.length === 0) {
			void navigate({ to: '/workspaces/new', replace: true })
			return
		}
		const last = lastWorkspace()
		const target = spaces.find((w) => w.id === last) ?? spaces[0]!
		const open = () =>
			navigate({
				to: '/w/$workspaceId',
				params: { workspaceId: target.id },
				replace: true,
			})
		if (!pendingPlan()) {
			void open()
			return
		}
		if (!config.accountCenter || target.role === 'member') {
			clearPlan()
			void open()
			return
		}
		void api
			.workspace(target.id)
			.then((workspace) => {
				const center = takePlanFor({
					accountCenter: config.accountCenter,
					organization: workspace.sello_workspace_id,
					role: target.role,
				})
				if (center) window.location.assign(center)
				else void open()
			})
			.catch(() => {
				clearPlan()
				void open()
			})
	}, [me.data, navigate, config.accountCenter])

	return (
		<div className="flex min-h-svh items-center justify-center bg-subtle p-6">
			{me.error ? (
				<div className="w-full max-w-sm">
					<FormError message={errorMessage(me.error, 'abrir sus espacios')} />
				</div>
			) : (
				<Spinner />
			)}
		</div>
	)
}
