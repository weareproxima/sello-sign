import * as React from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import {
	useMutation,
	useQuery,
	useQueryClient,
	type UseQueryResult,
} from '@tanstack/react-query'
import { ExternalLinkIcon, MailIcon, TriangleAlertIcon } from 'lucide-react'
import { Alert, AlertDescription } from '#/components/ui/alert'
import { Badge } from '#/components/ui/badge'
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
import { Confirm } from '#/components/confirm'
import { TableCards } from '#/components/table-cards'
import { CopyButton } from '#/components/copy-button'
import { FormError, FormField, Page, PageHeader } from '#/components/page'
import {
	ApiError,
	api,
	errorMessage,
	type Invitation,
	type Member,
	type Role,
} from '#/lib/api'
import { ROLE_LABELS, formatDate } from '#/lib/format'
import type { InviteResult } from '#/lib/invitations'
import {
	canChangeRole,
	canRemove,
	membersPageUrl,
	rolesToGive,
} from '#/lib/members'
import { notify } from '#/lib/notify'
import { buttonVariants } from '#/lib/button-variants'
import { CODE_BLOCK } from '#/lib/recipes'
import { inviteMember, resendInvitation } from '#/server/invitations'
import { useMe, useWorkspace } from '#/lib/workspace'

export const Route = createFileRoute('/_app/w/$workspaceId/members')({
	head: () => ({ meta: [{ title: 'Miembros — Sello Sign' }] }),
	component: Members,
})

