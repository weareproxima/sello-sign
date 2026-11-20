import * as React from 'react'
import { createFileRoute } from '@tanstack/react-router'
import {
	CircleAlertIcon,
	CircleCheckIcon,
	CircleXIcon,
	ShieldCheckIcon,
	UploadIcon,
} from 'lucide-react'
import { Button } from '#/components/ui/button'
import {
	Card,
	CardDescription,
	CardHeader,
	CardTitle,
} from '#/components/ui/card'
import { Textarea } from '#/components/ui/textarea'
import { Empty } from '#/components/empty'
import {
	CountryCode,
	FormField,
	Mono,
	Page,
	PageHeader,
} from '#/components/page'
import { countryName } from '#/lib/countries'
import {
	verifySignatureClient,
	type ClientVerifyResult,
} from '#/lib/verify-client'
import { cn } from '#/lib/utils'

/**
 * Verifying a signature, in the browser: the contract's `POST /verifications`
 * takes an API key, not the panel's token, so the panel runs the verifier of
 * Sello's site (`sello.tax/sign/verify`), a byte-for-byte copy of
 * ../sign-old's (`lib/verify-client.ts`). Neither the document nor the
 * certificate leave the browser.
 */
export const Route = createFileRoute('/_app/w/$workspaceId/verify')({
	head: () => ({ meta: [{ title: 'Verificar una firma — Sello Sign' }] }),
	component: Verify,
})

/** A JWS (El Salvador): three base64url parts. Its key isn't inside. */
const isJwt = (value: string) => /^[\w-]+\.[\w-]+\.[\w-]+$/.test(value.trim())

function Verify() {
	const [signed, setSigned] = React.useState('')
	const [certificate, setCertificate] = React.useState('')
	const [result, setResult] = React.useState<ClientVerifyResult | null>(null)
	const [verifying, setVerifying] = React.useState(false)
	const file = React.useRef<HTMLInputElement>(null)
	const jwt = isJwt(signed)
	const needsCertificate = jwt && !certificate.trim()

	async function verify() {
		setVerifying(true)
		setResult(null)
		try {
			setResult(
				await verifySignatureClient(
					signed.trim(),
					certificate.trim() || undefined
				)
			)
		} catch (error) {
			setResult({
				valid: false,
				payload: null,
				error: error instanceof Error ? error.message : String(error),
			})
		} finally {
			setVerifying(false)
		}
	}

	return (
		<Page>
			<PageHeader
				title="Verificar una firma"
				description="Pegue el JWT o el XML firmado. Se verifica en su navegador: ni el documento ni el certificado salen de él."
			/>
			<div className="grid items-start gap-6 lg:grid-cols-2">
				<Card>
					<form
						className="space-y-4"
						onSubmit={(e) => {
							e.preventDefault()
							void verify()
						}}
					>
						<FormField
							label="Documento firmado"
							htmlFor="document"
							help="Un JWT de El Salvador o el XML firmado de Costa Rica, Guatemala, Panamá o República Dominicana."
						>
							<Textarea
								id="document"
								required
								spellCheck={false}
								className="max-h-80 min-h-48 overflow-y-auto font-mono text-label break-all"
								placeholder="eyJhbGciOiJSUzUxMiJ9… o <?xml …"
								value={signed}
								onChange={(e) => {
									setSigned(e.target.value)
									setResult(null)
								}}
							/>
						</FormField>
						<FormField
							label="Certificado"
							htmlFor="certificate"
							help={
								jwt
									? 'Un JWT de El Salvador no lleva el certificado: pegue el del emisor (.crt del MH, .pem o .cer) o súbalo.'
									: 'Opcional: un XML firmado lleva el certificado dentro de la firma.'
							}
							aside={
								<Button
									type="button"
									variant="ghost"
									size="sm"
									onClick={() => file.current?.click()}
								>
									<UploadIcon />
									Subir
								</Button>
							}
						>
							<Textarea
								id="certificate"
								spellCheck={false}
								className="max-h-48 min-h-24 overflow-y-auto font-mono text-label break-all"
								placeholder="-----BEGIN CERTIFICATE-----"
								value={certificate}
								onChange={(e) => {
									setCertificate(e.target.value)
									setResult(null)
								}}
							/>
							<input
								ref={file}
								type="file"
								accept=".crt,.pem,.cer,.xml"
								className="hidden"
								onChange={async (e) => {
									const chosen = e.target.files?.[0]
									e.target.value = ''
									if (!chosen) return
									setCertificate(await chosen.text())
									setResult(null)
								}}
							/>
						</FormField>
						<div className="flex flex-wrap gap-2">
							<Button
								type="submit"
								disabled={verifying || !signed.trim() || needsCertificate}
							>
								{verifying ? 'Verificando…' : 'Verificar la firma'}
							</Button>
							{signed || certificate ? (
								<Button
									type="button"
									variant="ghost"
									onClick={() => {
										setSigned('')
										setCertificate('')
										setResult(null)
									}}
								>
									Limpiar
								</Button>
							) : null}
						</div>
					</form>
				</Card>
				<Card>
					<CardHeader>
						<CardTitle>Resultado</CardTitle>
					</CardHeader>
					{result ? (
						<Result result={result} />
					) : (
						<Empty
							icon={<ShieldCheckIcon />}
							title="Sin verificar todavía"
							description="El resultado aparecerá aquí."
						/>
					)}
				</Card>
			</div>
		</Page>
	)
}

