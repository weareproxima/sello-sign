import * as React from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { MoreHorizontalIcon, ShieldCheckIcon, UploadIcon } from 'lucide-react'
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
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from '#/components/ui/dropdown-menu'
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
import { CertificateBadge } from '#/components/certificate-badge'
import { Confirm } from '#/components/confirm'
import { FormError, FormField } from '#/components/page'
import { EnvironmentPicker } from '#/components/environment-picker'
import {
	api,
	errorMessage,
	type Certificate,
	type Environment,
} from '#/lib/api'
import { ENVIRONMENT_LABELS, formatDate } from '#/lib/format'
import { notify } from '#/lib/notify'
import { organizationKey, useOrganization } from '#/lib/organization'
import { useWorkspace } from '#/lib/workspace'

export const Route = createFileRoute(
	'/_app/w/$workspaceId/organizations/$organizationId/'
)({
	head: () => ({ meta: [{ title: 'Certificados — Sello Sign' }] }),
	component: Certificates,
})

function Certificates() {
	const { workspaceId, organizationId } = useOrganization()
	const { canManage } = useWorkspace()
	const queryClient = useQueryClient()
	const key = ['certificates', workspaceId, organizationId] as const
	const certificates = useQuery({
		queryKey: key,
		queryFn: () => api.certificates(workspaceId, organizationId),
	})
	const [uploading, setUploading] = React.useState(false)
	const [deleting, setDeleting] = React.useState<Certificate | null>(null)

	const refresh = () =>
		Promise.all([
			queryClient.invalidateQueries({ queryKey: key }),
			queryClient.invalidateQueries({
				queryKey: organizationKey(workspaceId, organizationId),
			}),
			queryClient.invalidateQueries({
				queryKey: ['organizations', workspaceId],
			}),
		])

	const toggle = useMutation({
		mutationFn: (c: Certificate) =>
			api.setCertificateActive(workspaceId, organizationId, c.id, !c.active),
		onSuccess: async (_, c) => {
			await refresh()
			notify(c.active ? 'Certificado desactivado.' : 'Certificado activado.')
		},
		onError: (error) =>
			notify(errorMessage(error, 'cambiar el certificado'), 'error'),
	})
	const remove = useMutation({
		mutationFn: (c: Certificate) =>
			api.deleteCertificate(workspaceId, organizationId, c.id),
		onSuccess: async () => {
			setDeleting(null)
			await refresh()
			notify('Certificado borrado.')
		},
		onError: (error) =>
			notify(errorMessage(error, 'borrar el certificado'), 'error'),
	})

	return (
		<Card>
			<CardHeader>
				<CardTitle>Certificados</CardTitle>
				<CardDescription>
					Uno activo por ambiente: con él se firma. Activar otro desactiva el
					anterior.
				</CardDescription>
				{canManage ? (
					<CardAction>
						<Button size="sm" onClick={() => setUploading(true)}>
							<UploadIcon />
							Subir un certificado
						</Button>
					</CardAction>
				) : null}
			</CardHeader>

			{certificates.isPending ? (
				<Skeleton className="h-32" />
			) : certificates.error ? (
				<FormError
					message={errorMessage(certificates.error, 'leer los certificados')}
				/>
			) : certificates.data.length === 0 ? (
				<Empty
					icon={<ShieldCheckIcon />}
					title="Sin certificados"
					description="Suba el certificado de firma de la organización para poder firmar con ella."
				/>
			) : (
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Ambiente</TableHead>
							<TableHead>Sujeto</TableHead>
							<TableHead>Serie</TableHead>
							<TableHead>Vigencia</TableHead>
							<TableHead>Estado</TableHead>
							{canManage ? (
								<TableHead className="w-10">
									<span className="sr-only">Acciones</span>
								</TableHead>
							) : null}
						</TableRow>
					</TableHeader>
					<TableBody>
						{certificates.data.map((c) => (
							<TableRow
								key={c.id}
								className={c.active ? undefined : 'text-faint'}
							>
								<TableCell>
									<Badge
										variant={c.environment === 'prod' ? 'default' : 'secondary'}
									>
										{ENVIRONMENT_LABELS[c.environment]}
									</Badge>
								</TableCell>
								<TableCell className="max-w-64 truncate" title={c.subject}>
									{c.subject}
									<span className="block font-mono text-label text-muted-foreground">
										{c.tax_id ?? '—'}
									</span>
								</TableCell>
								<TableCell className="font-mono text-label">
									{c.serial}
								</TableCell>
								<TableCell className="text-meta">
									{formatDate(c.not_before)} – {formatDate(c.not_after)}
								</TableCell>
								<TableCell>
									<CertificateBadge certificate={c} />
								</TableCell>
								{canManage ? (
									<TableCell>
										<DropdownMenu>
											<DropdownMenuTrigger
												render={
													<Button
														variant="ghost"
														size="icon-sm"
														aria-label="Acciones del certificado"
													/>
												}
											>
												<MoreHorizontalIcon />
											</DropdownMenuTrigger>
											<DropdownMenuContent align="end" className="min-w-48">
												<DropdownMenuItem onClick={() => toggle.mutate(c)}>
													{c.active ? 'Desactivar' : 'Activar'}
												</DropdownMenuItem>
												<DropdownMenuItem
													variant="destructive"
													onClick={() => setDeleting(c)}
												>
													Borrar
												</DropdownMenuItem>
											</DropdownMenuContent>
										</DropdownMenu>
									</TableCell>
								) : null}
							</TableRow>
						))}
					</TableBody>
				</Table>
			)}

			<UploadDialog
				open={uploading}
				onOpenChange={setUploading}
				workspaceId={workspaceId}
				organizationId={organizationId}
				onUploaded={refresh}
			/>
			<Confirm
				open={deleting !== null}
				onOpenChange={(open) => !open && setDeleting(null)}
				title="¿Borrar el certificado?"
				description="Si es el activo, la organización dejará de firmar en ese ambiente hasta que suba otro. Esta acción no se puede deshacer."
				confirm="Borrar el certificado"
				pending={remove.isPending}
				onConfirm={() => deleting && remove.mutate(deleting)}
			/>
		</Card>
	)
}

