import * as React from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { DownloadIcon } from 'lucide-react'
import { Button } from '#/components/ui/button'
import {
	Card,
	CardDescription,
	CardHeader,
	CardTitle,
} from '#/components/ui/card'
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from '#/components/ui/dialog'
import { Input } from '#/components/ui/input'
import { FormError, FormField } from '#/components/page'
import { ApiError, api, errorMessage, type Me, type Workspace } from '#/lib/api'
import { buttonVariants } from '#/lib/button-variants'
import { exportFileName, saveFile } from '#/lib/download'
import { notify } from '#/lib/notify'
import { meQuery, rememberWorkspace, useWorkspace } from '#/lib/workspace'

/**
 * «Borrar el espacio», at the end of «General», for its owners only
 * (`DELETE /workspaces/{w}`). The space of an organization of Sello (in the
 * cloud) isn't deleted here: its plan and members are Sello's, and it closes
 * when Sello deletes the organization, so the block only says so.
 */
export function DeleteWorkspace({ workspace }: { workspace: Workspace }) {
	const { role } = useWorkspace()
	const [open, setOpen] = React.useState(false)
	if (role !== 'owner') return null
	if (workspace.sello_workspace_id)
		return (
			<Card>
				<CardHeader>
					<CardTitle>Borrar el espacio</CardTitle>
					<CardDescription>
						Este espacio es el de su organización en Sello, que lleva su plan y
						sus miembros, y no se borra desde Sello Sign. Si la organización se
						elimina en Sello, el espacio se cierra y deja de firmar.
					</CardDescription>
				</CardHeader>
			</Card>
		)
	return (
		<Card>
			<CardHeader>
				<CardTitle>Borrar el espacio</CardTitle>
				<CardDescription>
					Borra el espacio con todo lo suyo: sus organizaciones con sus
					certificados, sus claves de API, sus miembros, sus invitaciones y su
					registro de firmas. Solo un propietario puede hacerlo.
				</CardDescription>
			</CardHeader>
			<Button
				variant="destructive"
				className="w-fit"
				onClick={() => setOpen(true)}
			>
				Borrar el espacio
			</Button>
			<DeleteWorkspaceDialog
				workspace={workspace}
				open={open}
				onOpenChange={setOpen}
			/>
		</Card>
	)
}

function DeleteWorkspaceDialog({
	workspace,
	open,
	onOpenChange,
}: {
	workspace: Workspace
	open: boolean
	onOpenChange: (open: boolean) => void
}) {
	const navigate = useNavigate()
	const queryClient = useQueryClient()
	const [typed, setTyped] = React.useState('')
	const confirmed = typed.trim() === workspace.name
	const download = useMutation({
		mutationFn: () => api.exportWorkspace(workspace.id),
		onSuccess: (blob) => {
			saveFile(blob, exportFileName(workspace.id, new Date()))
			notify('Datos exportados.')
		},
	})
	const remove = useMutation({
		mutationFn: () => api.deleteWorkspace(workspace.id, typed.trim()),
		onSuccess: async () => {
			const rest = (
				queryClient.getQueryData<Me>(meQuery.queryKey)?.workspaces ?? []
			).filter((w) => w.id !== workspace.id)
			notify('Espacio borrado.')
			// To the way in, which opens another space, or to creating one;
			// `/me` changes once the page has left this space.
			if (rest[0]) rememberWorkspace(rest[0].id)
			await navigate({ to: rest[0] ? '/' : '/workspaces/new', replace: true })
			queryClient.setQueryData<Me>(meQuery.queryKey, (me) =>
				me ? { ...me, workspaces: rest } : me
			)
			queryClient.removeQueries({ queryKey: ['workspace', workspace.id] })
			void queryClient.invalidateQueries({ queryKey: meQuery.queryKey })
		},
	})
	const delegated =
		remove.error instanceof ApiError &&
		remove.error.type === 'workspace-has-delegations'

	return (
		<Dialog
			open={open}
			onOpenChange={(next) => {
				if (!next) {
					setTyped('')
					remove.reset()
					download.reset()
				}
				onOpenChange(next)
			}}
		>
			<DialogContent>
				<form
					className="space-y-6"
					onSubmit={(e) => {
						e.preventDefault()
						if (confirmed) remove.mutate()
					}}
				>
					<DialogHeader>
						<DialogTitle>¿Borrar el espacio?</DialogTitle>
						<DialogDescription>
							Esta acción no se puede deshacer.
						</DialogDescription>
					</DialogHeader>
					<div className="space-y-4 text-ui">
						<p>
							Se borrarán <span className="font-medium">{workspace.name}</span>{' '}
							y todo lo suyo: sus organizaciones con sus certificados y sus
							delegaciones, sus claves de API, que dejan de funcionar al
							momento, sus miembros, sus invitaciones, su registro de firmas y
							su uso.
						</p>
						<div className="space-y-3 rounded-panel bg-fill p-4">
							<p className="text-pretty text-muted-foreground">
								Si necesita sus datos, expórtelos antes: un archivo JSON con
								todo lo del espacio, sin llaves privadas ni claves completas.
							</p>
							<Button
								type="button"
								variant="outline"
								size="sm"
								className="w-fit"
								disabled={download.isPending}
								onClick={() => download.mutate()}
							>
								<DownloadIcon />
								{download.isPending ? 'Exportando…' : 'Exportar los datos'}
							</Button>
						</div>
						<FormError
							message={
								download.error
									? errorMessage(download.error, 'exportar los datos')
									: remove.error
										? errorMessage(remove.error, 'borrar el espacio')
										: null
							}
						>
							{delegated ? (
								<Link
									to="/w/$workspaceId/organizations"
									params={{ workspaceId: workspace.id }}
									className={buttonVariants({
										variant: 'secondary',
										size: 'sm',
									})}
									onClick={() => onOpenChange(false)}
								>
									Ver las organizaciones
								</Link>
							) : null}
						</FormError>
						<FormField
							label={`Escriba «${workspace.name}» para confirmar`}
							htmlFor="delete-workspace-name"
						>
							<Input
								id="delete-workspace-name"
								autoComplete="off"
								spellCheck={false}
								value={typed}
								onChange={(e) => setTyped(e.target.value)}
							/>
						</FormField>
					</div>
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
							variant="destructive"
							disabled={!confirmed || remove.isPending}
						>
							{remove.isPending ? 'Borrando…' : 'Borrar el espacio'}
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	)
}
