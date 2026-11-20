import type * as React from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { FileSignatureIcon } from 'lucide-react'
import { Badge } from '#/components/ui/badge'
import { Button } from '#/components/ui/button'
import { Card } from '#/components/ui/card'
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from '#/components/ui/dialog'
import { Empty } from '#/components/empty'
import { Input } from '#/components/ui/input'
import { NativeSelect, NativeSelectOption } from '#/components/ui/native-select'
import { Skeleton } from '#/components/ui/skeleton'
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from '#/components/ui/table'
import { FormError, Mono, Page, PageHeader } from '#/components/page'
import {
	api,
	errorMessage,
	type Environment,
	type SignatureEntry,
} from '#/lib/api'
import { ENVIRONMENT_LABELS, formatDateTime, formatNumber } from '#/lib/format'
import { useWorkspace } from '#/lib/workspace'

type Search = {
	organization?: string
	environment?: Environment
	from?: string
	to?: string
	entry?: string
}

export const Route = createFileRoute('/_app/w/$workspaceId/signatures')({
	validateSearch: (s: Record<string, unknown>): Search => ({
		organization:
			typeof s.organization === 'string' ? s.organization : undefined,
		environment:
			s.environment === 'test' || s.environment === 'prod'
				? s.environment
				: undefined,
		from: typeof s.from === 'string' ? s.from : undefined,
		to: typeof s.to === 'string' ? s.to : undefined,
		entry: typeof s.entry === 'string' ? s.entry : undefined,
	}),
	head: () => ({ meta: [{ title: 'Registro de firmas — Sello Sign' }] }),
	component: Signatures,
})

/** A date of the filter (`YYYY-MM-DD`, the person's day) as an instant. */
const startOf = (day?: string) =>
	day ? new Date(`${day}T00:00:00`).toISOString() : undefined
const endOf = (day?: string) =>
	day ? new Date(`${day}T23:59:59.999`).toISOString() : undefined

function Signatures() {
	const { workspaceId } = useWorkspace()
	const search = Route.useSearch()
	const navigate = useNavigate({ from: Route.fullPath })
	const setSearch = (patch: Partial<Search>) =>
		navigate({ search: (prev) => ({ ...prev, ...patch }), replace: true })

	const organizations = useQuery({
		queryKey: ['organizations', workspaceId],
		queryFn: () => api.organizations(workspaceId),
	})
	const filters = {
		organization_id: search.organization,
		environment: search.environment,
		from: startOf(search.from),
		to: endOf(search.to),
	}
	const log = useInfiniteQuery({
		queryKey: ['signatures', workspaceId, filters],
		queryFn: ({ pageParam }) =>
			api.signatures(workspaceId, { ...filters, limit: 50, cursor: pageParam }),
		initialPageParam: undefined as string | undefined,
		getNextPageParam: (last) => last.next_cursor ?? undefined,
	})
	const entries = log.data?.pages.flatMap((p) => p.items) ?? []
	const filtered = !!(
		search.organization ||
		search.environment ||
		search.from ||
		search.to
	)

	return (
		<Page size="wide">
			<PageHeader
				title="Registro de firmas"
				description="Cada documento firmado por el espacio y, en las organizaciones propias, también lo que firmó otro espacio por delegación."
			/>
			<Card>
				<div className="flex flex-wrap items-end gap-3">
					<NativeSelect
						aria-label="Organización"
						className="min-w-56"
						value={search.organization ?? ''}
						onChange={(e) =>
							setSearch({ organization: e.target.value || undefined })
						}
					>
						<NativeSelectOption value="">
							Todas las organizaciones
						</NativeSelectOption>
						{(organizations.data ?? []).map((o) => (
							<NativeSelectOption key={o.id} value={o.id}>
								{o.name}
							</NativeSelectOption>
						))}
					</NativeSelect>
					<NativeSelect
						aria-label="Ambiente"
						value={search.environment ?? ''}
						onChange={(e) =>
							setSearch({
								environment: (e.target.value || undefined) as
									Environment | undefined,
							})
						}
					>
						<NativeSelectOption value="">
							Todos los ambientes
						</NativeSelectOption>
						<NativeSelectOption value="test">Pruebas</NativeSelectOption>
						<NativeSelectOption value="prod">Producción</NativeSelectOption>
					</NativeSelect>
					<label className="flex items-center gap-2 text-meta text-muted-foreground">
						Desde
						<Input
							type="date"
							className="w-auto"
							value={search.from ?? ''}
							onChange={(e) => setSearch({ from: e.target.value || undefined })}
						/>
					</label>
					<label className="flex items-center gap-2 text-meta text-muted-foreground">
						Hasta
						<Input
							type="date"
							className="w-auto"
							value={search.to ?? ''}
							onChange={(e) => setSearch({ to: e.target.value || undefined })}
						/>
					</label>
					{filtered ? (
						<Button
							variant="ghost"
							size="sm"
							onClick={() =>
								setSearch({
									organization: undefined,
									environment: undefined,
									from: undefined,
									to: undefined,
								})
							}
						>
							Limpiar
						</Button>
					) : null}
				</div>

				{log.isPending ? (
					<Skeleton className="h-64" />
				) : log.error ? (
					<FormError
						message={errorMessage(log.error, 'leer el registro de firmas')}
					/>
				) : entries.length === 0 ? (
					filtered ? (
						<Empty
							title="Sin resultados"
							description="Ninguna firma coincide con los filtros."
						/>
					) : (
						<Empty
							icon={<FileSignatureIcon />}
							title="Sin firmas todavía"
							description="Las firmas del espacio aparecerán aquí."
						/>
					)
				) : (
					<>
						<Table>
							<TableHeader>
								<TableRow>
									<TableHead>Organización</TableHead>
									<TableHead>Tipo</TableHead>
									<TableHead>Documento</TableHead>
									<TableHead>Ambiente</TableHead>
									<TableHead>Origen</TableHead>
									<TableHead className="text-right">Fecha</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{entries.map((entry) => (
									<TableRow
										key={entry.id}
										className="cursor-pointer hover:bg-fill"
										onClick={() => setSearch({ entry: entry.id })}
									>
										<TableCell className="font-medium">
											{entry.organization_name}
										</TableCell>
										<TableCell className="font-mono text-label">
											{entry.document_type ?? '—'}
										</TableCell>
										<TableCell className="max-w-48 truncate font-mono text-label">
											{entry.document_id ?? '—'}
										</TableCell>
										<TableCell>
											<Badge
												variant={
													entry.environment === 'prod' ? 'default' : 'secondary'
												}
											>
												{ENVIRONMENT_LABELS[entry.environment]}
											</Badge>
										</TableCell>
										<TableCell>
											<Origin entry={entry} workspaceId={workspaceId} />
										</TableCell>
										<TableCell className="text-right text-meta text-muted-foreground">
											{formatDateTime(entry.created_at)}
										</TableCell>
									</TableRow>
								))}
							</TableBody>
						</Table>
						<div className="flex items-center justify-between">
							<p className="text-meta text-muted-foreground">
								{formatNumber(entries.length)} firmas
							</p>
							{log.hasNextPage ? (
								<Button
									variant="secondary"
									size="sm"
									disabled={log.isFetchingNextPage}
									onClick={() => log.fetchNextPage()}
								>
									Cargar más
								</Button>
							) : null}
						</div>
					</>
				)}
			</Card>
			<EntryDetail
				workspaceId={workspaceId}
				id={search.entry}
				onClose={() => setSearch({ entry: undefined })}
			/>
		</Page>
	)
}

