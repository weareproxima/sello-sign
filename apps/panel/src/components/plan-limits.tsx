import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { ExternalLinkIcon } from 'lucide-react'
import {
	Card,
	CardDescription,
	CardHeader,
	CardTitle,
} from '#/components/ui/card'
import { usePlanPage } from '#/components/limit-alert'
import type { Limits } from '#/lib/api'
import { buttonVariants } from '#/lib/button-variants'
import { formatNumber } from '#/lib/format'
import { effectiveRate, planLabel } from '#/lib/limits'
import { TEXT_LINK } from '#/lib/recipes'
import { cn } from '#/lib/utils'

/**
 * The plan's limits besides the month's signatures (which the usage cards
 * show with their cap): signatures a minute (the plan's ceiling, or the
 * space's own below it), organizations, members and how long the signing
 * log keeps what was signed. Self-hosted, the ones the installation sets.
 */
export function PlanLimits({
	workspaceId,
	plan,
	limits,
	ownRate,
	ownedOrganizations,
	canManage,
}: {
	workspaceId: string
	plan: string | null
	limits: Limits
	/** The space's own signatures a minute; `null` is the plan's. */
	ownRate: number | null
	/** How many organizations the space owns (delegated ones don't count). */
	ownedOrganizations: number | null
	canManage: boolean
}) {
	const planPage = usePlanPage()
	const ceiling = limits.requests_per_minute
	const rate = effectiveRate(ceiling, ownRate)
	const name = planLabel(plan)
	// Self-hosted, the limits are the installation's, not a plan's.
	const whose = plan ? 'el plan' : 'la instalación'
	const none =
		rate === null &&
		limits.organizations === null &&
		limits.members === null &&
		limits.retention_days === null

	return (
		<Card>
			<CardHeader>
				<CardTitle>Límites del plan</CardTitle>
				<CardDescription>
					{none
						? 'Esta instalación no le pone límites al espacio.'
						: name
							? `Lo que permite el plan ${name} del espacio, además de las firmas del mes. Sello Sign rechaza la firma que pase un límite.`
							: 'Los que esta instalación le pone al espacio, además de las firmas del mes. Sello Sign rechaza la firma que pase un límite.'}
				</CardDescription>
			</CardHeader>
			{none ? null : (
				<dl className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
					<Limit
						term="Firmas por minuto"
						value={rate === null ? 'Sin límite' : formatNumber(rate)}
					>
						{ownRate !== null && ceiling !== null && ownRate < ceiling
							? `Las que se puso el espacio; ${whose} permite ${formatNumber(ceiling)}.`
							: ownRate !== null && ceiling === null
								? 'Las que se puso el espacio.'
								: ceiling !== null
									? `Las ${plan ? 'del plan' : 'de la instalación'}, para todo el espacio.`
									: `Ni ${whose} ni el espacio ponen un límite.`}
						{canManage ? (
							<>
								{' '}
								<Link
									to="/w/$workspaceId/settings"
									params={{ workspaceId }}
									hash="signing-rate"
									className={TEXT_LINK}
								>
									Cambiar
								</Link>
							</>
						) : null}
					</Limit>
					<Limit
						term="Organizaciones propias"
						value={
							limits.organizations === null
								? 'Sin límite'
								: formatNumber(limits.organizations)
						}
					>
						{ownedOrganizations === null
							? 'Las delegadas no cuentan.'
							: `Tiene ${formatNumber(ownedOrganizations)}; las delegadas no cuentan.`}
					</Limit>
					<Limit
						term="Miembros"
						value={
							limits.members === null
								? 'Sin límite'
								: formatNumber(limits.members)
						}
					>
						Las personas que el espacio admite.
					</Limit>
					<Limit
						term="Registro de firmas"
						value={
							limits.retention_days === null
								? 'Sin plazo'
								: limits.retention_days === 1
									? '1 día'
									: `${formatNumber(limits.retention_days)} días`
						}
					>
						{limits.retention_days === null
							? 'Lo firmado se queda en el registro.'
							: 'Después, lo firmado se borra del registro.'}
					</Limit>
				</dl>
			)}
			{planPage.url ? (
				// Below the limits, not as the header's action: on a phone that
				// would squeeze the description into a narrow column.
				<a
					href={planPage.url}
					className={cn(
						buttonVariants({ variant: 'secondary', size: 'sm' }),
						'w-fit'
					)}
				>
					<ExternalLinkIcon />
					Cambiar el plan
				</a>
			) : null}
		</Card>
	)
}

function Limit({
	term,
	value,
	children,
}: {
	term: string
	value: string
	children: ReactNode
}) {
	return (
		<div className="rounded-panel bg-fill px-4 py-3">
			<dt className="text-meta text-muted-foreground">{term}</dt>
			<dd className="mt-1 text-h3 tabular-nums">{value}</dd>
			<dd className="mt-1 text-meta text-muted-foreground">{children}</dd>
		</div>
	)
}
