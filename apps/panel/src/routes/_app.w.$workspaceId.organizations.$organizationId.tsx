import { Link, Outlet, createFileRoute } from '@tanstack/react-router'
import { Badge } from '#/components/ui/badge'
import { Skeleton } from '#/components/ui/skeleton'
import {
	CountryCode,
	FormError,
	Mono,
	Page,
	PageHeader,
} from '#/components/page'
import { errorMessage } from '#/lib/api'
import { countryName, taxIdLabel } from '#/lib/countries'
import { useOrganization } from '#/lib/organization'

/** An organization: its header and its tabs (certificates, delegations, signing). */
export const Route = createFileRoute(
	'/_app/w/$workspaceId/organizations/$organizationId'
)({
	component: OrganizationLayout,
})

const TAB =
	'inline-flex h-control-sm items-center rounded-full px-3 text-ui text-muted-foreground transition-colors duration-fast hover:bg-fill hover:text-foreground'

function OrganizationLayout() {
	const {
		workspaceId,
		organizationId,
		data: organization,
		isPending,
		error,
	} = useOrganization()
	const params = { workspaceId, organizationId }

	return (
		<Page size="wide">
			{isPending ? (
				<Skeleton className="h-14 w-80" />
			) : error ? (
				<FormError message={errorMessage(error, 'abrir la organización')} />
			) : (
				<PageHeader
					title={organization.name}
					back={{
						label: 'Volver a las organizaciones',
						to: '/w/$workspaceId/organizations',
						params: { workspaceId },
					}}
					meta={
						<>
							<CountryCode code={organization.country} />
							<span className="text-meta text-muted-foreground">
								{countryName(organization.country)}
							</span>
							<span className="text-meta text-muted-foreground">·</span>
							<span className="text-meta text-muted-foreground">
								{taxIdLabel(organization.country)}
							</span>
							<Mono className="text-meta">{organization.tax_id}</Mono>
							{organization.relation === 'delegated' ? (
								<Badge variant="outline">Delegada a este espacio</Badge>
							) : null}
							{organization.sello_workspace_id ? (
								<Badge variant="secondary">Vinculada a Sello</Badge>
							) : null}
						</>
					}
				/>
			)}
			<nav
				className="flex flex-wrap gap-1"
				aria-label="Secciones de la organización"
			>
				<Link
					to="/w/$workspaceId/organizations/$organizationId"
					params={params}
					activeOptions={{ exact: true }}
					activeProps={{
						className: 'bg-selected text-foreground hover:bg-selected',
					}}
					className={TAB}
				>
					Certificados
				</Link>
				<Link
					to="/w/$workspaceId/organizations/$organizationId/delegations"
					params={params}
					activeProps={{
						className: 'bg-selected text-foreground hover:bg-selected',
					}}
					className={TAB}
				>
					Delegaciones
				</Link>
				<Link
					to="/w/$workspaceId/organizations/$organizationId/sign"
					params={params}
					activeProps={{
						className: 'bg-selected text-foreground hover:bg-selected',
					}}
					className={TAB}
				>
					Firmar de prueba
				</Link>
				<Link
					to="/w/$workspaceId/organizations/$organizationId/settings"
					params={params}
					activeProps={{
						className: 'bg-selected text-foreground hover:bg-selected',
					}}
					className={TAB}
				>
					Datos
				</Link>
			</nav>
			<Outlet />
		</Page>
	)
}
