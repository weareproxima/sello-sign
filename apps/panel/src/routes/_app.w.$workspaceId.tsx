import * as React from 'react'
import { Link, Outlet, createFileRoute } from '@tanstack/react-router'
import { AppShell } from '#/components/app-shell'
import { FormError } from '#/components/page'
import { Spinner } from '#/components/ui/spinner'
import { buttonVariants } from '#/lib/button-variants'
import { errorMessage } from '#/lib/api'
import { rememberWorkspace, useMe } from '#/lib/workspace'

/** A space: everything under it is the space's (`/w/<id>/…`). */
export const Route = createFileRoute('/_app/w/$workspaceId')({
	component: WorkspaceLayout,
})

function WorkspaceLayout() {
	const { workspaceId } = Route.useParams()
	const { session, config } = Route.useRouteContext()
	const me = useMe()
	const current = me.data?.workspaces.find((w) => w.id === workspaceId) ?? null

	React.useEffect(() => {
		if (current) rememberWorkspace(current.id)
	}, [current])

	const body = me.isPending ? (
		<div className="flex flex-1 items-center justify-center">
			<Spinner />
		</div>
	) : me.error ? (
		<FormError message={errorMessage(me.error, 'abrir el espacio')} />
	) : !current ? (
		<div className="flex flex-1 flex-col items-center justify-center gap-4 text-center">
			<h1 className="text-h1">No tiene acceso a este espacio.</h1>
			<p className="text-ui text-muted-foreground">
				Puede que no exista o que ya no sea miembro.
			</p>
			<Link to="/" className={buttonVariants({ variant: 'secondary' })}>
				Volver al inicio
			</Link>
		</div>
	) : (
		<Outlet />
	)

	return (
		<AppShell
			workspaceId={workspaceId}
			current={current}
			workspaces={me.data?.workspaces ?? []}
			canManage={current?.role === 'owner' || current?.role === 'admin'}
			user={{ name: session.user.name, email: session.user.email }}
			cloud={config.cloud}
		>
			{body}
		</AppShell>
	)
}