function Origin({
	entry,
	workspaceId,
}: {
	entry: SignatureEntry
	workspaceId: string
}) {
	if (entry.signed_by_workspace_id !== workspaceId)
		return <Badge variant="outline">Por delegación</Badge>
	if (entry.api_key_id)
		return <span className="text-meta text-muted-foreground">Clave de API</span>
	return <span className="text-meta text-muted-foreground">Panel</span>
}

function EntryDetail({
	workspaceId,
	id,
	onClose,
}: {
	workspaceId: string
	id?: string
	onClose: () => void
}) {
	const entry = useQuery({
		queryKey: ['signature', workspaceId, id],
		queryFn: () => api.signature(workspaceId, id!),
		enabled: !!id,
	})
	const rows: [string, React.ReactNode][] = entry.data
		? [
				['Organización', entry.data.organization_name],
				['Ambiente', ENVIRONMENT_LABELS[entry.data.environment]],
				[
					'Tipo de documento',
					<Mono key="t">{entry.data.document_type ?? '—'}</Mono>,
				],
				[
					'Id del documento',
					<Mono key="d">{entry.data.document_id ?? '—'}</Mono>,
				],
				[
					'Origen',
					entry.data.signed_by_workspace_id !== workspaceId
						? 'Otro espacio, por delegación'
						: entry.data.api_key_id
							? 'Clave de API'
							: 'Firmado desde el panel',
				],
				['Clave de API', <Mono key="k">{entry.data.api_key_id ?? '—'}</Mono>],
				[
					'Espacio que firmó',
					<Mono key="w">{entry.data.signed_by_workspace_id}</Mono>,
				],
				['Duración', `${formatNumber(entry.data.duration_ms)} ms`],
				['Fecha', formatDateTime(entry.data.created_at)],
				['Id del registro', <Mono key="i">{entry.data.id}</Mono>],
			]
		: []
	return (
		<Dialog open={!!id} onOpenChange={(open) => !open && onClose()}>
			<DialogContent className="sm:max-w-lg">
				<DialogHeader>
					<DialogTitle>Detalle de la firma</DialogTitle>
					<DialogDescription>
						Lo que el registro guarda de la operación. El documento no se
						guarda.
					</DialogDescription>
				</DialogHeader>
				{entry.isPending ? (
					<Skeleton className="h-48" />
				) : entry.error ? (
					<FormError message={errorMessage(entry.error, 'leer la firma')} />
				) : (
					<dl className="divide-y divide-line">
						{rows.map(([label, value]) => (
							<div
								key={label}
								className="grid grid-cols-[10rem_1fr] gap-3 py-2 text-ui"
							>
								<dt className="text-muted-foreground">{label}</dt>
								<dd className="min-w-0 break-all">{value}</dd>
							</div>
						))}
					</dl>
				)}
			</DialogContent>
		</Dialog>
	)
}
