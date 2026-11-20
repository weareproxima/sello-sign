import * as React from 'react'
import { Link, createFileRoute, useRouter } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ExternalLinkIcon, MonitorSmartphoneIcon } from 'lucide-react'
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from '#/components/ui/dialog'
import { Badge } from '#/components/ui/badge'
import { Button } from '#/components/ui/button'
import { Checkbox } from '#/components/ui/checkbox'
import {
	Card,
	CardDescription,
	CardHeader,
	CardTitle,
} from '#/components/ui/card'
import { Input } from '#/components/ui/input'
import { Skeleton } from '#/components/ui/skeleton'
import { Confirm } from '#/components/confirm'
import { FormError, FormField, Page, PageHeader } from '#/components/page'
import {
	deleteAccountMessage,
	type DeleteAccountResult,
} from '#/lib/account-deletion'
import { api, clearToken, errorMessage, type AccountDeletion } from '#/lib/api'
import { authClient } from '#/lib/auth-client'
import { buttonVariants } from '#/lib/button-variants'
import { formatDateTime } from '#/lib/format'
import { notify } from '#/lib/notify'
import { TEXT_LINK } from '#/lib/recipes'
import { accountCenterPage, describeUserAgent } from '#/lib/user-agent'
import { deleteAccount } from '#/server/account'

/**
 * The person's account: their name, their password and their open sessions,
 * all better-auth's. With `OIDC_ONLY` the provider owns the name and the
 * password (better-auth rewrites the name at each sign-in), so they're shown,
 * not changed; in Sello's cloud they link to Sello's account center.
 */
export const Route = createFileRoute('/_app/w/$workspaceId/account')({
	head: () => ({ meta: [{ title: 'Su cuenta — Sello Sign' }] }),
	component: Account,
})

/** A better-auth error as a sentence of the panel. */
function authError(
	error: { code?: string; status?: number } | null | undefined,
	action: string
): string | null {
	if (!error) return null
	if (error.code === 'INVALID_PASSWORD')
		return 'La contraseña actual no es correcta.'
	if (error.code === 'PASSWORD_TOO_SHORT')
		return 'La contraseña nueva debe tener al menos 8 caracteres.'
	if (error.status === 429)
		return 'Demasiados intentos. Espere un momento y vuelva a intentarlo.'
	return `No se pudo ${action}. Vuelva a intentarlo.`
}

function Account() {
	const { session, config } = Route.useRouteContext()
	const managed = config.oidc?.only === true
	const provider = config.oidc?.name ?? 'su proveedor'
	const [here, setHere] = React.useState<string | undefined>()
	React.useEffect(() => setHere(window.location.href), [])
	const center = (section: 'profile' | 'security') =>
		config.accountCenter
			? accountCenterPage(config.accountCenter, section, here)
			: null

	return (
		<Page size="narrow">
			<PageHeader
				title="Su cuenta"
				description="Su nombre, su contraseña y dónde tiene la sesión abierta. Valen para todos sus espacios."
			/>
			<Profile
				name={session.user.name}
				email={session.user.email}
				managedBy={managed ? provider : null}
				centerUrl={center('profile')}
			/>
			{managed ? (
				<ManagedPassword provider={provider} centerUrl={center('security')} />
			) : (
				<Password provider={config.oidc?.name ?? null} />
			)}
			<Sessions />
			{config.accountDeletion ? <DeleteAccount /> : null}
		</Page>
	)
}

function CenterLink({ href, children }: { href: string; children: string }) {
	return (
		<a
			href={href}
			className={buttonVariants({ variant: 'secondary', className: 'w-fit' })}
		>
			{children}
			<ExternalLinkIcon />
		</a>
	)
}

