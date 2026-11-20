import * as React from 'react'
import { Link, createFileRoute, useNavigate } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Button } from '#/components/ui/button'
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
} from '#/components/ui/card'
import { Skeleton } from '#/components/ui/skeleton'
import { Logo } from '#/components/logo'
import { FormError } from '#/components/page'
import { ApiError, api, clearToken, errorMessage } from '#/lib/api'
import { authClient } from '#/lib/auth-client'
import { buttonVariants } from '#/lib/button-variants'
import { ROLE_LABELS, formatDate } from '#/lib/format'
import { notify } from '#/lib/notify'
import { rememberWorkspace } from '#/lib/workspace'
import { getPublicConfig, getSession } from '#/server/session'

type Search = { token?: string }

/**
 * Accepting an invitation, from the link of its e-mail
 * (`/invitation?token=…`). Without a session it asks to sign in or create
 * the account, and comes back here; with one, it shows the invitation and
 * accepts it. Outside `_auth` (which sends whoever is signed in away) and
 * `_app` (which goes straight to signing in).
 */
export const Route = createFileRoute('/invitation')({
	validateSearch: (search: Record<string, unknown>): Search => ({
		token: typeof search.token === 'string' ? search.token : undefined,
	}),
	beforeLoad: async () => ({
		session: await getSession(),
		config: await getPublicConfig(),
	}),
	head: () => ({ meta: [{ title: 'Invitación — Sello Sign' }] }),
	component: InvitationPage,
})

function InvitationPage() {
	const { token } = Route.useSearch()
	const { session, config } = Route.useRouteContext()
	const here = token ? `/invitation?token=${encodeURIComponent(token)}` : '/'
	return (
		<div className="flex min-h-svh flex-col bg-subtle">
			<main className="flex flex-1 flex-col items-center justify-center gap-6 px-4 py-8 sm:p-6 md:p-10">
				<Logo className="h-8" />
				<div className="w-full max-w-sm">
					{!token ? (
						<Message
							title="Enlace no válido"
							description="El enlace de la invitación está incompleto. Ábralo otra vez desde el correo."
						/>
					) : !session ? (
						<SignInFirst
							here={here}
							signUp={!config.oidc?.only}
							provider={config.oidc?.name ?? null}
						/>
					) : (
						<Accept token={token} email={session.user.email} here={here} />
					)}
				</div>
			</main>
			<footer className="px-4 py-6 text-center text-meta text-muted-foreground">
				Sello Sign · AGPL-3.0
			</footer>
		</div>
	)
}

function Message({
	title,
	description,
	children,
}: {
	title: string
	description: React.ReactNode
	children?: React.ReactNode
}) {
	return (
		<Card>
			<CardHeader className="text-center">
				<h1 className="text-h1">{title}</h1>
				<CardDescription>{description}</CardDescription>
			</CardHeader>
			<CardContent className="space-y-3">
				{children ?? (
					<Link to="/" className={buttonVariants({ className: 'w-full' })}>
						Ir al panel
					</Link>
				)}
			</CardContent>
		</Card>
	)
}

function SignInFirst({
	here,
	signUp,
	provider,
}: {
	here: string
	signUp: boolean
	provider: string | null
}) {
	return (
		<Message
			title="Aceptar la invitación"
			description={
				provider && !signUp
					? `Le invitaron a un espacio de Sello Sign. Para aceptar, inicie sesión con su cuenta de ${provider} del correo al que le llegó la invitación.`
					: 'Le invitaron a un espacio de Sello Sign. Para aceptar, inicie sesión o cree una cuenta con el correo al que le llegó la invitación.'
			}
		>
			<Link
				to="/sign-in"
				search={{ redirect: here }}
				className={buttonVariants({ className: 'w-full' })}
			>
				Iniciar sesión
			</Link>
			{signUp ? (
				<Link
					to="/sign-up"
					search={{ redirect: here }}
					className={buttonVariants({
						variant: 'secondary',
						className: 'w-full',
					})}
				>
					Crear una cuenta
				</Link>
			) : null}
		</Message>
	)
}

function Accept({
	token,
	email,
	here,
}: {
	token: string
	email: string
	here: string
}) {
	const navigate = useNavigate()
	const queryClient = useQueryClient()
	const lookup = useQuery({
		queryKey: ['invitation', token],
		queryFn: () => api.lookupInvitation(token),
		retry: false,
	})
	const accept = useMutation({
		mutationFn: () => api.acceptInvitation(token),
		onSuccess: async (workspace) => {
			rememberWorkspace(workspace.id)
			await queryClient.invalidateQueries({ queryKey: ['me'] })
			notify('Invitación aceptada.')
			await navigate({
				to: '/w/$workspaceId',
				params: { workspaceId: workspace.id },
			})
		},
	})
	const signOut = useMutation({
		mutationFn: async () => {
			await authClient.signOut()
			clearToken()
			queryClient.clear()
		},
		onSuccess: () => {
			window.location.assign(`/sign-in?redirect=${encodeURIComponent(here)}`)
		},
	})

	if (lookup.isPending) return <Skeleton className="h-56" />
	if (lookup.error) {
		const gone = lookup.error instanceof ApiError && lookup.error.status === 404
		return (
			<Message
				title={gone ? 'La invitación ya no existe' : 'No se pudo abrir'}
				description={
					gone
						? 'Se revocó, o el enlace no es el del correo. Pida otra a quien administra el espacio.'
						: errorMessage(lookup.error, 'leer la invitación')
				}
			/>
		)
	}

	const invitation = lookup.data
	const inviter =
		invitation.invited_by.name || invitation.invited_by.email || 'Alguien'
	const summary = `${inviter} le invitó a «${invitation.workspace_name}» con el rol de ${ROLE_LABELS[invitation.role]}.`

	if (invitation.status === 'accepted')
		return (
			<Message
				title="Invitación aceptada"
				description="Esta invitación ya se aceptó."
			/>
		)
	if (invitation.status === 'expired')
		return (
			<Message
				title="La invitación venció"
				description={`${summary} Venció el ${formatDate(invitation.expires_at)}: pida otra a quien administra el espacio.`}
			/>
		)
	if (invitation.email !== email.trim().toLowerCase())
		return (
			<Message
				title="Otra cuenta"
				description={
					<>
						La invitación es para <strong>{invitation.email}</strong>, y usted
						inició sesión como <strong>{email}</strong>. Inicie sesión con la
						cuenta de ese correo, o créela.
					</>
				}
			>
				<Button
					className="w-full"
					disabled={signOut.isPending}
					onClick={() => signOut.mutate()}
				>
					Cerrar sesión
				</Button>
			</Message>
		)

	return (
		<Card>
			<CardHeader className="text-center">
				<h1 className="text-h1 break-words">
					Unirse a {invitation.workspace_name}
				</h1>
				<CardDescription>{summary}</CardDescription>
			</CardHeader>
			<CardContent className="space-y-4">
				<FormError
					message={
						accept.error
							? errorMessage(accept.error, 'aceptar la invitación')
							: null
					}
				/>
				<Button
					className="w-full"
					disabled={accept.isPending}
					onClick={() => accept.mutate()}
				>
					Aceptar la invitación
				</Button>
				<p className="text-center text-meta text-muted-foreground">
					Vence el {formatDate(invitation.expires_at)}.
				</p>
			</CardContent>
		</Card>
	)
}