function Members() {
	const { workspaceId, role, canManage } = useWorkspace()
	const { config } = Route.useRouteContext()
	const me = useMe()
	const myId = me.data?.user.id
	const queryClient = useQueryClient()
	const workspace = useQuery({
		queryKey: ['workspace', workspaceId],
		queryFn: () => api.workspace(workspaceId),
	})
	const members = useQuery({
		queryKey: ['members', workspaceId],
		queryFn: () => api.members(workspaceId),
	})
	// A space of an organization of Sello: its members are the organization's
	// seats, invited and changed in Sello's account center.
	const selloOrganization = workspace.data?.sello_workspace_id ?? null
	const managedBySello = selloOrganization !== null
	const manages = canManage && workspace.isSuccess && !managedBySello
	const invitations = useQuery({
		queryKey: ['invitations', workspaceId],
		queryFn: () => api.invitations(workspaceId),
		enabled: manages,
	})
	const [inviting, setInviting] = React.useState(false)
	const [invited, setInvited] = React.useState<Sent | null>(null)
	const [revoking, setRevoking] = React.useState<Invitation | null>(null)
	const [demotingSelf, setDemotingSelf] = React.useState<Role | null>(null)
	const [removing, setRemoving] = React.useState<Member | null>(null)
	const navigate = useNavigate()

	const revoke = useMutation({
		mutationFn: (invitation: Invitation) =>
			api.revokeInvitation(workspaceId, invitation.id),
		onSuccess: async () => {
			setRevoking(null)
			await queryClient.invalidateQueries({
				queryKey: ['invitations', workspaceId],
			})
			notify('Invitación revocada.')
		},
		onError: (error) =>
			notify(errorMessage(error, 'revocar la invitación'), 'error'),
	})
	const resend = useMutation({
		mutationFn: async (invitation: Invitation) => {
			const result = await resendInvitation({
				data: { workspaceId, invitationId: invitation.id },
			})
			if (!result.ok) throw new ApiError(result.status, result.problem)
			return result
		},
		onSuccess: async (result) => {
			await queryClient.invalidateQueries({
				queryKey: ['invitations', workspaceId],
			})
			setInvited({ ...result, resent: true })
		},
		onError: (error) =>
			notify(errorMessage(error, 'reenviar la invitación'), 'error'),
	})
	const remove = useMutation({
		mutationFn: (member: Member) =>
			api.removeMember(workspaceId, member.user_id),
		onSuccess: async (_, member) => {
			setRemoving(null)
			if (member.user_id === myId) {
				await queryClient.invalidateQueries({ queryKey: ['me'] })
				notify('Salió del espacio.')
				await navigate({ to: '/', replace: true })
				return
			}
			await queryClient.invalidateQueries({
				queryKey: ['members', workspaceId],
			})
			notify('Miembro quitado.')
		},
		onError: (error) => {
			setRemoving(null)
			notify(
				errorMessage(
					error,
					removing?.user_id === myId ? 'salir del espacio' : 'quitar al miembro'
				),
				'error'
			)
		},
	})
	const changeRole = useMutation({
		mutationFn: ({ userId, role }: { userId: string; role: Role }) =>
			api.updateMemberRole(workspaceId, userId, role),
		onSuccess: async () => {
			setDemotingSelf(null)
			await queryClient.invalidateQueries({
				queryKey: ['members', workspaceId],
			})
			await queryClient.invalidateQueries({ queryKey: ['me'] })
			notify('Rol actualizado.')
		},
		onError: (error) => {
			setDemotingSelf(null)
			notify(errorMessage(error, 'cambiar el rol'), 'error')
		},
	})

	const toGive = rolesToGive(role)
	// The last owner can't be removed nor leave (`last-owner`): not offered.
	const owners = members.data?.filter((m) => m.role === 'owner').length ?? 0
	function onRoleChange(member: Member, next: Role) {
		if (!member.user_id || next === member.role) return
		if (member.user_id === myId) {
			setDemotingSelf(next)
			return
		}
		changeRole.mutate({ userId: member.user_id, role: next })
	}

	const centerUrl =
		managedBySello && config.accountCenter
			? membersPageUrl(config.accountCenter, selloOrganization)
			: null

	return (
		<Page>
			<PageHeader
				title="Miembros"
				description={
					managedBySello
						? 'Las personas de su organización en Sello. Se invitan y cambian de rol en «Su cuenta › Organización › Miembros y acceso».'
						: 'Quién tiene acceso al espacio y con qué rol.'
				}
				actions={
					manages ? (
						<Button size="sm" onClick={() => setInviting(true)}>
							<MailIcon />
							Invitar
						</Button>
					) : centerUrl && canManage ? (
						<a
							href={centerUrl}
							className={buttonVariants({ variant: 'secondary', size: 'sm' })}
						>
							<ExternalLinkIcon />
							Abrir en Sello
						</a>
					) : null
				}
			/>
			<Card>
				{members.isPending ? (
					<Skeleton className="h-40" />
				) : members.error ? (
					<FormError
						message={errorMessage(members.error, 'leer los miembros')}
					/>
				) : (
					<TableCards>
						<Table>
							<TableHeader>
								<TableRow>
									<TableHead>Nombre</TableHead>
									<TableHead>Correo</TableHead>
									<TableHead>Rol</TableHead>
									<TableHead>
										<span className="sr-only">Acciones</span>
									</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{members.data.map((m, i) => (
									<TableRow key={m.user_id ?? `seat-${i}`}>
										<TableCell className="font-medium">
											{m.name || m.email || m.user_id || 'Sin entrar todavía'}
											{m.user_id && m.user_id === myId ? (
												<span className="ms-2 text-meta text-muted-foreground">
													(usted)
												</span>
											) : null}
										</TableCell>
										<TableCell
											data-label="Correo"
											className="text-muted-foreground"
										>
											{m.email ?? '—'}
										</TableCell>
										<TableCell data-label="Rol">
											{manages && m.user_id && canChangeRole(role, m.role) ? (
												<NativeSelect
													aria-label={`Rol de ${m.name || m.email || m.user_id}`}
													value={m.role}
													disabled={changeRole.isPending}
													onChange={(e) =>
														onRoleChange(m, e.target.value as Role)
													}
												>
													{toGive.map((r) => (
														<NativeSelectOption key={r} value={r}>
															{ROLE_LABELS[r]}
														</NativeSelectOption>
													))}
												</NativeSelect>
											) : (
												<Badge variant="secondary">{ROLE_LABELS[m.role]}</Badge>
											)}
										</TableCell>
										<TableCell data-actions className="text-right">
											{!managedBySello &&
											workspace.isSuccess &&
											m.user_id &&
											!(m.role === 'owner' && owners === 1) &&
											canRemove(role, m.role, m.user_id === myId) ? (
												<Button
													variant="ghost"
													size="sm"
													disabled={remove.isPending}
													onClick={() => setRemoving(m)}
												>
													{m.user_id === myId ? 'Salir del espacio' : 'Quitar'}
												</Button>
											) : null}
										</TableCell>
									</TableRow>
								))}
							</TableBody>
						</Table>
					</TableCards>
				)}
			</Card>

			{manages ? (
				<Pending
					invitations={invitations}
					onRevoke={(invitation) => setRevoking(invitation)}
					onResend={(invitation) => resend.mutate(invitation)}
					resending={resend.isPending ? (resend.variables?.id ?? null) : null}
				/>
			) : null}

			{manages ? (
				<Invite
					open={inviting}
					onOpenChange={setInviting}
					workspaceId={workspaceId}
					roles={toGive}
					onInvited={async (result) => {
						await queryClient.invalidateQueries({
							queryKey: ['invitations', workspaceId],
						})
						setInviting(false)
						setInvited({ ...result, resent: false })
					}}
				/>
			) : null}
			<Invited result={invited} onClose={() => setInvited(null)} />
			<Confirm
				open={revoking !== null}
				onOpenChange={(open) => !open && setRevoking(null)}
				title="¿Revocar la invitación?"
				description={`El enlace que recibió ${revoking?.email ?? ''} dejará de funcionar. Esta acción no se puede deshacer.`}
				confirm="Revocar la invitación"
				pending={revoke.isPending}
				onConfirm={() => revoking && revoke.mutate(revoking)}
			/>
			<Confirm
				open={removing !== null}
				onOpenChange={(open) => !open && setRemoving(null)}
				title={
					removing?.user_id === myId
						? '¿Salir del espacio?'
						: `¿Quitar a ${memberName(removing)}?`
				}
				description={
					removing?.user_id === myId
						? 'Dejará de tener acceso al espacio y a sus organizaciones. Para volver, alguien que lo administre tendrá que invitarle de nuevo.'
						: 'Dejará de tener acceso al espacio y a sus organizaciones. Las claves que creó y lo que firmó siguen en el espacio. Para que vuelva, habrá que invitarle de nuevo.'
				}
				confirm={removing?.user_id === myId ? 'Salir del espacio' : 'Quitar'}
				pending={remove.isPending}
				onConfirm={() => removing && remove.mutate(removing)}
			/>
			<Confirm
				open={demotingSelf !== null}
				onOpenChange={(open) => !open && setDemotingSelf(null)}
				title="¿Cambiar su propio rol?"
				description={`Pasará a ${demotingSelf ? ROLE_LABELS[demotingSelf] : ''}${
					demotingSelf === 'member'
						? ', y ya no podrá administrar el espacio ni volver a cambiar su rol'
						: ''
				}.`}
				confirm="Cambiar mi rol"
				pending={changeRole.isPending}
				onConfirm={() =>
					demotingSelf &&
					myId &&
					changeRole.mutate({ userId: myId, role: demotingSelf })
				}
			/>
		</Page>
	)
}