function Profile({
	name: initial,
	email,
	managedBy,
	centerUrl,
}: {
	name: string
	email: string
	managedBy: string | null
	centerUrl: string | null
}) {
	const router = useRouter()
	const [name, setName] = React.useState(initial)
	const save = useMutation({
		mutationFn: async () => {
			const { error } = await authClient.updateUser({ name: name.trim() })
			if (error) throw error
		},
		onSuccess: async () => {
			notify('Cambios guardados.')
			await router.invalidate()
		},
	})
	const editable = !managedBy

	return (
		<Card>
			<CardHeader>
				<CardTitle>Datos personales</CardTitle>
				<CardDescription>
					{managedBy
						? `Su nombre y su correo vienen de su cuenta de ${managedBy}, y se cambian allí.`
						: 'Su nombre aparece en los espacios donde es miembro. El correo no se cambia aquí.'}
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
					message={authError(
						save.error as { code?: string; status?: number } | null,
						'guardar los cambios'
					)}
				/>
				<FormField label="Nombre" htmlFor="name">
					<Input
						id="name"
						required
						maxLength={100}
						autoComplete="name"
						disabled={!editable}
						value={name}
						onChange={(e) => setName(e.target.value)}
					/>
				</FormField>
				<FormField label="Correo electrónico" htmlFor="email">
					<Input id="email" type="email" disabled value={email} />
				</FormField>
				{editable ? (
					<Button
						type="submit"
						disabled={save.isPending || !name.trim() || name.trim() === initial}
					>
						Guardar cambios
					</Button>
				) : centerUrl ? (
					<CenterLink href={centerUrl}>Abrir su cuenta de Sello</CenterLink>
				) : null}
			</form>
		</Card>
	)
}

function ManagedPassword({
	provider,
	centerUrl,
}: {
	provider: string
	centerUrl: string | null
}) {
	return (
		<Card>
			<CardHeader>
				<CardTitle>Contraseña y seguridad</CardTitle>
				<CardDescription>
					Entra con su cuenta de {provider}: su contraseña
					{centerUrl
						? ', su verificación en dos pasos y sus llaves de acceso se administran allí.'
						: ' se administra allí.'}
				</CardDescription>
			</CardHeader>
			{centerUrl ? (
				<CenterLink href={centerUrl}>
					Abrir la seguridad de su cuenta
				</CenterLink>
			) : null}
		</Card>
	)
}

function Password({ provider }: { provider: string | null }) {
	const accounts = useQuery({
		queryKey: ['auth-accounts'],
		queryFn: async () => {
			const { data, error } = await authClient.listAccounts()
			if (error) throw error
			return data
		},
	})
	const [current, setCurrent] = React.useState('')
	const [next, setNext] = React.useState('')
	const [confirm, setConfirm] = React.useState('')
	const [revokeOthers, setRevokeOthers] = React.useState(true)
	const queryClient = useQueryClient()
	const change = useMutation({
		mutationFn: async () => {
			const { error } = await authClient.changePassword({
				currentPassword: current,
				newPassword: next,
				revokeOtherSessions: revokeOthers,
			})
			if (error) throw error
		},
		onSuccess: async () => {
			setCurrent('')
			setNext('')
			setConfirm('')
			notify('Contraseña actualizada.')
			await queryClient.invalidateQueries({ queryKey: ['auth-sessions'] })
		},
	})
	const mismatch = confirm !== '' && confirm !== next
	const hasPassword = accounts.data?.some((a) => a.providerId === 'credential')

	return (
		<Card>
			<CardHeader>
				<CardTitle>Contraseña</CardTitle>
				<CardDescription>
					{hasPassword === false
						? `Su cuenta entra con ${provider ?? 'un proveedor externo'} y no tiene contraseña en Sello Sign.`
						: 'Al menos 8 caracteres. Necesita la actual para cambiarla.'}
				</CardDescription>
			</CardHeader>
			{accounts.isPending ? (
				<Skeleton className="h-48" />
			) : hasPassword === false ? null : (
				<form
					className="space-y-4"
					onSubmit={(e) => {
						e.preventDefault()
						if (!mismatch) change.mutate()
					}}
				>
					<FormError
						message={authError(
							change.error as { code?: string; status?: number } | null,
							'cambiar la contraseña'
						)}
					/>
					<FormField label="Contraseña actual" htmlFor="current-password">
						<Input
							id="current-password"
							type="password"
							required
							autoComplete="current-password"
							value={current}
							onChange={(e) => setCurrent(e.target.value)}
						/>
					</FormField>
					<FormField label="Contraseña nueva" htmlFor="new-password">
						<Input
							id="new-password"
							type="password"
							required
							minLength={8}
							autoComplete="new-password"
							value={next}
							onChange={(e) => setNext(e.target.value)}
						/>
					</FormField>
					<FormField
						label="Repetir la contraseña nueva"
						htmlFor="confirm-password"
						error={mismatch ? 'Las contraseñas no coinciden.' : null}
					>
						<Input
							id="confirm-password"
							type="password"
							required
							minLength={8}
							autoComplete="new-password"
							value={confirm}
							onChange={(e) => setConfirm(e.target.value)}
						/>
					</FormField>
					<label className="flex items-center gap-3 text-ui">
						<Checkbox
							checked={revokeOthers}
							onCheckedChange={(v) => setRevokeOthers(v === true)}
						/>
						Cerrar las demás sesiones
					</label>
					<Button
						type="submit"
						disabled={change.isPending || mismatch || next.length < 8}
					>
						Cambiar la contraseña
					</Button>
				</form>
			)}
		</Card>
	)
}

