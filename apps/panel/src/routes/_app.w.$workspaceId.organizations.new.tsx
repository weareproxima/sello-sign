import * as React from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Button } from '#/components/ui/button'
import { Card } from '#/components/ui/card'
import { Input } from '#/components/ui/input'
import { CountrySelect } from '#/components/country-select'
import { FormError, FormField, Page, PageHeader } from '#/components/page'
import { api, errorMessage } from '#/lib/api'
import { taxIdLabel } from '#/lib/countries'
import { useWorkspace } from '#/lib/workspace'

export const Route = createFileRoute('/_app/w/$workspaceId/organizations/new')({
	head: () => ({ meta: [{ title: 'Crear una organización — Sello Sign' }] }),
	component: NewOrganization,
})

function NewOrganization() {
	const { workspaceId, workspace } = useWorkspace()
	const navigate = useNavigate()
	const queryClient = useQueryClient()
	const [name, setName] = React.useState('')
	const [country, setCountry] = React.useState(workspace?.default_country ?? '')
	const [taxId, setTaxId] = React.useState('')
	const [email, setEmail] = React.useState('')
	const create = useMutation({
		mutationFn: () =>
			api.createOrganization(workspaceId, {
				name: name.trim(),
				country,
				tax_id: taxId.trim(),
				contact_email: email.trim() || undefined,
			}),
		onSuccess: async (organization) => {
			await queryClient.invalidateQueries({
				queryKey: ['organizations', workspaceId],
			})
			await navigate({
				to: '/w/$workspaceId/organizations/$organizationId',
				params: { workspaceId, organizationId: organization.id },
			})
		},
	})

	return (
		<Page size="narrow">
			<PageHeader
				title="Crear una organización"
				description="El contribuyente con el que firmará: su nombre, su país y su identificación tributaria."
				back={{
					label: 'Volver a las organizaciones',
					to: '/w/$workspaceId/organizations',
					params: { workspaceId },
				}}
			/>
			<Card>
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
								? errorMessage(create.error, 'crear la organización')
								: null
						}
					/>
					<FormField label="Nombre" htmlFor="name">
						<Input
							id="name"
							required
							maxLength={200}
							autoFocus
							placeholder="Acme, S.A."
							value={name}
							onChange={(e) => setName(e.target.value)}
						/>
					</FormField>
					<FormField label="País" htmlFor="country">
						<CountrySelect id="country" value={country} onChange={setCountry} />
					</FormField>
					<FormField
						label={country ? taxIdLabel(country) : 'Identificación tributaria'}
						htmlFor="tax-id"
						help="La del certificado: Sello Sign la compara con la de cada certificado que suba."
					>
						<Input
							id="tax-id"
							required
							maxLength={30}
							className="font-mono"
							value={taxId}
							onChange={(e) => setTaxId(e.target.value)}
						/>
					</FormField>
					<FormField
						label="Correo de contacto"
						htmlFor="email"
						help="Opcional. Quien recibe los avisos de la organización, como un certificado por vencer."
					>
						<Input
							id="email"
							type="email"
							placeholder="contacto@empresa.com"
							value={email}
							onChange={(e) => setEmail(e.target.value)}
						/>
					</FormField>
					<div className="flex gap-2">
						<Button type="submit" disabled={create.isPending}>
							Crear la organización
						</Button>
						<Button
							type="button"
							variant="secondary"
							onClick={() =>
								navigate({
									to: '/w/$workspaceId/organizations',
									params: { workspaceId },
								})
							}
						>
							Cancelar
						</Button>
					</div>
				</form>
			</Card>
		</Page>
	)
}
