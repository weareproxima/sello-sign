import type * as React from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { HistoryIcon } from 'lucide-react'
import { Button } from '#/components/ui/button'
import { Card } from '#/components/ui/card'
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from '#/components/ui/dialog'
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
import { Empty } from '#/components/empty'
import { TableCards } from '#/components/table-cards'
import { FormError, Mono, Page, PageHeader } from '#/components/page'
import {
	AUDIT_TARGET_TYPES,
	api,
	errorMessage,
	type AuditEvent,
	type AuditTargetType,
} from '#/lib/api'
import {
	ACTOR_FILTERS,
	AUDIT_RETENTION_DAYS,
	TARGET_LABELS,
	TARGET_NAMES,
	actorFilter,
	actorOf,
	changeLines,
	describeEvent,
	detailRows,
	endOfDay,
	startOfDay,
} from '#/lib/audit'
import { formatDateTime, formatNumber } from '#/lib/format'
import { useWorkspace } from '#/lib/workspace'

type Search = {
	/** What the events are about (`target_type`). */
	type?: AuditTargetType
	/** Who: `user:<id>`, or an actor type (`lib/audit.ts`). */
	actor?: string
	from?: string
	to?: string
	/** The event open in the dialog. */
	event?: string
}

export const Route = createFileRoute('/_app/w/$workspaceId/activity')({
	validateSearch: (s: Record<string, unknown>): Search => ({
		type: (AUDIT_TARGET_TYPES as readonly unknown[]).includes(s.type)
			? (s.type as AuditTargetType)
			: undefined,
		actor: typeof s.actor === 'string' ? s.actor : undefined,
		from: typeof s.from === 'string' ? s.from : undefined,
		to: typeof s.to === 'string' ? s.to : undefined,
		event: typeof s.event === 'string' ? s.event : undefined,
	}),
	head: () => ({ meta: [{ title: 'Actividad — Sello Sign' }] }),
	component: Activity,
})

function Activity() {
	const { workspaceId, role, canManage } = useWorkspace()
	const search = Route.useSearch()
	const navigate = useNavigate({ from: Route.fullPath })
	const setSearch = (patch: Partial<Search>) =>
		navigate({ search: (prev) => ({ ...prev, ...patch }), replace: true })

	const members = useQuery({
		queryKey: ['members', workspaceId],
		queryFn: () => api.members(workspaceId),
		enabled: canManage,
	})
	const filters = {
		target_type: search.type,
		...actorFilter(search.actor),
		from: startOfDay(search.from),
		to: endOfDay(search.to),
	}
	const log = useInfiniteQuery({
		queryKey: ['audit-events', workspaceId, filters],
		queryFn: ({ pageParam }) =>
			api.auditEvents(workspaceId, {
				...filters,
				limit: 50,
				cursor: pageParam,
			}),
		initialPageParam: undefined as string | undefined,
		getNextPageParam: (last) => last.next_cursor ?? undefined,
		enabled: canManage,
	})
	const events = log.data?.pages.flatMap((p) => p.items) ?? []
	const open = events.find((e) => e.id === search.event) ?? null
	const filtered = !!(search.type || search.actor || search.from || search.to)

	const header = (
		<PageHeader
			title="Actividad"
			description={`Quién cambió qué en el espacio en los últimos ${AUDIT_RETENTION_DAYS} días. Las firmas están en «Registro de firmas».`}
		/>
	)
	if (role && !canManage)
		return (
			<Page size="wide">
				{header}
				<Empty
					icon={<HistoryIcon />}
					title="Solo para quien administra el espacio"
					description="La actividad del espacio la ven sus propietarios y sus administradores."
				/>
			</Page>
		)

	return (
		<Page size="wide">
			{header}
			<Card>
				<div className="flex flex-wrap items-end gap-3">
					<NativeSelect
						aria-label="Qué"
						className="min-w-48"
						value={search.type ?? ''}
						onChange={(e) =>
							setSearch({
								type: (e.target.value || undefined) as
									AuditTargetType | undefined,
							})
						}
					>
						<NativeSelectOption value="">Todo</NativeSelectOption>
						{AUDIT_TARGET_TYPES.map((type) => (
							<NativeSelectOption key={type} value={type}>
								{TARGET_LABELS[type]}
							</NativeSelectOption>
						))}
					</NativeSelect>
					<NativeSelect
						aria-label="Quién"
						className="min-w-56"
						value={search.actor ?? ''}
						onChange={(e) => setSearch({ actor: e.target.value || undefined })}
					>
						<NativeSelectOption value="">Todos</NativeSelectOption>
						{(members.data ?? [])
							.filter((m) => m.user_id)
							.map((m) => (
								<NativeSelectOption key={m.user_id} value={`user:${m.user_id}`}>
									{m.name || m.email || m.user_id}
								</NativeSelectOption>
							))}
						{ACTOR_FILTERS.map((f) => (
							<NativeSelectOption key={f.value} value={f.value}>
								{f.label}
							</NativeSelectOption>
						))}
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
									type: undefined,
									actor: undefined,
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
						message={errorMessage(log.error, 'leer la actividad del espacio')}
					/>
				) : events.length === 0 ? (
					filtered ? (
						<Empty
							title="Sin resultados"
							description="Ningún cambio coincide con los filtros."
						/>
					) : (
						<Empty
							icon={<HistoryIcon />}
							title="Sin actividad todavía"
							description="Los cambios al espacio aparecerán aquí."
						/>
					)
				) : (
					<>
						<TableCards>
							<Table>
								<TableHeader>
									<TableRow>
										<TableHead>Qué</TableHead>
										<TableHead>Quién</TableHead>
										<TableHead className="text-right">Fecha</TableHead>
									</TableRow>
								</TableHeader>
								<TableBody>
									{events.map((event) => (
										<EventRow
											key={event.id}
											event={event}
											workspaceId={workspaceId}
											onOpen={() => setSearch({ event: event.id })}
										/>
									))}
								</TableBody>
							</Table>
						</TableCards>
						<div className="flex items-center justify-between">
							<p className="text-meta text-muted-foreground">
								{formatNumber(events.length)}{' '}
								{events.length === 1 ? 'cambio' : 'cambios'}
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
			<EventDetail
				event={open}
				workspaceId={workspaceId}
				onClose={() => setSearch({ event: undefined })}
			/>
		</Page>
	)
}

