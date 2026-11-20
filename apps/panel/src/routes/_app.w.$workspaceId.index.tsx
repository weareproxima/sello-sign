import { Link, createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { AlertTriangleIcon, BuildingIcon, CircleCheckIcon } from 'lucide-react'
import { Badge } from '#/components/ui/badge'
import {
	Card,
	CardDescription,
	CardHeader,
	CardTitle,
} from '#/components/ui/card'
import { Skeleton } from '#/components/ui/skeleton'
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from '#/components/ui/table'
import { Empty } from '#/components/empty'
import { FormError, Page, PageHeader } from '#/components/page'
import { api, errorMessage, type Certificate } from '#/lib/api'
import { buttonVariants } from '#/lib/button-variants'
import {
	ENVIRONMENT_LABELS,
	daysUntil,
	formatDate,
	formatDateTime,
	formatNumber,
} from '#/lib/format'
import { useWorkspace } from '#/lib/workspace'
import { cn } from '#/lib/utils'

export const Route = createFileRoute('/_app/w/$workspaceId/')({
	head: () => ({ meta: [{ title: 'Resumen — Sello Sign' }] }),
	component: Dashboard,
})

/** A certificate that expires within this many days is «por vencer». */
const SOON_DAYS = 30

function Dashboard() {
	const { workspaceId, workspace } = useWorkspace()
	const usage = useQuery({
		queryKey: ['usage', workspaceId],
		queryFn: () => api.usage(workspaceId),
	})
	const organizations = useQuery({
		queryKey: ['organizations', workspaceId],
		queryFn: () => api.organizations(workspaceId),
	})
	const recent = useQuery({
		queryKey: ['signatures', workspaceId, 'recent'],
		queryFn: () => api.signatures(workspaceId, { limit: 8 }),
	})

	const month = new Date().toISOString().slice(0, 7)
	const used = (env: 'prod' | 'test') =>
		usage.data?.items.find((u) => u.environment === env && u.month === month)
			?.signatures ?? 0
	const limits = usage.data?.limits

	const certificates = (organizations.data ?? []).flatMap((o) =>
		(['prod', 'test'] as const)
			.map((env) => o.certificates?.[env])
			.filter((c): c is Certificate => !!c)
			.map((c) => ({ organization: o, certificate: c }))
	)
	const atRisk = certificates.filter(
		({ certificate }) =>
			certificate.not_after !== null &&
			daysUntil(certificate.not_after) <= SOON_DAYS
	)

	return (
		<Page size="wide">
			<PageHeader
				title="Resumen"
				description={`La actividad de firma, el uso del plan y los certificados de ${workspace?.name ?? 'su espacio'}.`}
			/>

			<div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
				<UsageCard
					label="Firmas de producción este mes"
					used={used('prod')}
					limit={limits?.prod_signatures ?? null}
					loading={usage.isPending}
				/>
				<UsageCard
					label="Firmas de prueba este mes"
					used={used('test')}
					limit={limits?.test_signatures ?? null}
					loading={usage.isPending}
				/>
				<Card className="gap-2">
					<CardDescription>Organizaciones</CardDescription>
					{organizations.isPending ? (
						<Skeleton className="h-8 w-16" />
					) : (
						<p className="text-h1">
							{formatNumber(organizations.data?.length ?? 0)}
						</p>
					)}
					<p className="text-meta text-muted-foreground">
						{formatNumber(
							(organizations.data ?? []).filter(
								(o) => o.relation === 'delegated'
							).length
						)}{' '}
						por delegación
					</p>
				</Card>
				<Card className="gap-2">
					<CardDescription>Certificados por vencer</CardDescription>
					{organizations.isPending ? (
						<Skeleton className="h-8 w-16" />
					) : (
						<p className={cn('text-h1', atRisk.length > 0 && 'text-warn')}>
							{formatNumber(atRisk.length)}
						</p>
					)}
					<p className="text-meta text-muted-foreground">
						de {formatNumber(certificates.length)} activos, en {SOON_DAYS} días
					</p>
				</Card>
			</div>

			{usage.error ? (
				<FormError message={errorMessage(usage.error, 'leer el uso')} />
			) : null}

			{atRisk.length > 0 ? (
				<Card>
					<CardHeader>
						<CardTitle>Certificados por vencer</CardTitle>
						<CardDescription>
							Renuévelos antes de que venzan para seguir firmando.
						</CardDescription>
					</CardHeader>
					<ul className="space-y-2">
						{atRisk.map(({ organization, certificate }) => {
							const days = daysUntil(certificate.not_after!)
							return (
								<li
									key={certificate.id}
									className="flex flex-wrap items-center justify-between gap-3 rounded-panel bg-fill p-4"
								>
									<div className="flex min-w-0 items-center gap-3">
										<AlertTriangleIcon
											className={cn(
												'size-4 shrink-0',
												days < 0 ? 'text-bad' : 'text-warn'
											)}
										/>
										<div className="min-w-0">
											<p className="truncate text-ui font-medium">
												{organization.name}
											</p>
											<p className="text-meta text-muted-foreground">
												{ENVIRONMENT_LABELS[certificate.environment]} ·{' '}
												{days < 0
													? `Venció el ${formatDate(certificate.not_after)}`
													: `Vence el ${formatDate(certificate.not_after)}`}
											</p>
										</div>
									</div>
									<Link
										to="/w/$workspaceId/organizations/$organizationId"
										params={{ workspaceId, organizationId: organization.id }}
										className={buttonVariants({
											variant: 'outline',
											size: 'sm',
										})}
									>
										Ver certificados
									</Link>
								</li>
							)
						})}
					</ul>
				</Card>
			) : null}

			<Card>
				<CardHeader>
					<CardTitle>Actividad reciente</CardTitle>
					<CardDescription>Las últimas firmas del espacio.</CardDescription>
				</CardHeader>
				{recent.isPending ? (
					<Skeleton className="h-40" />
				) : recent.error ? (
					<FormError
						message={errorMessage(recent.error, 'leer el registro de firmas')}
					/>
				) : recent.data.items.length === 0 ? (
					organizations.data?.length === 0 ? (
						<Empty
							icon={<BuildingIcon />}
							title="Sin organizaciones"
							description="Cree la primera organización con la que firmará, y suba su certificado."
						>
							<Link
								to="/w/$workspaceId/organizations/new"
								params={{ workspaceId }}
								className={buttonVariants({ size: 'sm' })}
							>
								Crear una organización
							</Link>
						</Empty>
					) : (
						<Empty
							title="Sin firmas todavía"
							description="Las firmas de sus organizaciones aparecerán aquí."
						/>
					)
				) : (
					<Table>
						<TableHeader>
							<TableRow>
								<TableHead>Organización</TableHead>
								<TableHead>Documento</TableHead>
								<TableHead>Ambiente</TableHead>
								<TableHead className="text-right">Fecha</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{recent.data.items.map((entry) => (
								<TableRow key={entry.id}>
									<TableCell className="font-medium">
										<span className="inline-flex items-center gap-2">
											<CircleCheckIcon
												className="size-4 text-ok"
												aria-label="Firmado"
											/>
											{entry.organization_name}
										</span>
									</TableCell>
									<TableCell className="font-mono text-label">
										{entry.document_type ?? '—'}
									</TableCell>
									<TableCell>
										<Badge
											variant={
												entry.environment === 'prod' ? 'default' : 'secondary'
											}
										>
											{ENVIRONMENT_LABELS[entry.environment]}
										</Badge>
									</TableCell>
									<TableCell className="text-right text-meta text-muted-foreground">
										{formatDateTime(entry.created_at)}
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

function UsageCard({
	label,
	used,
	limit,
	loading,
}: {
	label: string
	used: number
	limit: number | null
	loading: boolean
}) {
	const ratio = limit ? Math.min(used / limit, 1) : 0
	const exhausted = limit !== null && used >= limit
	return (
		<Card className="gap-2">
			<CardDescription>{label}</CardDescription>
			{loading ? (
				<Skeleton className="h-8 w-24" />
			) : (
				<p className={cn('text-h1', exhausted && 'text-bad')}>
					{formatNumber(used)}
				</p>
			)}
			<p className="text-meta text-muted-foreground">
				{limit === null ? 'Sin tope' : `de ${formatNumber(limit)} del plan`}
			</p>
			{limit !== null ? (
				<div className="h-1.5 overflow-hidden rounded-full bg-fill" aria-hidden>
					<div
						className={cn(
							'h-full rounded-full',
							exhausted ? 'bg-bad' : 'bg-primary'
						)}
						style={{ width: `${ratio * 100}%` }}
					/>
				</div>
			) : null}
		</Card>
	)
}
