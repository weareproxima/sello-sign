import * as React from 'react'
import { Link, createFileRoute, useNavigate } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Building2Icon, RefreshCwIcon } from 'lucide-react'
import { Badge } from '#/components/ui/badge'
import { Button } from '#/components/ui/button'
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
} from '#/components/ui/card'
import { Input } from '#/components/ui/input'
import { Skeleton } from '#/components/ui/skeleton'
import { CountrySelect } from '#/components/country-select'
import { Logo } from '#/components/logo'
import { FormError, FormField } from '#/components/page'
import { api, errorMessage, type SelloOrganization } from '#/lib/api'
import { authClient } from '#/lib/auth-client'
import { countryName } from '#/lib/countries'
import { ROLE_LABELS } from '#/lib/format'
import { takePlanFor } from '#/lib/plan'
import { useMe } from '#/lib/workspace'

/**
 * A new space. Self-hosted, a name and a country. In Sello's cloud a space is
 * one of the person's organizations at Sello: they pick it, and then bring
 * their organizations from Sello's space («Traer sus organizaciones»).
 */
export const Route = createFileRoute('/_app/workspaces/new')({
	head: () => ({ meta: [{ title: 'Nuevo espacio — Sello Sign' }] }),
	component: NewWorkspace,
})

function NewWorkspace() {
	const { config } = Route.useRouteContext()
	const me = useMe()
	const hasSpaces = (me.data?.workspaces.length ?? 0) > 0

	return (
		<div className="flex min-h-svh flex-col items-center bg-subtle px-4 py-10 sm:py-16">
			<div className="w-full max-w-xl space-y-8">
				<div className="flex items-center justify-between">
					<Logo />
					{hasSpaces ? (
						<Link
							to="/"
							className="text-ui text-muted-foreground transition-colors duration-fast hover:text-foreground"
						>
							Cancelar
						</Link>
					) : null}
				</div>
				{config.cloud ? (
					<CloudPicker
						providerId={config.oidc?.providerId}
						accountCenter={config.accountCenter}
					/>
				) : (
					<CreateForm first={!hasSpaces} />
				)}
			</div>
		</div>
	)
}

function CreateForm({ first }: { first: boolean }) {
	const navigate = useNavigate()
	const queryClient = useQueryClient()
	const [name, setName] = React.useState('')
	const [country, setCountry] = React.useState('')
	const create = useMutation({
		mutationFn: () =>
			api.createWorkspace({ name: name.trim(), default_country: country }),
		onSuccess: async (workspace) => {
			await queryClient.invalidateQueries({ queryKey: ['me'] })
			await navigate({
				to: '/w/$workspaceId',
				params: { workspaceId: workspace.id },
			})
		},
	})

	return (
		<Card>
			<CardHeader>
				<h1 className="text-h1">
					{first ? 'Cree su primer espacio' : 'Crear un espacio'}
				</h1>
				<CardDescription>
					Un espacio guarda las organizaciones con las que firma, sus
					certificados, sus claves y su registro de firmas. Puede cambiarlo todo
					después.
				</CardDescription>
			</CardHeader>
			<CardContent>
				<form
					className="space-y-4"
					onSubmit={(e) => {
						e.preventDefault()
						create.mutate()
					}}
				>
					<FormError
						message={
							create.error
								? errorMessage(create.error, 'crear el espacio')
								: null
						}
					/>
					<FormField label="Nombre del espacio" htmlFor="name">
						<Input
							id="name"
							required
							minLength={2}
							maxLength={100}
							autoFocus
							placeholder="Acme, S.A."
							value={name}
							onChange={(e) => setName(e.target.value)}
						/>
					</FormField>
					<FormField
						label="País principal"
						htmlFor="country"
						help="El país donde están la mayoría de sus organizaciones. Cada organización tiene el suyo."
					>
						<CountrySelect id="country" value={country} onChange={setCountry} />
					</FormField>
					<Button type="submit" disabled={create.isPending}>
						Crear el espacio
					</Button>
				</form>
			</CardContent>
		</Card>
	)
}

/**
 * A plan chosen on Sello's site (`lib/plan.ts`): once the space is open, the
 * account center's page to buy it, for whoever manages the organization.
 */
function planPage(
	accountCenter: string | null,
	organization: SelloOrganization
): string | null {
	return takePlanFor({
		accountCenter,
		organization: organization.id,
		role: organization.role,
	})
}

