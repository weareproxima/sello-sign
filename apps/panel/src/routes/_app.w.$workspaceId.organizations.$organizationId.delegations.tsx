import * as React from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { HandshakeIcon, LockIcon, PlusIcon } from 'lucide-react'
import { Badge } from '#/components/ui/badge'
import { Button } from '#/components/ui/button'
import {
	Card,
	CardAction,
	CardDescription,
	CardHeader,
	CardTitle,
} from '#/components/ui/card'
import { Checkbox } from '#/components/ui/checkbox'
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from '#/components/ui/dialog'
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
import { TableCards } from '#/components/table-cards'
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from '#/components/ui/tooltip'
import { Confirm } from '#/components/confirm'
import { FormError, FormField, Mono } from '#/components/page'
import {
	DELEGATION_SCOPES,
	api,
	errorMessage,
	type Delegation,
	type DelegationScope,
} from '#/lib/api'
import { formatDate } from '#/lib/format'
import { DELEGATION_SCOPE_LABELS } from '#/lib/scopes'
import { notify } from '#/lib/notify'
import { useOrganization } from '#/lib/organization'
import { useWorkspace } from '#/lib/workspace'

export const Route = createFileRoute(
	'/_app/w/$workspaceId/organizations/$organizationId/delegations'
)({
	head: () => ({ meta: [{ title: 'Delegaciones — Sello Sign' }] }),
	component: Delegations,
})

function Delegations() {
	const { workspaceId, organizationId, data: organization } = useOrganization()
	const { canManage } = useWorkspace()
	const queryClient = useQueryClient()
	const key = ['delegations', workspaceId, organizationId] as const
	const owned = organization?.relation === 'owned'
	const delegations = useQuery({
		queryKey: key,
		queryFn: () => api.delegations(workspaceId, organizationId),
		enabled: owned,
	})
	const [creating, setCreating] = React.useState(false)
	const [removing, setRemoving] = React.useState<Delegation | null>(null)
	const remove = useMutation({
		mutationFn: (d: Delegation) =>
			api.deleteDelegation(workspaceId, organizationId, d.id),
		onSuccess: async () => {
			setRemoving(null)
			await queryClient.invalidateQueries({ queryKey: key })
			notify('Delegación quitada.')
		},
		onError: (error) =>
			notify(errorMessage(error, 'quitar la delegación'), 'error'),
	})

	if (organization && !owned) {
		return (
			<Card>
				<CardHeader>
					<CardTitle>Delegaciones</CardTitle>
					<CardDescription>
						Esta organización es de otro espacio, que le delegó firmar con ella.
						Solo su dueño da y quita delegaciones.
					</CardDescription>
				</CardHeader>
			</Card>
		)
	}

	return (
		<Card>
			<CardHeader>
				<CardTitle>Delegaciones</CardTitle>
				<CardDescription>
					Los espacios a los que esta organización deja firmar con ella sin
					entregarles su certificado, como su ERP o su proveedor de facturación.
				</CardDescription>
				{canManage ? (
					<CardAction>
						<Button size="sm" onClick={() => setCreating(true)}>
							<PlusIcon />
							Delegar
						</Button>
					</CardAction>
				) : null}
			</CardHeader>

			{delegations.isPending ? (
				<Skeleton className="h-32" />
			) : delegations.error ? (
				<FormError
					message={errorMessage(delegations.error, 'leer las delegaciones')}
				/>
			) : delegations.data.length === 0 ? (
				<Empty
					icon={<HandshakeIcon />}
					title="Sin delegaciones"
					description="Ningún otro espacio firma con esta organización."
				/>
			) : (
				<TableCards>
					<Table>
						<TableHeader>
							<TableRow>
								<TableHead>Espacio</TableHead>
								<TableHead>Permisos</TableHead>
								<TableHead>Desde</TableHead>
								{canManage ? (
									<TableHead className="text-right">
										<span className="sr-only">Acciones</span>
									</TableHead>
								) : null}
							</TableRow>
						</TableHeader>
						<TableBody>
							{delegations.data.map((d) => (
								<TableRow key={d.id}>
									<TableCell>
										<span className="font-medium">{d.workspace_name}</span>
										{d.locked ? (
											<Badge variant="secondary" className="ms-2">
												<LockIcon data-icon="inline-start" />
												Plataforma de Sello
											</Badge>
										) : null}
										<Mono className="block text-muted-foreground">
											{d.workspace_id}
										</Mono>
									</TableCell>
									<TableCell
										data-label="Permisos"
										className="whitespace-normal"
									>
										<div className="flex flex-wrap gap-1">
											{d.scopes.map((s) => (
												<Badge key={s} variant="outline">
													{DELEGATION_SCOPE_LABELS[s]?.label ?? s}
												</Badge>
											))}
										</div>
									</TableCell>
									<TableCell
										data-label="Desde"
										className="text-meta text-muted-foreground"
									>
										{formatDate(d.created_at)}
									</TableCell>
									{canManage ? (
										<TableCell data-actions className="text-right">
											{d.locked ? (
												<Tooltip>
													<TooltipTrigger render={<span tabIndex={0} />}>
														<Button variant="ghost" size="sm" disabled>
															Quitar
														</Button>
													</TooltipTrigger>
													<TooltipContent className="max-w-xs">
														Es de la plataforma de Sello, que emite con esta
														organización. Solo ella la quita, cuando la
														organización deja de emitir con Sello.
													</TooltipContent>
												</Tooltip>
											) : (
												<Button
													variant="ghost"
													size="sm"
													onClick={() => setRemoving(d)}
												>
													Quitar
												</Button>
											)}
										</TableCell>
									) : null}
								</TableRow>
							))}
						</TableBody>
					</Table>
				</TableCards>
			)}

			{delegations.data?.some((d) => d.locked) ? (
				<p className="flex items-start gap-2 text-meta text-pretty text-muted-foreground">
					<LockIcon className="mt-0.5 size-3.5 shrink-0" />
					La delegación de la plataforma de Sello no se quita desde aquí:
					mientras la organización emita con Sello, quitarla rompería su
					emisión.
				</p>
			) : null}

			<CreateDelegation
				open={creating}
				onOpenChange={setCreating}
				workspaceId={workspaceId}
				organizationId={organizationId}
				onCreated={() => queryClient.invalidateQueries({ queryKey: key })}
			/>
			<Confirm
				open={removing !== null}
				onOpenChange={(open) => !open && setRemoving(null)}
				title="¿Quitar la delegación?"
				description={`${removing?.workspace_name ?? 'Ese espacio'} dejará de firmar con esta organización de inmediato.`}
				confirm="Quitar la delegación"
				pending={remove.isPending}
				onConfirm={() => removing && remove.mutate(removing)}
			/>
		</Card>
	)
}

