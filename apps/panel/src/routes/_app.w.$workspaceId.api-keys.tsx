import * as React from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRoundIcon, PlusIcon, TriangleAlertIcon } from 'lucide-react'
import { Alert, AlertDescription } from '#/components/ui/alert'
import { Badge } from '#/components/ui/badge'
import { Button } from '#/components/ui/button'
import { Card } from '#/components/ui/card'
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
import { NativeSelect, NativeSelectOption } from '#/components/ui/native-select'
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
import { Confirm } from '#/components/confirm'
import { CopyButton } from '#/components/copy-button'
import { EnvironmentPicker } from '#/components/environment-picker'
import { FormError, FormField, Page, PageHeader } from '#/components/page'
import {
	API_KEY_SCOPES,
	api,
	errorMessage,
	type ApiKey,
	type ApiKeyScope,
	type CreatedApiKey,
	type Environment,
} from '#/lib/api'
import { ENVIRONMENT_LABELS, formatDate } from '#/lib/format'
import { notify } from '#/lib/notify'
import { CODE_BLOCK } from '#/lib/recipes'
import { API_KEY_SCOPE_LABELS } from '#/lib/scopes'
import { useWorkspace } from '#/lib/workspace'

export const Route = createFileRoute('/_app/w/$workspaceId/api-keys')({
	head: () => ({ meta: [{ title: 'Claves de API — Sello Sign' }] }),
	component: ApiKeys,
})

function ApiKeys() {
	const { workspaceId, canManage } = useWorkspace()
	const queryClient = useQueryClient()
	const keys = useQuery({
		queryKey: ['api-keys', workspaceId],
		queryFn: () => api.apiKeys(workspaceId),
	})
	const organizations = useQuery({
		queryKey: ['organizations', workspaceId],
		queryFn: () => api.organizations(workspaceId),
	})
	const [creating, setCreating] = React.useState(false)
	const [created, setCreated] = React.useState<CreatedApiKey | null>(null)
	const [revoking, setRevoking] = React.useState<ApiKey | null>(null)
	const revoke = useMutation({
		mutationFn: (key: ApiKey) => api.revokeApiKey(workspaceId, key.id),
		onSuccess: async () => {
			setRevoking(null)
			await queryClient.invalidateQueries({
				queryKey: ['api-keys', workspaceId],
			})
			notify('Clave revocada.')
		},
		onError: (error) =>
			notify(errorMessage(error, 'revocar la clave'), 'error'),
	})
	const organizationName = (id: string | null) =>
		id ? (organizations.data?.find((o) => o.id === id)?.name ?? id) : null

	return (
		<Page size="wide">
			<PageHeader
				title="Claves de API"
				description="Las claves con las que sus sistemas llaman al API en nombre del espacio, cada una con su ambiente y sus permisos."
				actions={
					canManage ? (
						<Button size="sm" onClick={() => setCreating(true)}>
							<PlusIcon />
							Crear una clave
						</Button>
					) : null
				}
			/>
			<Card>
				{keys.isPending ? (
					<Skeleton className="h-40" />
				) : keys.error ? (
					<FormError message={errorMessage(keys.error, 'leer las claves')} />
				) : keys.data.length === 0 ? (
					<Empty
						icon={<KeyRoundIcon />}
						title="Sin claves de API"
						description="Cree una clave para firmar desde su sistema con POST /signatures."
					/>
				) : (
					<TableCards>
						<Table>
							<TableHeader>
								<TableRow>
									<TableHead>Nombre</TableHead>
									<TableHead>Ambiente</TableHead>
									<TableHead>Alcance</TableHead>
									<TableHead>Permisos</TableHead>
									<TableHead>Último uso</TableHead>
									<TableHead>Creada</TableHead>
									{canManage ? (
										<TableHead>
											<span className="sr-only">Acciones</span>
										</TableHead>
									) : null}
								</TableRow>
							</TableHeader>
							<TableBody>
								{keys.data.map((key) => {
									return (
										<TableRow key={key.id}>
											<TableCell>
												<span className="font-medium">{key.name}</span>
												<span className="block font-mono text-label text-muted-foreground">
													{key.prefix}…
												</span>
											</TableCell>
											<TableCell data-label="Ambiente">
												<Badge
													variant={
														key.environment === 'prod' ? 'default' : 'secondary'
													}
												>
													{ENVIRONMENT_LABELS[key.environment]}
												</Badge>
											</TableCell>
											<TableCell data-label="Alcance">
												{organizationName(key.organization_id) ??
													'Todo el espacio'}
											</TableCell>
											<TableCell
												data-label="Permisos"
												className="max-w-72 whitespace-normal"
											>
												<div className="flex flex-wrap gap-1">
													{key.scopes.map((s) => (
														<Badge
															key={s}
															variant="outline"
															className="font-mono"
														>
															{s}
														</Badge>
													))}
												</div>
											</TableCell>
											<TableCell data-label="Último uso" className="text-meta">
												{key.last_used_at
													? formatDate(key.last_used_at)
													: 'Nunca'}
											</TableCell>
											<TableCell data-label="Creada" className="text-meta">
												{formatDate(key.created_at)}
											</TableCell>
											{canManage ? (
												<TableCell data-actions className="text-right">
													<Button
														variant="ghost"
														size="sm"
														onClick={() => setRevoking(key)}
													>
														Revocar
													</Button>
												</TableCell>
											) : null}
										</TableRow>
									)
								})}
							</TableBody>
						</Table>
					</TableCards>
				)}
			</Card>

			<CreateKey
				open={creating}
				onOpenChange={setCreating}
				workspaceId={workspaceId}
				organizations={(organizations.data ?? []).map((o) => ({
					id: o.id,
					name: o.name,
				}))}
				onCreated={async (key) => {
					await queryClient.invalidateQueries({
						queryKey: ['api-keys', workspaceId],
					})
					setCreating(false)
					setCreated(key)
				}}
			/>
			<RevealKey created={created} onClose={() => setCreated(null)} />
			<Confirm
				open={revoking !== null}
				onOpenChange={(open) => !open && setRevoking(null)}
				title="¿Revocar la clave?"
				description="Cualquier integración que la use dejará de funcionar de inmediato. Esta acción no se puede deshacer."
				confirm="Revocar la clave"
				pending={revoke.isPending}
				onConfirm={() => revoking && revoke.mutate(revoking)}
			/>
		</Page>
	)
}

