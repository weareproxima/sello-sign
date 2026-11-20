import * as React from 'react'
import { Link, createFileRoute, useNavigate } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { DownloadIcon } from 'lucide-react'
import { Button } from '#/components/ui/button'
import {
	Card,
	CardDescription,
	CardHeader,
	CardTitle,
} from '#/components/ui/card'
import { Checkbox } from '#/components/ui/checkbox'
import { Empty } from '#/components/empty'
import { ActionError } from '#/components/limit-alert'
import { Skeleton } from '#/components/ui/skeleton'
import {
	CountryCode,
	FormError,
	Mono,
	Page,
	PageHeader,
} from '#/components/page'
import { api, errorMessage } from '#/lib/api'
import { buttonVariants } from '#/lib/button-variants'
import { countryName } from '#/lib/countries'
import { notify } from '#/lib/notify'
import { useWorkspace } from '#/lib/workspace'

/**
 * «Traer sus organizaciones» (Sello's cloud): the organizations of Sello's
 * space linked to IdP organizations the person manages come to this space
 * with their certificates and history, leaving Sello's platform a locked
 * delegation (../sello/docs/sello-sign.md › «Usar en Sello Sign»).
 */
export const Route = createFileRoute(
	'/_app/w/$workspaceId/organizations/claim'
)({
	head: () => ({ meta: [{ title: 'Traer sus organizaciones — Sello Sign' }] }),
	component: Claim,
})

function Claim() {
	const { workspaceId, workspace } = useWorkspace()
	const navigate = useNavigate()
	const queryClient = useQueryClient()
	const claimable = useQuery({
		queryKey: ['claimable', workspaceId],
		queryFn: () => api.claimableOrganizations(workspaceId),
	})
	const [selected, setSelected] = React.useState<Set<string> | null>(null)

	// The space's own organization comes marked.
	const chosen =
		selected ??
		new Set(
			(claimable.data ?? [])
				.filter((o) => workspace && o.name === workspace.name)
				.map((o) => o.id)
		)

	const claim = useMutation({
		mutationFn: () => api.claimOrganizations(workspaceId, [...chosen]),
		onSuccess: async () => {
			await queryClient.invalidateQueries({
				queryKey: ['organizations', workspaceId],
			})
			await queryClient.invalidateQueries({
				queryKey: ['claimable', workspaceId],
			})
			notify(
				chosen.size === 1 ? 'Organización traída.' : 'Organizaciones traídas.'
			)
			await navigate({
				to: '/w/$workspaceId/organizations',
				params: { workspaceId },
			})
		},
	})

	function toggle(id: string, on: boolean) {
		const next = new Set(chosen)
		if (on) next.add(id)
		else next.delete(id)
		setSelected(next)
	}

	return (
		<Page size="narrow">
			<PageHeader
				title="Traer sus organizaciones"
				description="Las organizaciones que ya emiten con Sello y que usted administra. Pasan a este espacio con sus certificados y su historial."
			/>
			<Card>
				<CardHeader>
					<CardTitle>Elija las que trae</CardTitle>
					<CardDescription>
						La plataforma de Sello sigue firmando con ellas por una delegación,
						que se ve en cada una y que solo Sello quita.
					</CardDescription>
				</CardHeader>
				<ActionError error={claim.error} action="traer las organizaciones" />
				{claimable.isPending ? (
					<Skeleton className="h-32" />
				) : claimable.error ? (
					<FormError
						message={errorMessage(
							claimable.error,
							'leer las organizaciones de Sello'
						)}
					/>
				) : claimable.data.length === 0 ? (
					<Empty
						icon={<DownloadIcon />}
						title="Nada que traer"
						description="Ninguna organización que usted administra emite con Sello todavía. Puede crear las suyas aquí."
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
					<ul className="space-y-2">
						{claimable.data.map((o) => (
							<li key={o.id}>
								<label className="flex cursor-pointer items-center gap-4 rounded-panel bg-fill p-4 transition-colors duration-fast hover:bg-fill-hover">
									<Checkbox
										checked={chosen.has(o.id)}
										onCheckedChange={(v) => toggle(o.id, v === true)}
									/>
									<span className="min-w-0 flex-1">
										<span className="block truncate text-ui font-medium">
											{o.name}
										</span>
										<span className="flex items-center gap-2 text-meta text-muted-foreground">
											<CountryCode code={o.country} />
											{countryName(o.country)} · <Mono>{o.tax_id}</Mono>
										</span>
									</span>
								</label>
							</li>
						))}
					</ul>
				)}
				<div className="flex flex-wrap gap-2">
					{claimable.data && claimable.data.length > 0 ? (
						<Button
							disabled={chosen.size === 0 || claim.isPending}
							onClick={() => claim.mutate()}
						>
							Traer{' '}
							{chosen.size === 1
								? 'la organización'
								: `${chosen.size} organizaciones`}
						</Button>
					) : null}
					<Link
						to="/w/$workspaceId"
						params={{ workspaceId }}
						className={buttonVariants({ variant: 'secondary' })}
					>
						Ahora no
					</Link>
				</div>
			</Card>
		</Page>
	)
}