function CreateDelegation({
	open,
	onOpenChange,
	workspaceId,
	organizationId,
	onCreated,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	workspaceId: string
	organizationId: string
	onCreated: () => Promise<unknown>
}) {
	const [target, setTarget] = React.useState('')
	const [scopes, setScopes] = React.useState<DelegationScope[]>(['sign'])
	const create = useMutation({
		mutationFn: () =>
			api.createDelegation(workspaceId, organizationId, {
				workspace_id: target.trim(),
				scopes,
			}),
		onSuccess: async () => {
			await onCreated()
			notify('Delegación creada.')
			onOpenChange(false)
			setTarget('')
			setScopes(['sign'])
		},
	})

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent>
				<form
					className="space-y-6"
					onSubmit={(e) => {
						e.preventDefault()
						create.mutate()
					}}
				>
					<DialogHeader>
						<DialogTitle>Delegar la organización</DialogTitle>
						<DialogDescription>
							El espacio que la recibe firma con ella con sus propias claves, y
							lo que firma cuenta en su plan.
						</DialogDescription>
					</DialogHeader>
					<FormError
						message={
							create.error
								? errorMessage(create.error, 'crear la delegación')
								: null
						}
					/>
					<FormField
						label="Id del espacio"
						htmlFor="workspace"
						help="Quien administra ese espacio lo encuentra en «General»."
					>
						<Input
							id="workspace"
							required
							className="font-mono"
							value={target}
							onChange={(e) => setTarget(e.target.value)}
						/>
					</FormField>
					<fieldset className="space-y-3">
						<legend className="mb-2 text-ui font-medium">Permisos</legend>
						{DELEGATION_SCOPES.map((scope) => (
							<label key={scope} className="flex items-start gap-3">
								<Checkbox
									className="mt-0.5"
									checked={scopes.includes(scope)}
									onCheckedChange={(checked) =>
										setScopes((current) =>
											checked
												? [...current, scope]
												: current.filter((s) => s !== scope)
										)
									}
								/>
								<span>
									<span className="block text-ui">
										{DELEGATION_SCOPE_LABELS[scope].label}
									</span>
									<span className="block text-meta text-muted-foreground">
										{DELEGATION_SCOPE_LABELS[scope].description}
									</span>
								</span>
							</label>
						))}
					</fieldset>
					<DialogFooter>
						<Button
							type="button"
							variant="secondary"
							onClick={() => onOpenChange(false)}
						>
							Cancelar
						</Button>
						<Button
							type="submit"
							disabled={
								create.isPending || scopes.length === 0 || !target.trim()
							}
						>
							Delegar
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	)
}