function CloudPicker({
	providerId,
	accountCenter,
}: {
	providerId: string | undefined
	accountCenter: string | null
}) {
	const navigate = useNavigate()
	const queryClient = useQueryClient()
	const organizations = useQuery({
		queryKey: ['sello-organizations'],
		queryFn: () => api.selloOrganizations(),
	})
	const open = useMutation({
		mutationFn: (organization: SelloOrganization) =>
			api.createWorkspace({ sello_workspace_id: organization.id }),
		onSuccess: async (workspace, organization) => {
			await queryClient.invalidateQueries({ queryKey: ['me'] })
			const center = planPage(accountCenter, organization)
			if (center) {
				window.location.assign(center)
				return
			}
			await navigate({
				to: '/w/$workspaceId/organizations/claim',
				params: { workspaceId: workspace.id },
			})
		},
	})
	const shown = (organizations.data ?? []).filter((o) => o.status !== 'closed')

	// Sello's ID token lists the organizations; signing in again reads it anew.
	async function refresh() {
		if (!providerId) return
		await authClient.signIn.social({
			provider: providerId,
			callbackURL: '/workspaces/new',
		})
	}

	return (
		<section className="space-y-6" aria-labelledby="sello-organizations">
			<div className="space-y-1">
				<h1 id="sello-organizations" className="text-h1">
					Elija su organización
				</h1>
				<p className="text-ui text-pretty text-muted-foreground">
					Los espacios de Sello Sign son las organizaciones de su cuenta de
					Sello, con su plan y sus miembros. Abra una; si todavía no usa Sello
					Sign, quien la administra puede empezar a usarlo.
				</p>
			</div>

			<FormError
				message={
					organizations.error
						? errorMessage(
								organizations.error,
								'leer sus organizaciones de Sello'
							)
						: open.error
							? errorMessage(open.error, 'abrir el espacio')
							: null
				}
			/>

			{organizations.isPending ? (
				<div className="space-y-2">
					<Skeleton className="h-16" />
					<Skeleton className="h-16" />
				</div>
			) : shown.length === 0 ? (
				<div className="rounded-card bg-surface p-6 text-ui text-pretty text-muted-foreground shadow-raised">
					Todavía no está en ninguna organización de Sello. Cree una en su
					cuenta de Sello y vuelva aquí.
				</div>
			) : (
				<ul className="space-y-2">
					{shown.map((organization) => (
						<li
							key={organization.id}
							className="flex flex-wrap items-center justify-between gap-3 rounded-panel bg-surface p-4 shadow-raised"
						>
							<div className="flex min-w-0 items-center gap-3">
								<Building2Icon className="size-4 shrink-0 text-muted-foreground" />
								<div className="min-w-0">
									<p className="truncate text-ui font-medium">
										{organization.name}
									</p>
									<p className="text-meta text-muted-foreground">
										{organization.country
											? countryName(organization.country)
											: ''}
									</p>
								</div>
							</div>
							<div className="flex items-center gap-2">
								<Badge variant="secondary">
									{ROLE_LABELS[organization.role]}
								</Badge>
								{organization.status === 'open' && organization.workspace_id ? (
									<Button
										variant="secondary"
										size="sm"
										onClick={() => {
											const center = planPage(accountCenter, organization)
											if (center) {
												window.location.assign(center)
												return
											}
											void navigate({
												to: '/w/$workspaceId',
												params: { workspaceId: organization.workspace_id! },
											})
										}}
									>
										Abrir
									</Button>
								) : organization.status === 'available' ? (
									<Button
										size="sm"
										disabled={open.isPending}
										onClick={() => open.mutate(organization)}
									>
										Usar en Sello Sign
									</Button>
								) : null}
							</div>
							{organization.status === 'ask' ? (
								<p className="w-full text-meta text-pretty text-muted-foreground">
									Esta organización todavía no usa Sello Sign. Pida a quien la
									administra que lo empiece.
								</p>
							) : organization.status === 'full' ? (
								<p className="w-full text-meta text-pretty text-muted-foreground">
									El plan de esta organización ya no admite más miembros.
								</p>
							) : null}
						</li>
					))}
				</ul>
			)}

			{providerId ? (
				<div className="flex flex-wrap items-center gap-2">
					<span className="text-meta text-muted-foreground">
						¿No ve una organización?
					</span>
					<Button variant="ghost" size="sm" onClick={refresh}>
						<RefreshCwIcon />
						Actualizar la lista
					</Button>
				</div>
			) : null}
		</section>
	)
}
