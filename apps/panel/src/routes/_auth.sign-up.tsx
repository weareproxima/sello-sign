import * as React from 'react'
import {
	Link,
	createFileRoute,
	redirect,
	useNavigate,
} from '@tanstack/react-router'
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
import { getPublicConfig } from '#/server/session'

export const Route = createFileRoute('/_auth/sign-up')({
	validateSearch: (search: Record<string, unknown>): { redirect?: string } => ({
		redirect: typeof search.redirect === 'string' ? search.redirect : undefined,
	}),
	// With OIDC_ONLY the account is made at the provider.
	beforeLoad: async () => {
		const config = await getPublicConfig()
		if (config.oidc?.only) throw redirect({ to: '/sign-in' })
	},
	head: () => ({ meta: [{ title: 'Crear una cuenta — Sello Sign' }] }),
	component: SignUp,
})

function SignUp() {
	const search = Route.useSearch()
	const navigate = useNavigate()
	const [name, setName] = React.useState('')
	const [email, setEmail] = React.useState('')
	const [password, setPassword] = React.useState('')
	const [confirm, setConfirm] = React.useState('')
	const [error, setError] = React.useState<string | null>(null)
	const [pending, setPending] = React.useState(false)

	async function submit(event: React.FormEvent) {
		event.preventDefault()
		if (password.length < 8) {
			setError('La contraseña debe tener al menos 8 caracteres.')
			return
		}
		if (password !== confirm) {
			setError('Las contraseñas no coinciden.')
			return
		}
		setPending(true)
		setError(null)
		const { error } = await authClient.signUp.email({ name, email, password })
		setPending(false)
		if (error) {
			setError(
				error.code === 'USER_ALREADY_EXISTS' ||
					error.code === 'USER_ALREADY_EXISTS_USE_ANOTHER_EMAIL'
					? 'Ya existe una cuenta con este correo.'
					: 'No se pudo crear la cuenta. Vuelva a intentarlo.'
			)
			return
		}
		await navigate({ to: safeRedirect(search.redirect) })
	}

	return (
		<Card>
			<CardHeader className="text-center">
				<h1 className="text-h1">Crear una cuenta</h1>
				<CardDescription>
					Con su nombre, su correo y una contraseña.
				</CardDescription>
			</CardHeader>
			<CardContent>
				<form onSubmit={submit} className="space-y-4">
					<FormError message={error} />
					<FormField label="Nombre completo" htmlFor="name">
						<Input
							id="name"
							autoComplete="name"
							required
							autoFocus
							value={name}
							onChange={(e) => setName(e.target.value)}
						/>
					</FormField>
					<FormField label="Correo electrónico" htmlFor="email">
						<Input
							id="email"
							type="email"
							autoComplete="email"
							required
							value={email}
							onChange={(e) => setEmail(e.target.value)}
						/>
					</FormField>
					<FormField
						label="Contraseña"
						htmlFor="password"
						help="Al menos 8 caracteres."
					>
						<Input
							id="password"
							type="password"
							autoComplete="new-password"
							required
							value={password}
							onChange={(e) => setPassword(e.target.value)}
						/>
					</FormField>
					<FormField label="Confirmar contraseña" htmlFor="confirm">
						<Input
							id="confirm"
							type="password"
							autoComplete="new-password"
							required
							value={confirm}
							onChange={(e) => setConfirm(e.target.value)}
						/>
					</FormField>
					<Button type="submit" className="w-full" disabled={pending}>
						Crear cuenta
					</Button>
					<p className="text-center text-ui text-muted-foreground">
						¿Ya tiene cuenta?{' '}
						<Link
							to="/sign-in"
							search={{ redirect: search.redirect }}
							className="font-medium text-foreground underline decoration-line-strong underline-offset-4 transition-colors duration-fast hover:decoration-foreground"
						>
							Iniciar sesión
						</Link>
					</p>
				</form>
			</CardContent>
		</Card>
	)
}
