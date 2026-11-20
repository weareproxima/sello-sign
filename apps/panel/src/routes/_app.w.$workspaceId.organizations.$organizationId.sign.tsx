import * as React from 'react'
import { Link, createFileRoute } from '@tanstack/react-router'
import { useMutation } from '@tanstack/react-query'
import { CircleCheckIcon } from 'lucide-react'
import { Button } from '#/components/ui/button'
import {
	Card,
	CardDescription,
	CardHeader,
	CardTitle,
} from '#/components/ui/card'
import { Textarea } from '#/components/ui/textarea'
import { CopyButton } from '#/components/copy-button'
import { EnvironmentPicker } from '#/components/environment-picker'
import { FormError, FormField, Mono } from '#/components/page'
import { api, errorMessage, type Environment } from '#/lib/api'
import { buttonVariants } from '#/lib/button-variants'
import { documentFormat } from '#/lib/countries'
import { checkDocument } from '#/lib/documents'
import { CODE_BLOCK } from '#/lib/recipes'
import { useOrganization } from '#/lib/organization'

export const Route = createFileRoute(
	'/_app/w/$workspaceId/organizations/$organizationId/sign'
)({
	head: () => ({ meta: [{ title: 'Firmar de prueba — Sello Sign' }] }),
	component: SignTest,
})

function SignTest() {
	const { workspaceId, organizationId, data: organization } = useOrganization()
	const format = organization ? documentFormat(organization.country) : 'json'
	const [environment, setEnvironment] = React.useState<Environment>('test')
	const [document, setDocument] = React.useState('')
	const [invalid, setInvalid] = React.useState<string | null>(null)
	const sign = useMutation({
		mutationFn: () =>
			api.sign(
				workspaceId,
				organizationId,
				environment,
				document.trim(),
				format === 'json' ? 'application/json' : 'application/xml'
			),
	})

	function submit(event: React.FormEvent) {
		event.preventDefault()
		const problem = checkDocument(document, format)
		setInvalid(problem)
		if (!problem) sign.mutate()
	}

	function formatJson() {
		try {
			setDocument(JSON.stringify(JSON.parse(document), null, 2))
		} catch {
			setInvalid(
				'El documento debe ser un JSON válido. Revise su estructura y vuelva a intentarlo.'
			)
		}
	}

	return (
		<div className="grid gap-4 lg:grid-cols-2">
			<Card>
				<CardHeader>
					<CardTitle>Firmar un documento</CardTitle>
					<CardDescription>
						Con el certificado activo del ambiente. La firma queda en el
						registro, como las del API.{' '}
						{format === 'json'
							? 'En El Salvador, el DTE en JSON.'
							: 'El documento en XML.'}
					</CardDescription>
				</CardHeader>
				<form className="space-y-4" onSubmit={submit}>
					<FormError
						message={
							invalid ??
							(sign.error
								? errorMessage(sign.error, 'firmar el documento')
								: null)
						}
					/>
					<FormField label="Ambiente">
						<EnvironmentPicker value={environment} onChange={setEnvironment} />
					</FormField>
					<FormField
						label="Documento"
						htmlFor="document"
						aside={
							format === 'json' ? (
								<Button
									type="button"
									variant="ghost"
									size="xs"
									onClick={formatJson}
								>
									Formatear JSON
								</Button>
							) : null
						}
					>
						<Textarea
							id="document"
							rows={16}
							spellCheck={false}
							className="font-mono text-meta"
							placeholder="Pegue aquí el documento que va a firmar"
							value={document}
							onChange={(e) => setDocument(e.target.value)}
						/>
					</FormField>
					<Button type="submit" disabled={sign.isPending}>
						Firmar
					</Button>
				</form>
			</Card>

			<Card>
				<CardHeader>
					<CardTitle>Firma</CardTitle>
					<CardDescription>
						{sign.data
							? 'El documento firmado, tal como lo devuelve el API.'
							: 'La firma del documento aparecerá aquí después de firmar.'}
					</CardDescription>
				</CardHeader>
				{sign.data ? (
					<div className="space-y-4">
						<p className="flex items-center gap-2 text-ui">
							<CircleCheckIcon className="size-4 text-ok" />
							Firmado
							{sign.data.document_type ? (
								<Mono className="text-muted-foreground">
									{sign.data.document_type}
								</Mono>
							) : null}
							{sign.data.document_id ? (
								<Mono className="text-muted-foreground">
									{sign.data.document_id}
								</Mono>
							) : null}
						</p>
						<pre className={`${CODE_BLOCK} max-h-96`}>
							{sign.data.signature}
						</pre>
						<div className="flex flex-wrap gap-2">
							<CopyButton value={sign.data.signature} />
							<Link
								to="/w/$workspaceId/signatures"
								params={{ workspaceId }}
								search={{ organization: organizationId }}
								className={buttonVariants({ variant: 'ghost', size: 'sm' })}
							>
								Ver en el registro
							</Link>
						</div>
					</div>
				) : null}
			</Card>
		</div>
	)
}
