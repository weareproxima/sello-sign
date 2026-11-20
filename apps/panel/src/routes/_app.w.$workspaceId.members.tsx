import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Badge } from '#/components/ui/badge'
import { Card } from '#/components/ui/card'
import { Skeleton } from '#/components/ui/skeleton'
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from '#/components/ui/table'
import { FormError, Page, PageHeader } from '#/components/page'
import { api, errorMessage } from '#/lib/api'
import { ROLE_LABELS } from '#/lib/format'
import { useMe, useWorkspace } from '#/lib/workspace'

export const Route = createFileRoute('/_app/w/$workspaceId/members')({
	head: () => ({ meta: [{ title: 'Miembros — Sello Sign' }] }),
	component: Members,
})

function Members() {
	const { workspaceId } = useWorkspace()
	const { config } = Route.useRouteContext()
	const me = useMe()
	const members = useQuery({
		queryKey: ['members', workspaceId],
		queryFn: () => api.members(workspaceId),
	})
	return (
		<Page>
			<PageHeader
				title="Miembros"
				description={
					config.cloud
						? 'Las personas de su organización en Sello. Se invitan y cambian de rol en su cuenta de Sello.'
						: 'Quién tiene acceso al espacio y con qué rol.'
				}
			/>
			<Card>
				{members.isPending ? (
					<Skeleton className="h-40" />
				) : members.error ? (
					<FormError
						message={errorMessage(members.error, 'leer los miembros')}
					/>
				) : (
					<Table>
						<TableHeader>
							<TableRow>
								<TableHead>Nombre</TableHead>
								<TableHead>Correo</TableHead>
								<TableHead>Rol</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{members.data.map((m) => (
								<TableRow key={m.user_id}>
									<TableCell className="font-medium">
										{m.name ?? m.email ?? m.user_id}
										{m.user_id === me.data?.user.id ? (
											<span className="ms-2 text-meta text-muted-foreground">
												(usted)
											</span>
										) : null}
									</TableCell>
									<TableCell className="text-muted-foreground">
										{m.email ?? '—'}
									</TableCell>
									<TableCell>
										<Badge variant="secondary">{ROLE_LABELS[m.role]}</Badge>
									</TableCell>
								</TableRow>
							))}
						</TableBody>
					</Table>
				)}
			</Card>
			{!config.cloud ? (
				<p className="text-meta text-muted-foreground">
					Las invitaciones por correo llegan en una próxima versión.
				</p>
			) : null}
		</Page>
	)
}