/** What the dialog after inviting shows; `resent` when it was sent again. */
type Sent = Extract<InviteResult, { ok: true }> & { resent: boolean }

/** How a member is named: their name, their e-mail or their id. */
function memberName(m: Member | null): string {
	return m?.name || m?.email || m?.user_id || ''
}

/** The invitations nobody accepted yet, expired ones included. */
function Pending({
	invitations,
	onRevoke,
	onResend,
	resending,
}: {
	invitations: UseQueryResult<Invitation[]>
	onRevoke: (invitation: Invitation) => void
	onResend: (invitation: Invitation) => void
	/** The one being sent again, if any. */
	resending: string | null
}) {
	if (invitations.isPending) return <Skeleton className="h-24" />
	if (invitations.error)
		return (
			<FormError
				message={errorMessage(invitations.error, 'leer las invitaciones')}
			/>
		)
	if (invitations.data.length === 0) return null
	return (
		<Card>
			<CardHeader>
				<CardTitle>Invitaciones pendientes</CardTitle>
				<CardDescription>
					Cada una vale 7 días y se acepta una sola vez, con la cuenta del
					correo al que se envió. Reenviarla le da 7 días más y un enlace nuevo;
					el anterior deja de funcionar.
				</CardDescription>
			</CardHeader>
			<TableCards>
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Correo</TableHead>
							<TableHead>Rol</TableHead>
							<TableHead>Invitó</TableHead>
							<TableHead>Vence</TableHead>
							<TableHead>
								<span className="sr-only">Acciones</span>
							</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{invitations.data.map((invitation) => (
							<TableRow key={invitation.id}>
								<TableCell className="font-medium">
									{invitation.email}
								</TableCell>
								<TableCell data-label="Rol">
									<Badge variant="secondary">
										{ROLE_LABELS[invitation.role]}
									</Badge>
								</TableCell>
								<TableCell
									data-label="Invitó"
									className="text-muted-foreground"
								>
									{invitation.invited_by.name ||
										invitation.invited_by.email ||
										'—'}
								</TableCell>
								<TableCell data-label="Vence" className="text-meta">
									{invitation.status === 'expired' ? (
										<Badge variant="warn">Vencida</Badge>
									) : (
										formatDate(invitation.expires_at)
									)}
								</TableCell>
								<TableCell data-actions className="text-right">
									<div className="flex justify-end gap-1">
										<Button
											variant="ghost"
											size="sm"
											disabled={resending !== null}
											onClick={() => onResend(invitation)}
										>
											{resending === invitation.id ? 'Reenviando…' : 'Reenviar'}
										</Button>
										<Button
											variant="ghost"
											size="sm"
											onClick={() => onRevoke(invitation)}
										>
											Revocar
										</Button>
									</div>
								</TableCell>
							</TableRow>
						))}
					</TableBody>
				</Table>
			</TableCards>
		</Card>
	)
}

