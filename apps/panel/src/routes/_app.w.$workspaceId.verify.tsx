import * as React from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useMutation, useQuery } from '@tanstack/react-query'
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
import { NativeSelect, NativeSelectOption } from '#/components/ui/native-select'
import { Skeleton } from '#/components/ui/skeleton'
import { Textarea } from '#/components/ui/textarea'
import { Empty } from '#/components/empty'
import { EnvironmentPicker } from '#/components/environment-picker'
import { ActionError } from '#/components/limit-alert'
import {
	CountryCode,
	FormField,
	Mono,
	Page,
	PageHeader,
} from '#/components/page'
import { api, type Environment, type Organization } from '#/lib/api'
import { countryName } from '#/lib/countries'
import { ENVIRONMENT_LABELS, formatDate } from '#/lib/format'
import { SEGMENT, SEGMENTED } from '#/lib/recipes'
import {
	verifySignatureClient,
	type ClientVerifyCountry,
	type ClientVerifyResult,
} from '#/lib/verify-client'
import { cn } from '#/lib/utils'
import { useWorkspace } from '#/lib/workspace'

/**
 * Verifying a signature, two ways. With an organization of the space, by
 * the API (`POST /verifications` with the panel's token): against its
 * active certificate in an environment, so a JWS of El Salvador needs no
 * certificate pasted; the API processes it in memory and keeps nothing. Or,
 * without sending anything, in the browser, with the verifier of Sello's
 * site (`sello.tax/sign/verify`), a byte-for-byte copy of ../sign-old's
 * (`lib/verify-client.ts`).
 */
export const Route = createFileRoute('/_app/w/$workspaceId/verify')({
	head: () => ({ meta: [{ title: 'Verificar una firma — Sello Sign' }] }),
	component: Verify,
})

/** A JWS (El Salvador): three base64url parts. Its key isn't inside. */
const isJwt = (value: string) => /^[\w-]+\.[\w-]+\.[\w-]+$/.test(value.trim())

type Mode = 'api' | 'browser'

const CLIENT_COUNTRIES: ClientVerifyCountry[] = ['SV', 'DO', 'CR', 'PA', 'GT']

/** The organizations whose certificate the person may verify with: the
 * space's own, and delegated ones that sign or read certificates. */
function verifiable(orgs: Organization[]): Organization[] {
	return orgs.filter(
		(o) =>
			o.relation === 'owned' ||
			o.delegation?.scopes.some(
				(s) => s === 'sign' || s === 'certificates:read'
			)
	)
}

/** The API's answer as the result the page shows. */
function fromApi(
	result: {
		valid: boolean
		document: string | null
		document_id: string
		document_type: string
	},
	signed: string,
	country: string
): ClientVerifyResult {
	let payload: unknown = result.document
	if (result.document && isJwt(signed)) {
		try {
			payload = JSON.parse(result.document)
		} catch {
			payload = result.document
		}
	}
	return {
		valid: result.valid,
		payload,
		kind: isJwt(signed) ? 'jwt' : 'xml',
		country: CLIENT_COUNTRIES.find((c) => c === country),
		documentId: result.document_id || null,
		documentType: result.document_type || null,
	}
}