function Sessions() {
	const queryClient = useQueryClient()
	const sessions = useQuery({
		queryKey: ['auth-sessions'],
		queryFn: async () => {
			const [list, current] = await Promise.all([
				authClient.listSessions(),
				authClient.getSession(),
			])
			if (list.error) throw list.error
			const token = current.data?.session.token ?? null
			return [...(list.data ?? [])]
				.map((s) => ({ ...s, current: s.token === token }))
				.sort(
					(a, b) =>
						Number(b.current) - Number(a.current) ||
						new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime()
				)
		},
	})
	const [confirming, setConfirming] = React.useState(false)
	const refresh = () =>
		queryClient.invalidateQueries({ queryKey: ['auth-sessions'] })
	const revokeOthers = useMutation({
		mutationFn: async () => {
			const { error } = await authClient.revokeOtherSessions()
			if (error) throw error
		},
		onSuccess: async () => {
			setConfirming(false)
			notify('Sesiones cerradas.')
			await refresh()
		},
		onError: () => {
			setConfirming(false)
			notify(
				'No se pudieron cerrar las sesiones. Vuelva a intentarlo.',
				'error'
			)
		},
	})
	const revoke = useMutation({
		mutationFn: async (token: string) => {
			const { error } = await authClient.revokeSession({ token })
			if (error) throw error
		},
		onSuccess: async () => {
			notify('Sesión cerrada.')
			await refresh()
		},
		onError: () =>
			notify('No se pudo cerrar la sesión. Vuelva a intentarlo.', 'error'),
	})
	const others = sessions.data?.filter((s) => !s.current).length ?? 0

	return (
		<Card>
			<CardHeader>
				<CardTitle>Sesiones activas</CardTitle>
				<CardDescription>
					Los navegadores donde su cuenta tiene la sesión abierta en Sello Sign.
					Si no reconoce alguno, ciérrelo.
				</CardDescription>
			</CardHeader>
			{sessions.isPending ? (
				<Skeleton className="h-32" />
			) : sessions.error ? (
				<FormError message="No se pudieron leer las sesiones. Vuelva a intentarlo." />
			) : (
				<>
					<ul className="divide-y divide-line">
						{sessions.data.map((s) => (
							<li
								key={s.id}
								className="flex items-center gap-3 py-3 first:pt-0 last:pb-0"
							>
								<span className="flex size-9 shrink-0 items-center justify-center rounded-row bg-fill text-muted-foreground [&_svg]:size-4">
									<MonitorSmartphoneIcon />
								</span>
								<div className="grid min-w-0 flex-1 gap-0.5">
									<div className="flex flex-wrap items-center gap-2">
										<span className="truncate text-ui font-medium">
											{describeUserAgent(s.userAgent) ?? 'Sesión'}
										</span>
										{s.current ? <Badge variant="ok">Esta sesión</Badge> : null}
									</div>
									<span className="text-meta text-pretty text-muted-foreground">
										{[
											s.ipAddress,
											`Última actividad: ${formatDateTime(s.updatedAt)}`,
										]
											.filter(Boolean)
											.join(' · ')}
									</span>
								</div>
								{s.current ? null : (
									<Button
										variant="ghost"
										size="sm"
										disabled={revoke.isPending}
										onClick={() => revoke.mutate(s.token)}
									>
										Cerrar
									</Button>
								)}
							</li>
						))}
					</ul>
					<Button
						variant="secondary"
						className="w-fit"
						disabled={others === 0 || revokeOthers.isPending}
						onClick={() => setConfirming(true)}
					>
						Cerrar las demás sesiones
					</Button>
				</>
			)}
			<Confirm
				open={confirming}
				onOpenChange={setConfirming}
				title="¿Cerrar las demás sesiones?"
				description="Su cuenta saldrá de los demás navegadores. Esta sesión sigue abierta."
				confirm="Cerrar las sesiones"
				pending={revokeOthers.isPending}
				onConfirm={() => revokeOthers.mutate()}
			/>
		</Card>
	)
}

