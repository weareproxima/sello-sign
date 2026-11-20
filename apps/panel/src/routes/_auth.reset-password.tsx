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

type Search = { token?: string; error?: string }

export const Route = createFileRoute('/_auth/reset-password')({
	validateSearch: (search: Record<string, unknown>): Search => ({
		token: typeof search.token === 'string' ? search.token : undefined,
		error: typeof search.error === 'string' ? search.error : undefined,
	}),
	head: () => ({ meta: [{ title: 'Restablecer la contraseña — Sello Sign' }] }),
	component: ResetPassword,
})

const INVALID =
	'El enlace no es válido o ya venció. Pida uno nuevo desde «¿Olvidó su contraseña?».'

function ResetPassword() {
	const { token, error: linkError } = Route.useSearch()
	const [password, setPassword] = React.useState('')
	const [confirm, setConfirm] = React.useState('')
	const [done, setDone] = React.useState(false)
	const [error, setError] = React.useState<string | null>(
		!token || linkError ? INVALID : null
	)
	const [pending, setPending] = React.useState(false)

	async function submit(event: React.FormEvent) {
		event.preventDefault()
		if (!token) return
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
		const { error } = await authClient.resetPassword({
			newPassword: password,
			token,
		})
		setPending(false)
		if (error) {
			setError(
				error.code === 'INVALID_TOKEN'
					? INVALID
					: 'No se pudo restablecer la contraseña. Vuelva a intentarlo.'
			)
			return
		}
		setDone(true)
	}

	return (
		<Card>
			<CardHeader className="text-center">
				<h1 className="text-h1">Restablecer la contraseña</h1>
				<CardDescription>Elija su contraseña nueva.</CardDescription>
			</CardHeader>
			<CardContent className="space-y-4">
				{done ? (
					<div
						role="status"
						className="rounded-panel bg-ok-fill px-4 py-3 text-ui"
					>
						Contraseña actualizada. Ya puede iniciar sesión con ella.
					</div>
				) : (
					<form onSubmit={submit} className="space-y-4">
						<FormError message={error} />
						<FormField
							label="Contraseña nueva"
							htmlFor="password"
							help="Al menos 8 caracteres."
						>
							<Input
								id="password"
								type="password"
								autoComplete="new-password"
								required
								disabled={!token}
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
								disabled={!token}
								value={confirm}
								onChange={(e) => setConfirm(e.target.value)}
							/>
						</FormField>
						<Button
							type="submit"
							className="w-full"
							disabled={pending || !token}
						>
							Restablecer la contraseña
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