function Verify() {
	const { workspaceId } = useWorkspace()
	const organizations = useQuery({
		queryKey: ['organizations', workspaceId],
		queryFn: () => api.organizations(workspaceId),
	})
	const usable = verifiable(organizations.data ?? [])
	const [chosen, setChosen] = React.useState<Mode | null>(null)
	// With an organization, the API by default; without one, the browser.
	const mode: Mode = usable.length === 0 ? 'browser' : (chosen ?? 'api')

	const [signed, setSigned] = React.useState('')
	const [certificate, setCertificate] = React.useState('')
	const [organizationId, setOrganizationId] = React.useState('')
	const [environment, setEnvironment] = React.useState<Environment>('test')
	const [result, setResult] = React.useState<ClientVerifyResult | null>(null)
	const file = React.useRef<HTMLInputElement>(null)
	const organization =
		usable.find((o) => o.id === organizationId) ?? usable[0] ?? null
	const jwt = isJwt(signed)
	const needsCertificate = mode === 'browser' && jwt && !certificate.trim()

	const verify = useMutation({
		mutationFn: async (): Promise<ClientVerifyResult> => {
			if (mode === 'api' && organization) {
				const answer = await api.verify(
					organization.id,
					environment,
					signed.trim()
				)
				return fromApi(answer, signed, organization.country)
			}
			try {
				return await verifySignatureClient(
					signed.trim(),
					certificate.trim() || undefined
				)
			} catch (error) {
				return {
					valid: false,
					payload: null,
					error: error instanceof Error ? error.message : String(error),
				}
			}
		},
		onMutate: () => setResult(null),
		onSuccess: setResult,
	})
	const clear = () => {
		setResult(null)
		verify.reset()
	}
	const active = organization?.certificates[environment] ?? null

	return (
		<Page>
			<PageHeader
				title="Verificar una firma"
				description={
					mode === 'api'
						? 'Pegue el JWT o el XML firmado. Sello Sign lo verifica con el certificado de la organización, en memoria y sin guardarlo.'
						: 'Pegue el JWT o el XML firmado. Se verifica en su navegador: ni el documento ni el certificado salen de él.'
				}
			/>
			<div className="grid items-start gap-6 lg:grid-cols-2">
				<Card>
					{organizations.isPending ? (
						<Skeleton className="h-9 w-72" />
					) : usable.length > 0 ? (
						<div
							className={cn(SEGMENTED, 'w-fit max-w-full')}
							role="radiogroup"
							aria-label="Cómo verificar"
						>
							{(
								[
									['api', 'En Sello Sign'],
									['browser', 'En su navegador'],
								] as const
							).map(([value, label]) => (
								<button
									key={value}
									type="button"
									role="radio"
									aria-checked={mode === value}
									className={cn(SEGMENT, 'h-8 px-4 text-ui whitespace-nowrap')}
									onClick={() => {
										setChosen(value)
										clear()
									}}
								>
									{label}
								</button>
							))}
						</div>
					) : null}
					<form
						className="space-y-4"
						onSubmit={(e) => {
							e.preventDefault()
							verify.mutate()
						}}
					>
						<ActionError error={verify.error} action="verificar la firma" />
						{mode === 'api' && organization ? (
							<>
								<FormField
									label="Organización"
									htmlFor="organization"
									help={
										active
											? `Se verifica con su certificado activo de ${ENVIRONMENT_LABELS[environment]}${
													active.not_after
														? `, que vence el ${formatDate(active.not_after)}`
														: ''
												}.`
											: `No tiene un certificado activo de ${ENVIRONMENT_LABELS[environment]}.`
									}
								>
									<NativeSelect
										id="organization"
										className="w-full"
										value={organization.id}
										onChange={(e) => {
											setOrganizationId(e.target.value)
											clear()
										}}
									>
										{usable.map((o) => (
											<NativeSelectOption key={o.id} value={o.id}>
												{`${o.name} (${o.country})`}
											</NativeSelectOption>
										))}
									</NativeSelect>
								</FormField>
								<FormField label="Ambiente">
									<EnvironmentPicker
										value={environment}
										onChange={(env) => {
											setEnvironment(env)
											clear()
										}}
									/>
								</FormField>
							</>
						) : null}
						<FormField
							label="Documento firmado"
							htmlFor="document"
							help={
								mode === 'api'
									? 'El JWT de El Salvador o el XML firmado de los demás países.'
									: 'Un JWT de El Salvador o el XML firmado de Costa Rica, Guatemala, Panamá o República Dominicana.'
							}
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
									clear()
								}}
							/>
						</FormField>
						{mode === 'browser' ? (
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
										clear()
									}}
								/>
								<input
									ref={file}
									type="file"
									accept=".crt,.pem,.cer,.xml"
									className="hidden"
									onChange={async (e) => {
										const chosenFile = e.target.files?.[0]
										e.target.value = ''
										if (!chosenFile) return
										setCertificate(await chosenFile.text())
										clear()
									}}
								/>
							</FormField>
						) : null}
						<div className="flex flex-wrap gap-2">
							<Button
								type="submit"
								disabled={
									verify.isPending || !signed.trim() || needsCertificate
								}
							>
								{verify.isPending ? 'Verificando…' : 'Verificar la firma'}
							</Button>
							{signed || certificate ? (
								<Button
									type="button"
									variant="ghost"
									onClick={() => {
										setSigned('')
										setCertificate('')
										clear()
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