function CreateKey({
	open,
	onOpenChange,
	workspaceId,
	organizations,
	onCreated,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	workspaceId: string
	organizations: { id: string; name: string }[]
	onCreated: (key: CreatedApiKey) => Promise<void>
}) {
	const [name, setName] = React.useState('')
	const [environment, setEnvironment] = React.useState<Environment>('test')
	const [organization, setOrganization] = React.useState('')
	const [scopes, setScopes] = React.useState<ApiKeyScope[]>(['sign'])
	const create = useMutation({
		mutationFn: () =>
			api.createApiKey(workspaceId, {
				name: name.trim(),
				environment,
				scopes,
				organization_id: organization || undefined,
			}),
		onSuccess: async (key) => {
			await onCreated(key)
			setName('')
			setScopes(['sign'])
			setOrganization('')
		},
	})

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="sm:max-w-lg">
				<form
					className="space-y-6"
					onSubmit={(e) => {
						e.preventDefault()
						create.mutate()
					}}
				>
					<DialogHeader>
						<DialogTitle>Crear una clave de API</DialogTitle>
						<DialogDescription>
							Su ambiente es el de su prefijo: sign_test_ o sign_prod_.
						</DialogDescription>
					</DialogHeader>
					<FormError
						message={
							create.error ? errorMessage(create.error, 'crear la clave') : null
						}
					/>
					<FormField label="Nombre" htmlFor="key-name">
						<Input
							id="key-name"
							required
							placeholder="Mi integración"
							value={name}
							onChange={(e) => setName(e.target.value)}
						/>
					</FormField>
					<FormField label="Ambiente">
						<EnvironmentPicker value={environment} onChange={setEnvironment} />
					</FormField>
					<FormField
						label="Organización"
						htmlFor="key-organization"
						help="Una clave de una sola organización solo firma con ella."
					>
						<NativeSelect
							id="key-organization"
							className="w-full"
							value={organization}
							onChange={(e) => setOrganization(e.target.value)}
						>
							<NativeSelectOption value="">Todo el espacio</NativeSelectOption>
							{organizations.map((o) => (
								<NativeSelectOption key={o.id} value={o.id}>
									{o.name}
								</NativeSelectOption>
							))}
						</NativeSelect>
					</FormField>
					<fieldset className="space-y-3">
						<legend className="mb-2 text-ui font-medium">Permisos</legend>
						<div className="grid max-h-64 gap-3 overflow-y-auto sm:grid-cols-2">
							{API_KEY_SCOPES.map((scope) => (
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
											{API_KEY_SCOPE_LABELS[scope].label}
										</span>
										<span className="block text-meta text-muted-foreground">
											{API_KEY_SCOPE_LABELS[scope].description}
										</span>
									</span>
								</label>
							))}
						</div>
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
							disabled={create.isPending || scopes.length === 0 || !name.trim()}
						>
							Crear la clave
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	)
}

/** The key, once: the API keeps only its hash. */
function RevealKey({
	created,
	onClose,
}: {
	created: CreatedApiKey | null
	onClose: () => void
}) {
	return (
		<Dialog open={created !== null} onOpenChange={(open) => !open && onClose()}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>Clave creada</DialogTitle>
					<DialogDescription>{created?.name}</DialogDescription>
				</DialogHeader>
				<Alert variant="warn">
					<TriangleAlertIcon />
					<AlertDescription className="text-foreground">
						Esta clave solo se muestra una vez. Cópiela ahora y guárdela en un
						lugar seguro.
					</AlertDescription>
				</Alert>
				<pre className={CODE_BLOCK}>{created?.key}</pre>
				<DialogFooter>
					{created ? <CopyButton value={created.key} size="sm" /> : null}
					<Button onClick={onClose}>Listo</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	)
}
