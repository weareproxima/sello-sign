import * as React from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Button } from '#/components/ui/button'
import {
	Card,
	CardDescription,
	CardHeader,
	CardTitle,
} from '#/components/ui/card'
import { Input } from '#/components/ui/input'
import { FormError, FormField } from '#/components/page'
import { ApiError, api, errorMessage, type Workspace } from '#/lib/api'
import { formatNumber } from '#/lib/format'
import {
	effectiveRate,
	ownRateProblem,
	rateError,
	rateInput,
} from '#/lib/limits'
import { notify } from '#/lib/notify'
import { useWorkspace } from '#/lib/workspace'

/**
 * The space's signatures a minute (`requests_per_minute` of
 * `PATCH /workspaces/{w}`, owners and admins): a limit the space sets for
 * itself below its plan's ceiling, or none (`null`), and then the ceiling
 * applies. The API refuses one above the ceiling with a `400`, said under
 * the field: billing may have lowered the ceiling since the page loaded.
 */
export function SigningRate({ workspace }: { workspace: Workspace }) {
	const { canManage } = useWorkspace()
	const queryClient = useQueryClient()
	const ceiling = workspace.limits.requests_per_minute
	const own = workspace.requests_per_minute
	const rate = effectiveRate(ceiling, own)
	const [value, setValue] = React.useState(own === null ? '' : String(own))
	const [fieldError, setFieldError] = React.useState<string | null>(null)
	const [checked, setChecked] = React.useState(false)
	// What failed in the API and isn't the field's.
	const [serverError, setServerError] = React.useState<string | null>(null)
	const check = (next: string) => setFieldError(rateError(next, ceiling))

	const save = useMutation({
		mutationFn: (next: number | null) =>
			api.updateWorkspace(workspace.id, { requests_per_minute: next }),
		onSuccess: async (updated, next) => {
			queryClient.setQueryData(['workspace', workspace.id], updated)
			setValue(next === null ? '' : String(next))
			setFieldError(null)
			setChecked(false)
			notify(next === null ? 'Límite quitado.' : 'Cambios guardados.')
		},
		onError: async (error) => {
			const field =
				error instanceof ApiError ? ownRateProblem(error.problem) : null
			if (!field) {
				setServerError(errorMessage(error, 'guardar el límite'))
				return
			}
			setFieldError(field)
			setChecked(true)
			// The ceiling may have changed: read it again.
			await queryClient.invalidateQueries({
				queryKey: ['workspace', workspace.id],
			})
		},
	})

	function submit(event: React.FormEvent) {
		event.preventDefault()
		setServerError(null)
		const problem = rateError(value, ceiling)
		setChecked(true)
		setFieldError(problem)
		if (problem) return
		save.mutate(value.trim() === '' ? null : Number(value))
	}

	// Self-hosted, the ceiling is the installation's, not a plan's.
	const source = workspace.plan ? 'El plan del espacio' : 'Esta instalación'
	const ofCeiling = workspace.plan ? 'del plan' : 'de la instalación'
	const now =
		rate === null
			? 'Ahora no tiene límite por minuto.'
			: own !== null && (ceiling === null || own < ceiling)
				? `Ahora firma hasta ${formatNumber(rate)} por minuto, el límite que se puso el espacio.`
				: `Ahora firma hasta ${formatNumber(rate)} por minuto, el límite ${ofCeiling}.`

	return (
		<Card id="signing-rate">
			<CardHeader>
				<CardTitle>Firmas por minuto</CardTitle>
				<CardDescription>
					{ceiling === null
						? `${source} no pone un límite de firmas por minuto. `
						: `${source} permite hasta ${formatNumber(ceiling)} firmas por minuto. `}
					{canManage
						? 'Puede ponerle al espacio uno menor; cada clave de API puede tener además el suyo. Lo que pase el límite se rechaza hasta el minuto siguiente.'
						: 'Solo quien administra el espacio le pone uno menor.'}
				</CardDescription>
			</CardHeader>
			{canManage ? (
				<form className="space-y-4" noValidate onSubmit={submit}>
					<FormError message={serverError} />
					<FormField
						label="Límite del espacio"
						htmlFor="signing-rate-value"
						help={
							fieldError
								? null
								: `${now} ${
										ceiling === null
											? 'Vacío, sin límite.'
											: `De 1 a ${formatNumber(ceiling)}; vacío, vale el ${ofCeiling}.`
									}`
						}
						error={fieldError}
					>
						<Input
							id="signing-rate-value"
							inputMode="numeric"
							autoComplete="off"
							className="sm:max-w-48"
							placeholder={
								ceiling === null ? 'Sin límite' : formatNumber(ceiling)
							}
							aria-invalid={fieldError ? true : undefined}
							value={value}
							onChange={(e) => {
								const next = rateInput(e.target.value)
								setValue(next)
								if (checked) check(next)
							}}
							onBlur={() => {
								if (value.trim() === '') return
								setChecked(true)
								check(value)
							}}
						/>
					</FormField>
					<div className="flex flex-wrap gap-2">
						<Button type="submit" disabled={save.isPending}>
							Guardar cambios
						</Button>
						{own !== null ? (
							<Button
								type="button"
								variant="secondary"
								disabled={save.isPending}
								onClick={() => {
									setServerError(null)
									save.mutate(null)
								}}
							>
								Quitar el límite
							</Button>
						) : null}
					</div>
				</form>
			) : (
				<p className="text-ui text-muted-foreground">{now}</p>
			)}
		</Card>
	)
}
