import * as React from 'react'
import { Link, createFileRoute } from '@tanstack/react-router'
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

export const Route = createFileRoute('/_auth/forgot-password')({
	head: () => ({ meta: [{ title: 'Recuperar la contraseña — Sello Sign' }] }),
	component: ForgotPassword,
})

function ForgotPassword() {
	const [email, setEmail] = React.useState('')
	const [sent, setSent] = React.useState(false)
	const [error, setError] = React.useState<string | null>(null)
	const [pending, setPending] = React.useState(false)

	async function submit(event: React.FormEvent) {
		event.preventDefault()
		setPending(true)
		setError(null)
		const { error } = await authClient.requestPasswordReset({
			email,
			redirectTo: '/reset-password',
		})
		setPending(false)
		if (error) {
			setError('No se pudo enviar el enlace. Vuelva a intentarlo.')
			return
		}
		setSent(true)
	}

	return (
		<Card>
			<CardHeader className="text-center">
				<h1 className="text-h1">Recuperar la contraseña</h1>
				<CardDescription>
					Le enviaremos un enlace para elegir una contraseña nueva.
				</CardDescription>
			</CardHeader>
			<CardContent className="space-y-4">
				{sent ? (
					<div
						role="status"
						className="rounded-panel bg-fill px-4 py-3 text-ui"
					>
						Si existe una cuenta con ese correo, le enviamos un enlace para
						restablecer la contraseña. Revise su bandeja de entrada.
					</div>
				) : (
					<form onSubmit={submit} className="space-y-4">
						<FormError message={error} />
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
						<Button type="submit" className="w-full" disabled={pending}>
							Enviar el enlace
						</Button>
					</form>
				)}
				<p className="text-center text-ui">
					<Link
						to="/sign-in"
						className="text-muted-foreground transition-colors duration-fast hover:text-foreground"
					>
						Volver al inicio de sesión
					</Link>
				</p>
			</CardContent>
		</Card>
	)
}
