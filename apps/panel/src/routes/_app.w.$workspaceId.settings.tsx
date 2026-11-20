import * as React from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Button } from '#/components/ui/button'
import {
	Card,
	CardDescription,
	CardHeader,
	CardTitle,
} from '#/components/ui/card'
import { Input } from '#/components/ui/input'
import { Skeleton } from '#/components/ui/skeleton'
import { CopyButton } from '#/components/copy-button'
import { CountrySelect } from '#/components/country-select'
import { FormError, FormField, Mono, Page, PageHeader } from '#/components/page'
import { api, errorMessage, type Workspace } from '#/lib/api'
import { notify } from '#/lib/notify'
import { useWorkspace } from '#/lib/workspace'

export const Route = createFileRoute('/_app/w/$workspaceId/settings')({
	head: () => ({ meta: [{ title: 'General — Sello Sign' }] }),
	component: WorkspaceSettings,
})

function WorkspaceSettings() {
	const { workspaceId } = useWorkspace()
	const workspace = useQuery({
		queryKey: ['workspace', workspaceId],
		queryFn: () => api.workspace(workspaceId),
	})
	return (
		<Page size="narrow">
			<PageHeader
				title="General"
				description="El nombre del espacio, su país principal y su identificador."
			/>
			{workspace.isPending ? (
				<Skeleton className="h-64" />
			) : workspace.error ? (
				<FormError message={errorMessage(workspace.error, 'leer el espacio')} />
			) : (
				<Details key={workspace.data.id} workspace={workspace.data} />
			)}
		</Page>
	)
}

function Details({ workspace }: { workspace: Workspace }) {
	const { canManage } = useWorkspace()
	const { config } = Route.useRouteContext()
	const queryClient = useQueryClient()
	const [name, setName] = React.useState(workspace.name)
	const [country, setCountry] = React.useState(workspace.default_country ?? '')
	const save = useMutation({
		mutationFn: () =>
			api.updateWorkspace(workspace.id, {
				name: name.trim(),
				default_country: country,
			}),
		onSuccess: async () => {
			await queryClient.invalidateQueries({
				queryKey: ['workspace', workspace.id],
			})
			await queryClient.invalidateQueries({ queryKey: ['me'] })
			notify('Cambios guardados.')
		},
	})
	// In the cloud the name is the organization's at Sello.
	const nameEditable = canManage && !config.cloud

	return (
		<>
			<Card>
				<CardHeader>
					<CardTitle>Información general</CardTitle>
					<CardDescription>
						{config.cloud
							? 'El nombre es el de su organización en Sello, y se cambia en su cuenta de Sello.'
							: 'Solo quien administra el espacio lo cambia.'}
					</CardDescription>
				</CardHeader>
				<form
					className="space-y-4"
					onSubmit={(e) => {
						e.preventDefault()
						save.mutate()
					}}
				>
					<FormError
						message={
							save.error
								? errorMessage(save.error, 'guardar los cambios')
								: null
						}
					/>
					<FormField label="Nombre del espacio" htmlFor="name">
						<Input
							id="name"
							required
							disabled={!nameEditable}
							value={name}
							onChange={(e) => setName(e.target.value)}
						/>
					</FormField>
					<FormField label="País principal" htmlFor="country">
						<CountrySelect id="country" value={country} onChange={setCountry} />
					</FormField>
					{canManage ? (
						<Button type="submit" disabled={save.isPending}>
							Guardar cambios
						</Button>
					) : null}
				</form>
			</Card>
			<Card>
				<CardHeader>
					<CardTitle>Identificador del espacio</CardTitle>
					<CardDescription>
						Para que otro espacio le delegue una organización: quien la
						administra lo escribe al crear la delegación.
					</CardDescription>
				</CardHeader>
				<div className="flex flex-wrap items-center gap-3 rounded-panel bg-fill p-4">
					<Mono className="min-w-0 flex-1 break-all text-meta">
						{workspace.id}
					</Mono>
					<CopyButton value={workspace.id} />
				</div>
			</Card>
		</>
	)
}