function EventRow({
	event,
	workspaceId,
	onOpen,
}: {
	event: AuditEvent
	workspaceId: string
	onOpen: () => void
}) {
	const actor = actorOf(event)
	// A role's change already says both roles in its sentence.
	const lines = event.action === 'member.role_changed' ? [] : changeLines(event)
	return (
		<TableRow className="cursor-pointer hover:bg-fill" onClick={onOpen}>
			<TableCell className="max-w-xl whitespace-normal">
				<p className="font-medium text-pretty">
					{describeEvent(event, workspaceId)}
				</p>
				{lines.slice(0, 2).map((l) => (
					<p
						key={l.field}
						className="text-meta break-words text-muted-foreground"
					>
						{l.label}: {l.from} → {l.to}
					</p>
				))}
				{lines.length > 2 ? (
					<p className="text-meta text-muted-foreground">
						y {formatNumber(lines.length - 2)} más
					</p>
				) : null}
			</TableCell>
			<TableCell data-label="Quién" className="whitespace-normal">
				{/* One block: on a phone the card puts it across from its label. */}
				<div>
					<p className="break-words">{actor.name}</p>
					{actor.detail ? (
						<p className="text-meta break-words text-muted-foreground">
							{actor.detail}
						</p>
					) : null}
				</div>
			</TableCell>
			<TableCell
				data-label="Fecha"
				className="text-right text-meta whitespace-nowrap text-muted-foreground"
			>
				{formatDateTime(event.created_at)}
			</TableCell>
		</TableRow>
	)
}

function EventDetail({
	event,
	workspaceId,
	onClose,
}: {
	event: AuditEvent | null
	workspaceId: string
	onClose: () => void
}) {
	const rows: [string, React.ReactNode][] = []
	if (event) {
		const actor = actorOf(event)
		rows.push(
			[
				'Quién',
				<span key="a">
					{actor.name}
					{actor.detail ? (
						<span className="block text-meta text-muted-foreground">
							{actor.detail}
						</span>
					) : null}
				</span>,
			],
			['Fecha', formatDateTime(event.created_at)],
			[
				TARGET_NAMES[event.target.type] ?? 'Sobre',
				<span key="t">
					{event.target.label || '—'}
					{event.target.id ? (
						<Mono className="block text-muted-foreground">
							{event.target.id}
						</Mono>
					) : null}
				</span>,
			]
		)
		for (const l of changeLines(event))
			rows.push([
				l.label,
				<span key={l.field}>
					{l.from} → {l.to}
				</span>,
			])
		rows.push(...detailRows(event))
		rows.push(
			['Acción', <Mono key="c">{event.action}</Mono>],
			['Id del evento', <Mono key="i">{event.id}</Mono>]
		)
	}
	return (
		<Dialog open={!!event} onOpenChange={(open) => !open && onClose()}>
			<DialogContent className="sm:max-w-lg">
				<DialogHeader>
					<DialogTitle>Detalle del cambio</DialogTitle>
					<DialogDescription>
						{event ? describeEvent(event, workspaceId) : ''}
					</DialogDescription>
				</DialogHeader>
				<dl className="divide-y divide-line">
					{rows.map(([label, value], i) => (
						<div
							key={`${i}-${label}`}
							className="grid grid-cols-[10rem_1fr] gap-3 py-2 text-ui"
						>
							<dt className="text-muted-foreground">{label}</dt>
							<dd className="min-w-0 break-words">{value}</dd>
						</div>
					))}
				</dl>
			</DialogContent>
		</Dialog>
	)
}
