import * as React from 'react'
import { Link, createFileRoute, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { BuildingIcon, DownloadIcon, PlusIcon } from 'lucide-react'
import { Badge } from '#/components/ui/badge'
import { Card } from '#/components/ui/card'
import { Empty } from '#/components/empty'
import { Input } from '#/components/ui/input'
import { Skeleton } from '#/components/ui/skeleton'
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from '#/components/ui/table'
import { CountryCode, FormError, Page, PageHeader } from '#/components/page'
import { CertificateBadge } from '#/components/certificate-badge'
import { api, errorMessage, type Relation } from '#/lib/api'
import { buttonVariants } from '#/lib/button-variants'
import { countryName } from '#/lib/countries'
import { useWorkspace } from '#/lib/workspace'
import { SEGMENT, SEGMENTED } from '#/lib/recipes'
import { cn } from '#/lib/utils'

export const Route = createFileRoute('/_app/w/$workspaceId/organizations/')({
	head: () => ({ meta: [{ title: 'Organizaciones — Sello Sign' }] }),
	component: Organizations,
})

function Organizations() {
	const { workspaceId, canManage } = useWorkspace()
	const { config } = Route.useRouteContext()
	const navigate = useNavigate()
	const [relation, setRelation] = React.useState<Relation | 'all'>('all')
	const [search, setSearch] = React.useState('')
	const organizations = useQuery({
		queryKey: ['organizations', workspaceId],
		queryFn: () => api.organizations(workspaceId),
	})

	const term = search.trim().toLowerCase()
	const shown = (organizations.data ?? []).filter(
		(o) =>
			(relation === 'all' || o.relation === relation) &&
			(!term ||
				o.name.toLowerCase().includes(term) ||
				o.tax_id.toLowerCase().includes(term) ||
				countryName(o.country).toLowerCase().includes(term))
	)

	return (
		<Page size="wide">
			<PageHeader
				title="Organizaciones"
				description="Los contribuyentes con los que firma: los de su espacio y los que otro espacio le delegó."
				actions={
					canManage ? (
						<>
							{config.cloud ? (
								<Link
									to="/w/$workspaceId/organizations/claim"
									params={{ workspaceId }}
									className={buttonVariants({
										variant: 'secondary',
										size: 'sm',
									})}
								>
									<DownloadIcon />
									Traer de Sello
								</Link>
							) : null}
							<Link
								to="/w/$workspaceId/organizations/new"
								params={{ workspaceId }}
								className={buttonVariants({ size: 'sm' })}
							>
								<PlusIcon />
								Crear una organización
							</Link>
						</>
					) : null
				}
			/>

			<Card>
				<div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
					<Input
						className="sm:max-w-xs"
						placeholder="Buscar por nombre, NIT o país"
						aria-label="Buscar"
						value={search}
						onChange={(e) => setSearch(e.target.value)}
					/>
					<div className={SEGMENTED} role="radiogroup" aria-label="Relación">
						{(
							[
								['all', 'Todas'],
								['owned', 'Propias'],
								['delegated', 'Delegadas'],
							] as const
						).map(([value, label]) => (
							<button
								key={value}
								type="button"
								role="radio"
								aria-checked={relation === value}
								className={cn(SEGMENT, 'h-7 px-3 text-ui')}
								onClick={() => setRelation(value)}
							>
								{label}
							</button>
						))}
					</div>
				</div>

				{organizations.isPending ? (
					<Skeleton className="h-48" />
				) : organizations.error ? (
					<FormError
						message={errorMessage(
							organizations.error,
							'leer las organizaciones'
						)}
					/>
				) : organizations.data.length === 0 ? (
					<Empty
						icon={<BuildingIcon />}
						title="Sin organizaciones"
						description="Cree la primera organización con la que firmará: su nombre, su país y su NIT."
					/>
				) : shown.length === 0 ? (
					<Empty
						title="Sin resultados"
						description="Ninguna organización coincide con la búsqueda."
					/>
				) : (
					<Table>
						<TableHeader>
							<TableRow>
								<TableHead>Nombre</TableHead>
								<TableHead>Identificación</TableHead>
								<TableHead>País</TableHead>
								<TableHead>Prueba</TableHead>
								<TableHead>Producción</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{shown.map((o) => (
								<TableRow
									key={o.id}
									className="cursor-pointer hover:bg-fill"
									onClick={() =>
										navigate({
											to: '/w/$workspaceId/organizations/$organizationId',
											params: { workspaceId, organizationId: o.id },
										})
									}
								>
									<TableCell>
										<Link
											to="/w/$workspaceId/organizations/$organizationId"
											params={{ workspaceId, organizationId: o.id }}
											className="inline-flex items-center gap-2 font-medium"
											onClick={(e) => e.stopPropagation()}
										>
											{o.name}
											{o.relation === 'delegated' ? (
												<Badge variant="outline">Delegada</Badge>
											) : null}
										</Link>
									</TableCell>
									<TableCell className="font-mono text-label">
										{o.tax_id}
									</TableCell>
									<TableCell>
										<span className="inline-flex items-center gap-2">
											<CountryCode code={o.country} />
											<span className="text-muted-foreground">
												{countryName(o.country)}
											</span>
										</span>
									</TableCell>
									<TableCell>
										<CertificateBadge
											certificate={o.certificates?.test ?? null}
										/>
									</TableCell>
									<TableCell>
										<CertificateBadge
											certificate={o.certificates?.prod ?? null}
										/>
									</TableCell>
								</TableRow>
							))}
						</TableBody>
					</Table>
				)}
			</Card>
		</Page>
	)
}