/** The file as base64, without the `data:` prefix. */
function readBase64(file: File): Promise<string> {
	return new Promise((resolve, reject) => {
		const reader = new FileReader()
		reader.onload = () => {
			const result = String(reader.result)
			resolve(result.slice(result.indexOf(',') + 1))
		}
		reader.onerror = () => reject(reader.error)
		reader.readAsDataURL(file)
	})
}

function UploadDialog({
	open,
	onOpenChange,
	workspaceId,
	organizationId,
	onUploaded,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	workspaceId: string
	organizationId: string
	onUploaded: () => Promise<unknown>
}) {
	const [environment, setEnvironment] = React.useState<Environment>('test')
	const [file, setFile] = React.useState<File | null>(null)
	const [password, setPassword] = React.useState('')
	const [activate, setActivate] = React.useState(true)
	const upload = useMutation({
		mutationFn: async () => {
			if (!file) throw new Error('no file')
			return api.uploadCertificate(workspaceId, organizationId, {
				environment,
				file: await readBase64(file),
				password: password || undefined,
				activate,
			})
		},
		onSuccess: async () => {
			await onUploaded()
			notify('Certificado subido.')
			onOpenChange(false)
			setFile(null)
			setPassword('')
		},
	})

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent>
				<form
					className="space-y-6"
					onSubmit={(e) => {
						e.preventDefault()
						upload.mutate()
					}}
				>
					<DialogHeader>
						<DialogTitle>Subir un certificado</DialogTitle>
						<DialogDescription>
							El archivo de la autoridad tributaria (.crt, .p12, .pfx) y, si lo
							lleva, su PIN. Sello Sign lee su NIT y su vigencia.
						</DialogDescription>
					</DialogHeader>
					<FormError
						message={
							upload.error
								? errorMessage(upload.error, 'subir el certificado')
								: null
						}
					/>
					<FormField label="Ambiente">
						<EnvironmentPicker value={environment} onChange={setEnvironment} />
					</FormField>
					<FormField label="Archivo" htmlFor="file">
						<Input
							id="file"
							type="file"
							required
							accept=".crt,.cer,.pem,.p12,.pfx"
							className="py-2"
							onChange={(e) => setFile(e.target.files?.[0] ?? null)}
						/>
					</FormField>
					<FormField
						label="PIN o contraseña"
						htmlFor="password"
						help="El del archivo .p12 o .pfx. Nunca se guarda en claro."
					>
						<Input
							id="password"
							type="password"
							autoComplete="off"
							value={password}
							onChange={(e) => setPassword(e.target.value)}
						/>
					</FormField>
					<label className="flex items-center gap-3 text-ui">
						<Checkbox
							checked={activate}
							onCheckedChange={(v) => setActivate(v === true)}
						/>
						Activarlo al subirlo
					</label>
					<DialogFooter>
						<Button
							type="button"
							variant="secondary"
							onClick={() => onOpenChange(false)}
						>
							Cancelar
						</Button>
						<Button type="submit" disabled={upload.isPending || !file}>
							Subir
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	)
}
