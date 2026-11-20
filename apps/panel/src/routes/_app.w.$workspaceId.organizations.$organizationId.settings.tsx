import * as React from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Button } from '#/components/ui/button'
import {
	Card,
	CardDescription,
	CardHeader,
	CardTitle,
} from '#/components/ui/card'
import { Input } from '#/components/ui/input'
import { Confirm } from '#/components/confirm'
import { FormError, FormField, Mono } from '#/components/page'
import { api, errorMessage, type Organization } from '#/lib/api'
import { notify } from '#/lib/notify'
import { organizationKey, useOrganization } from '#/lib/organization'
import { useWorkspace } from '#/lib/workspace'

export const Route = createFileRoute(
	'/_app/w/$workspaceId/organizations/$organizationId/settings'
)({
	head: () => ({ meta: [{ title: 'Datos de la organización — Sello Sign' }] }),
	component: OrganizationSettings,
})

function OrganizationSettings() {
	const { data: organization } = useOrganization()
	if (!organization) return null
	return <Settings key={organization.id} organization={organization} />
}

function Settings({ organization }: { organization: Organization }) {
	const { workspaceId, canManage } = useWorkspace()
	const navigate = useNavigate()
	const queryClient = useQueryClient()
	const editable = canManage && organization.relation === 'owned'
	const [name, setName] = React.useState(organization.name)
	const [email, setEmail] = React.useState(organization.contact_email ?? '')
	const [deleting, setDeleting] = React.useState(false)

	const save = useMutation({
		mutationFn: () =>
			api.updateOrganization(workspaceId, organization.id, {
				name: name.trim(),
				contact_email: email.trim() || null,
			}),
		onSuccess: async () => {
			await queryClient.invalidateQueries({
				queryKey: organizationKey(workspaceId, organization.id),
			})
			await queryClient.invalidateQueries({
				queryKey: ['organizations', workspaceId],
			})
			notify('Cambios guardados.')
		},
	})
	const remove = useMutation({
		mutationFn: () => api.deleteOrganization(workspaceId, organization.id),
		onSuccess: async () => {
			await queryClient.invalidateQueries({
				queryKey: ['organizations', workspaceId],
			})
			notify('Organización borrada.')
			await navigate({
				to: '/w/$workspaceId/organizations',
				params: { workspaceId },
			})
		},
		onError: (error) => {
			setDeleting(false)
			notify(errorMessage(error, 'borrar la organización'), 'error')
		},
	})

	return (
		<div className="space-y-4">
			<Card>
				<CardHeader>
					<CardTitle>Datos de la organización</CardTitle>
					<CardDescription>
						{editable
							? 'El país y la identificación tributaria no cambian: son los del contribuyente.'
							: 'Solo el espacio dueño de la organización cambia sus datos.'}
					</CardDescription>
				</CardHeader>
				<form
					className="max-w-lg space-y-4"
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
					<FormField label="Nombre" htmlFor="name">
						<Input
							id="name"
							required
							disabled={!editable}
							value={name}
							onChange={(e) => setName(e.target.value)}
						/>
					</FormField>
					<FormField label="Correo de contacto" htmlFor="email">
						<Input
							id="email"
							type="email"
							disabled={!editable}
							value={email}
							onChange={(e) => setEmail(e.target.value)}
						/>
					</FormField>
					<p className="text-meta text-muted-foreground">
						Id de la organización: <Mono>{organization.id}</Mono>
					</p>
					{editable ? (
						<Button type="submit" disabled={save.isPending}>
							Guardar cambios
						</Button>
					) : null}
				</form>
			</Card>

			{editable ? (
				<Card>
					<CardHeader>
						<CardTitle>Borrar la organización</CardTitle>
						<CardDescription>
							Se borran sus certificados. No se puede borrar mientras tenga
							delegaciones.
						</CardDescription>
					</CardHeader>
					<div>
						<Button variant="destructive" onClick={() => setDeleting(true)}>
							Borrar la organización
						</Button>
					</div>
				</Card>
			) : null}

			<Confirm
				open={deleting}
				onOpenChange={setDeleting}
				title="¿Borrar la organización?"
				description={`${organization.name} y sus certificados se borrarán. Esta acción no se puede deshacer.`}
				confirm="Borrar la organización"
				pending={remove.isPending}
				onConfirm={() => remove.mutate()}
			/>
		</div>
	)
}