type AccountSpace = AccountDeletion['workspaces'][number]

/**
 * Deleting the account (self-hosted only: `config.accountDeletion`). Before
 * asking, it shows what happens to each space (`GET /me/account-deletion`):
 * the only owner of a space with others has to make someone else owner
 * first, and the spaces where the person is alone go with the account.
 */
function DeleteAccount() {
	const [open, setOpen] = React.useState(false)
	return (
		<Card>
			<CardHeader>
				<CardTitle>Borrar la cuenta</CardTitle>
				<CardDescription>
					Borra su cuenta de Sello Sign y le saca de todos sus espacios. Los
					espacios donde es el único miembro se borran con ella, con sus
					organizaciones, sus certificados, sus claves de API y su registro de
					firmas.
				</CardDescription>
			</CardHeader>
			<Button
				variant="destructive"
				className="w-fit"
				onClick={() => setOpen(true)}
			>
				Borrar la cuenta
			</Button>
			<DeleteAccountDialog open={open} onOpenChange={setOpen} />
		</Card>
	)
}

function DeleteAccountDialog({
	open,
	onOpenChange,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
}) {
	const preview = useQuery({
		queryKey: ['account-deletion'],
		queryFn: () => api.accountDeletion(),
		enabled: open,
		staleTime: 0,
	})
	const accounts = useQuery({
		queryKey: ['auth-accounts'],
		queryFn: async () => {
			const { data, error } = await authClient.listAccounts()
			if (error) throw error
			return data
		},
		enabled: open,
	})
	const hasPassword = accounts.data?.some((a) => a.providerId === 'credential')
	const [password, setPassword] = React.useState('')
	const remove = useMutation({
		mutationFn: async (): Promise<DeleteAccountResult> =>
			deleteAccount({ data: { password: hasPassword ? password : null } }),
		onSuccess: async (result) => {
			if (!result.ok) {
				if (result.reason === 'blocked') await preview.refetch()
				return
			}
			clearToken()
			window.location.assign('/sign-in')
		},
	})
	const failed = remove.data && !remove.data.ok ? remove.data : null
	const spaces = preview.data?.workspaces ?? []
	const of = (...outcomes: AccountSpace['outcome'][]) =>
		spaces.filter((w) => outcomes.includes(w.outcome))
	const blocking = of('transfer', 'delegated')
	const deleted = of('delete')
	const left = of('leave')

	return (
		<Dialog
			open={open}
			onOpenChange={(next) => {
				if (!next) {
					setPassword('')
					remove.reset()
				}
				onOpenChange(next)
			}}
		>
			<DialogContent>
				<form
					className="space-y-6"
					onSubmit={(e) => {
						e.preventDefault()
						remove.mutate()
					}}
				>
					<DialogHeader>
						<DialogTitle>¿Borrar su cuenta?</DialogTitle>
						<DialogDescription>
							{blocking.length > 0
								? 'Antes de borrarla, resuelva estos espacios.'
								: 'Esta acción no se puede deshacer.'}
						</DialogDescription>
					</DialogHeader>
					{preview.isPending || accounts.isPending ? (
						<Skeleton className="h-24" />
					) : preview.error ? (
						<FormError
							message={errorMessage(preview.error, 'revisar sus espacios')}
						/>
					) : blocking.length > 0 ? (
						<ul className="space-y-3 text-ui">
							{blocking.map((w) => (
								<li key={w.id}>
									<span className="font-medium">{w.name}</span>:{' '}
									{w.outcome === 'transfer' ? (
										<>
											es su único propietario y tiene otros miembros. Haga
											propietario a otra persona, o quite a los demás, en{' '}
											<Link
												to="/w/$workspaceId/members"
												params={{ workspaceId: w.id }}
												className={TEXT_LINK}
											>
												Miembros
											</Link>
											.
										</>
									) : (
										<>
											otros espacios usan sus organizaciones por delegación.
											Quítelas en{' '}
											<Link
												to="/w/$workspaceId/organizations"
												params={{ workspaceId: w.id }}
												className={TEXT_LINK}
											>
												Organizaciones
											</Link>
											.
										</>
									)}
								</li>
							))}
						</ul>
					) : (
						<div className="space-y-4 text-ui">
							{deleted.length > 0 ? (
								<SpaceList
									title="Se borrarán, con todo lo suyo, los espacios donde es el único miembro:"
									spaces={deleted}
									note={
										<>
											Si necesita sus datos, expórtelos antes en «General» de
											cada uno.
										</>
									}
								/>
							) : null}
							{left.length > 0 ? (
								<SpaceList
									title="Saldrá de estos espacios, que siguen con sus demás miembros:"
									spaces={left}
								/>
							) : null}
							{spaces.length === 0 ? (
								<p className="text-muted-foreground">
									No está en ningún espacio.
								</p>
							) : null}
							<FormError
								message={
									failed
										? deleteAccountMessage(failed)
										: remove.error
											? 'No se pudo borrar la cuenta. Vuelva a intentarlo.'
											: null
								}
							/>
							{hasPassword ? (
								<FormField label="Contraseña" htmlFor="delete-password">
									<Input
										id="delete-password"
										type="password"
										required
										autoComplete="current-password"
										value={password}
										onChange={(e) => setPassword(e.target.value)}
									/>
								</FormField>
							) : null}
						</div>
					)}
					<DialogFooter>
						<Button
							type="button"
							variant="secondary"
							onClick={() => onOpenChange(false)}
						>
							Cancelar
						</Button>
						{preview.data?.allowed ? (
							<Button
								type="submit"
								variant="destructive"
								disabled={remove.isPending || (hasPassword && !password)}
							>
								{remove.isPending ? 'Borrando…' : 'Borrar la cuenta'}
							</Button>
						) : null}
					</DialogFooter>
				</form>
			</DialogContent>
		</Dialog>
	)
}

function SpaceList({
	title,
	spaces,
	note,
}: {
	title: string
	spaces: AccountSpace[]
	note?: React.ReactNode
}) {
	return (
		<div className="space-y-2">
			<p>{title}</p>
			<ul className="list-disc space-y-1 ps-5">
				{spaces.map((w) => (
					<li key={w.id}>
						<span className="font-medium">{w.name}</span>
						{w.organizations > 0 ? (
							<span className="text-muted-foreground">
								{' '}
								(
								{w.organizations === 1
									? '1 organización'
									: `${w.organizations} organizaciones`}
								)
							</span>
						) : null}
					</li>
				))}
			</ul>
			{note ? <p className="text-meta text-muted-foreground">{note}</p> : null}
		</div>
	)
}
