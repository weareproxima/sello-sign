import * as React from 'react'
import { Link, createFileRoute, useNavigate } from '@tanstack/react-router'
import { Button } from '#/components/ui/button'
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
} from '#/components/ui/card'
import { Input } from '#/components/ui/input'
import { FormError, FormField } from '#/components/page'
import { authClient } from '#/lib/auth-client'
import { safeRedirect } from '#/lib/redirect'

type Search = { redirect?: string; error?: string }

export const Route = createFileRoute('/_auth/sign-in')({
	validateSearch: (search: Record<string, unknown>): Search => ({
		redirect: typeof search.redirect === 'string' ? search.redirect : undefined,
		error: typeof search.error === 'string' ? search.error : undefined,
	}),
	head: () => ({ meta: [{ title: 'Iniciar sesión — Sello Sign' }] }),
	component: SignIn,
})

const LINK =
	'font-medium text-foreground underline decoration-line-strong underline-offset-4 transition-colors duration-fast hover:decoration-foreground'

function SignIn() {
	const { config } = Route.useRouteContext()
	const search = Route.useSearch()
	const navigate = useNavigate()
	const target = safeRedirect(search.redirect)
	const oidc = config.oidc
	const [email, setEmail] = React.useState('')
	const [password, setPassword] = React.useState('')
	const [error, setError] = React.useState<string | null>(
		search.error && oidc
			? `No se pudo iniciar sesión con ${oidc.name}. Vuelva a intentarlo.`
			: null
	)
	const [pending, setPending] = React.useState(false)

	async function continueWithOidc() {
		if (!oidc) return
		setPending(true)
		const { error } = await authClient.signIn.social({
			provider: oidc.providerId,
			callbackURL: target,
			errorCallbackURL: '/sign-in?error=oidc',
		})
		if (error) {
			setPending(false)
			setError(
				`No se pudo iniciar sesión con ${oidc.name}. Vuelva a intentarlo.`
			)
		}
	}

	async function submit(event: React.FormEvent) {
		event.preventDefault()
		setPending(true)
		setError(null)
		const { error } = await authClient.signIn.email({ email, password })
		setPending(false)
		if (error) {
			setError(
				error.status === 401 || error.code === 'INVALID_EMAIL_OR_PASSWORD'
					? 'El correo o la contraseña no son correctos.'
					: 'No se pudo iniciar sesión. Vuelva a intentarlo.'
			)
			return
		}
		await navigate({ to: target })
	}

	if (oidc?.only) {
		return (
			<Card>
				<CardHeader className="text-center">
					<h1 className="text-h1">Iniciar sesión</h1>
					<CardDescription>Con su cuenta de {oidc.name}.</CardDescription>
				</CardHeader>
				<CardContent className="space-y-4">
					<FormError message={error} />
					<Button
						className="w-full"
						onClick={continueWithOidc}
						disabled={pending}
					>
						Continuar con {oidc.name}
					</Button>
					<p className="text-center text-ui text-muted-foreground">
						¿Aún no tiene cuenta? La crea al continuar.
					</p>
				</CardContent>
			</Card>
		)
	}

	return (
		<Card>
			<CardHeader className="text-center">
				<h1 className="text-h1">Iniciar sesión</h1>
				<CardDescription>Con su correo y su contraseña.</CardDescription>
			</CardHeader>
			<CardContent>
				<form onSubmit={submit} className="space-y-4">
					<FormError message={error} />
					{oidc ? (
						<>
							<Button
								type="button"
								variant="secondary"
								className="w-full"
								onClick={continueWithOidc}
								disabled={pending}
							>
								Continuar con {oidc.name}
							</Button>
							<p className="text-center text-meta text-muted-foreground">
								O con su correo
							</p>
						</>
					) : null}
					<FormField label="Correo electrónico" htmlFor="email">
						<Input
							id="email"
							type="email"
							autoComplete="email"
							required
							autoFocus
							value={email}
							onChange={(e) => setEmail(e.target.value)}
						/>
					</FormField>
					<FormField
						label="Contraseña"
						htmlFor="password"
						aside={
							<Link
								to="/forgot-password"
								className="ms-auto text-meta text-muted-foreground underline-offset-4 transition-colors duration-fast hover:text-foreground hover:underline"
							>
								¿Olvidó su contraseña?
							</Link>
						}
					>
						<Input
							id="password"
							type="password"
							autoComplete="current-password"
							required
							value={password}
							onChange={(e) => setPassword(e.target.value)}
						/>
					</FormField>
					<Button type="submit" className="w-full" disabled={pending}>
						Iniciar sesión
					</Button>
					<p className="text-center text-ui text-muted-foreground">
						¿No tiene cuenta?{' '}
						<Link
							to="/sign-up"
							search={{ redirect: search.redirect }}
							className={LINK}
						>
							Crear una cuenta
						</Link>
					</p>
				</form>
			</CardContent>
		</Card>
	)
}