/** The verifier's error when a reference's digest doesn't match. */
const ALTERED = 'XML digest mismatch'

function Result({ result }: { result: ClientVerifyResult }) {
	// A digest that doesn't match is an answer, not a failure to read: the
	// document changed after it was signed.
	const altered = result.error === ALTERED
	const verdict =
		result.error && !altered
			? {
					icon: <CircleAlertIcon />,
					title: 'No se pudo verificar la firma.',
					description:
						'El documento no tiene una firma que se pueda leer, o falta el certificado.',
					tone: 'bg-warn-fill text-warn',
				}
			: result.valid && !altered
				? {
						icon: <CircleCheckIcon />,
						title: 'Firma válida',
						description: 'El documento no fue alterado después de firmarlo.',
						tone: 'bg-ok-fill text-ok',
					}
				: {
						icon: <CircleXIcon />,
						title: 'Firma inválida',
						description:
							'No coincide con el certificado, o el documento fue alterado.',
						tone: 'bg-bad-fill text-bad',
					}
	const rows: [string, React.ReactNode][] = [
		...(result.country
			? [
					[
						'País',
						<span key="c" className="flex items-center gap-2">
							{countryName(result.country)}
							<CountryCode code={result.country} />
						</span>,
					] as [string, React.ReactNode],
				]
			: []),
		...(result.kind
			? [
					[
						'Formato',
						result.kind === 'jwt' ? 'JWS (JWT RS512)' : 'XML firmado',
					] as [string, React.ReactNode],
				]
			: []),
		...(result.documentType
			? [
					['Tipo de documento', <Mono key="t">{result.documentType}</Mono>] as [
						string,
						React.ReactNode,
					],
				]
			: []),
		...(result.documentId
			? [
					['Id del documento', <Mono key="d">{result.documentId}</Mono>] as [
						string,
						React.ReactNode,
					],
				]
			: []),
	]
	const payload =
		result.payload == null
			? null
			: typeof result.payload === 'string'
				? result.payload
				: JSON.stringify(result.payload, null, 2)

	return (
		<div className="space-y-4" aria-live="polite">
			<div
				className={cn(
					'flex items-start gap-3 rounded-panel px-4 py-3 [&_svg]:mt-0.5 [&_svg]:size-5 [&_svg]:shrink-0',
					verdict.tone
				)}
			>
				{verdict.icon}
				<div>
					<p className="text-ui font-medium">{verdict.title}</p>
					<p className="text-meta">{verdict.description}</p>
				</div>
			</div>
			{rows.length ? (
				<dl className="divide-y divide-line">
					{rows.map(([label, value]) => (
						<div
							key={label}
							className="grid grid-cols-[9rem_1fr] gap-3 py-2 text-ui"
						>
							<dt className="text-muted-foreground">{label}</dt>
							<dd className="min-w-0 break-all">{value}</dd>
						</div>
					))}
				</dl>
			) : null}
			{result.error ? (
				<CardDescription>
					Detalle técnico: <Mono>{result.error}</Mono>
				</CardDescription>
			) : null}
			{payload ? (
				<details className="group">
					<summary className="cursor-pointer text-ui font-medium">
						Contenido del documento
					</summary>
					<pre className="mt-3 max-h-96 overflow-auto rounded-panel bg-fill p-4 font-mono text-label whitespace-pre-wrap break-all">
						{payload}
					</pre>
				</details>
			) : null}
		</div>
	)
}