function Invite({
	open,
	onOpenChange,
	workspaceId,
	roles,
	onInvited,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	workspaceId: string
	roles: Role[]
	onInvited: (result: Extract<InviteResult, { ok: true }>) => Promise<void>
}) {
	const [email, setEmail] = React.useState('')
	const [role, setRole] = React.useState<Role>('member')
	const invite = useMutation({
		mutationFn: async () => {
			const result = await inviteMember({
				data: { workspaceId, email: email.trim(), role },
			})
			if (!result.ok) throw new ApiError(result.status, result.problem)
			return result
		},
		onSuccess: async (result) => {
			await onInvited(result)
			setEmail('')
			setRole('member')
		},
	})

	return (
		<Dialog
			open={open}
			onOpenChange={(next) => {
				if (!next) invite.reset()
				onOpenChange(next)
			}}
		>
			<DialogContent>
				<form
					className="space-y-6"
					onSubmit={(e) => {
						e.preventDefault()
						invite.mutate()
					}}
				>
					<DialogHeader>
						<DialogTitle>Invitar al espacio</DialogTitle>
						<DialogDescription>
							Le enviamos un enlace para unirse, que vale 7 días.
						</DialogDescription>
					</DialogHeader>
					<FormError
						message={
							invite.error
								? errorMessage(invite.error, 'enviar la invitación')
								: null
						}
					/>
					<FormField label="Correo electrónico" htmlFor="invite-email">
						<Input
							id="invite-email"
							type="email"
							autoComplete="off"
							required
							autoFocus
							value={email}
							onChange={(e) => setEmail(e.target.value)}
						/>
					</FormField>
					<FormField label="Rol" htmlFor="invite-role" help={ROLE_HELP[role]}>
						<NativeSelect
							id="invite-role"
							className="w-full"
							value={role}
							onChange={(e) => setRole(e.target.value as Role)}
						>
							{roles.map((r) => (
								<NativeSelectOption key={r} value={r}>
									{ROLE_LABELS[r]}
								</NativeSelectOption>
							))}
						</NativeSelect>
					</FormField>
					<DialogFooter>
						<Button
							type="button"
							variant="secondary"
							onClick={() => onOpenChange(false)}
						>
							Cancelar
						</Button>
						<Button type="submit" disabled={invite.isPending || !email.trim()}>
							Enviar la invitación
						</Button>
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	)
}

const ROLE_HELP: Record<Role, string> = {
	owner: 'Todo, también dar y quitar el rol de propietario.',
	admin:
		'Organizaciones, certificados, delegaciones, claves y miembros, salvo los propietarios.',
	member: 'Consulta todo y firma de prueba desde el panel.',
}

/** The invitation just made: its link, shown once, in case the e-mail
 * doesn't arrive. */
function Invited({
	result: current,
	onClose,
}: {
	result: Sent | null
	onClose: () => void
}) {
	// The last one stays while the dialog closes, so it doesn't go blank.
	const last = React.useRef(current)
	if (current) last.current = current
	const result = current ?? last.current
	return (
		<Dialog open={current !== null} onOpenChange={(open) => !open && onClose()}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>
						{result?.emailed
							? result.resent
								? 'Invitación reenviada'
								: 'Invitación enviada'
							: result?.resent
								? 'Invitación renovada'
								: 'Invitación creada'}
					</DialogTitle>
					<DialogDescription>
						{result?.emailed
							? `Le enviamos el enlace a ${result.invitation.email}. También puede copiarlo y enviarlo usted.`
							: `Para ${result?.invitation.email ?? ''}.`}
						{result?.resent ? ' El enlace anterior ya no funciona.' : ''}
					</DialogDescription>
				</DialogHeader>
				{result && !result.emailed ? (
					<Alert variant="warn">
						<TriangleAlertIcon />
						<AlertDescription className="text-foreground">
							No se pudo enviar el correo. Copie el enlace y envíelo usted.
						</AlertDescription>
					</Alert>
				) : null}
				<pre className={CODE_BLOCK}>{result?.link}</pre>
				<DialogFooter>
					{result ? (
						<CopyButton value={result.link} label="Copiar el enlace" />
					) : null}
					<Button onClick={onClose}>Listo</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	)
}
