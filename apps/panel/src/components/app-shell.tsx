import * as React from 'react'
import {
	Link,
	useNavigate,
	useRouter,
	useRouterState,
} from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import {
	BuildingIcon,
	CheckIcon,
	ChevronsUpDownIcon,
	FileSignatureIcon,
	HistoryIcon,
	KeyRoundIcon,
	LayoutDashboardIcon,
	LogOutIcon,
	MenuIcon,
	MonitorIcon,
	MoonIcon,
	PlusIcon,
	SettingsIcon,
	ShieldCheckIcon,
	UserIcon,
	SunIcon,
	UsersIcon,
} from 'lucide-react'
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuGroup,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from '#/components/ui/dropdown-menu'
import { Sheet, SheetContent, SheetTitle } from '#/components/ui/sheet'
import { Button } from '#/components/ui/button'
import { Logo } from '#/components/logo'
import {
	PAGE_KEYS,
	ShellShortcuts,
	type PagePath,
} from '#/components/shell-shortcuts'
import { authClient } from '#/lib/auth-client'
import { clearToken, type WorkspaceSummary } from '#/lib/api'
import { ROLE_LABELS } from '#/lib/format'
import { sequenceText, THEME } from '#/lib/shortcuts'
import type { Theme } from '#/lib/theme'
import { useTheme } from '#/lib/use-theme'
import { cn } from '#/lib/utils'

type NavItem = {
	to: PagePath
	label: string
	icon: React.ComponentType<{ className?: string }>
	exact?: boolean
}

const MAIN: NavItem[] = [
	{
		to: '/w/$workspaceId',
		label: 'Resumen',
		icon: LayoutDashboardIcon,
		exact: true,
	},
	{
		to: '/w/$workspaceId/organizations',
		label: 'Organizaciones',
		icon: BuildingIcon,
	},
	{
		to: '/w/$workspaceId/signatures',
		label: 'Registro de firmas',
		icon: FileSignatureIcon,
	},
	{
		to: '/w/$workspaceId/verify',
		label: 'Verificar una firma',
		icon: ShieldCheckIcon,
	},
]

const SPACE: NavItem[] = [
	{
		to: '/w/$workspaceId/api-keys',
		label: 'Claves de API',
		icon: KeyRoundIcon,
	},
	{ to: '/w/$workspaceId/members', label: 'Miembros', icon: UsersIcon },
	{ to: '/w/$workspaceId/activity', label: 'Actividad', icon: HistoryIcon },
	{ to: '/w/$workspaceId/settings', label: 'General', icon: SettingsIcon },
]

/** What only those who administer the space see in its column. */
const MANAGERS_ONLY: NavItem['to'][] = [
	'/w/$workspaceId/api-keys',
	'/w/$workspaceId/activity',
]

/** A row of the column: 36 px (48 on touch), `selected` where one is. */
const ROW =
	'flex h-row items-center gap-3 rounded-row px-3 text-ui text-foreground transition-colors duration-fast pointer-coarse:h-control-lg hover:bg-fill [&_svg]:size-4 [&_svg]:shrink-0 [&_svg]:text-muted-foreground'

function Nav({
	workspaceId,
	canManage,
	onNavigate,
}: {
	workspaceId: string
	canManage: boolean
	onNavigate?: () => void
}) {
	const item = (entry: NavItem) => (
		<li key={entry.to}>
			<Link
				to={entry.to}
				params={{ workspaceId }}
				activeOptions={{ exact: entry.exact ?? false }}
				activeProps={{
					className: 'bg-selected hover:bg-selected',
					'aria-current': 'page',
				}}
				className={ROW}
				// Its keys (`ShellShortcuts`), while ⌘ is held: inside the row,
				// at its end. A sequence has no `aria-keyshortcuts`.
				data-hint={sequenceText(PAGE_KEYS[entry.to])}
				data-hint-side="end"
				onClick={onNavigate}
			>
				<entry.icon />
				<span className="truncate">{entry.label}</span>
			</Link>
		</li>
	)
	return (
		<nav className="flex flex-col gap-6" aria-label="Secciones">
			<ul className="flex flex-col gap-0.5">{MAIN.map(item)}</ul>
			<div>
				<p className="px-3 pb-1 text-meta font-medium text-muted-foreground">
					Espacio
				</p>
				<ul className="flex flex-col gap-0.5">
					{(canManage
						? SPACE
						: SPACE.filter((e) => !MANAGERS_ONLY.includes(e.to))
					).map(item)}
				</ul>
			</div>
		</nav>
	)
}

function WorkspaceSwitcher({
	current,
	workspaces,
	cloud,
}: {
	current: WorkspaceSummary | null
	workspaces: WorkspaceSummary[]
	cloud: boolean
}) {
	const navigate = useNavigate()
	const name = current?.name ?? 'Espacio'
	return (
		<DropdownMenu>
			<DropdownMenuTrigger
				render={
					<button
						type="button"
						className="flex h-control-lg w-full items-center gap-3 rounded-row px-2 text-left transition-colors duration-fast hover:bg-fill aria-expanded:bg-fill"
					/>
				}
			>
				<span className="flex size-8 shrink-0 items-center justify-center rounded-row bg-surface text-ui font-medium shadow-raised">
					{name.charAt(0).toUpperCase()}
				</span>
				<span className="grid min-w-0 flex-1">
					<span className="truncate text-ui font-medium">{name}</span>
					<span className="truncate text-meta text-muted-foreground">
						{current ? ROLE_LABELS[current.role] : ''}
					</span>
				</span>
				<ChevronsUpDownIcon className="size-4 text-muted-foreground" />
			</DropdownMenuTrigger>
			<DropdownMenuContent className="min-w-60">
				<DropdownMenuGroup>
					<DropdownMenuLabel>Cambiar de espacio</DropdownMenuLabel>
					{workspaces.map((w) => (
						<DropdownMenuItem
							key={w.id}
							className={w.id === current?.id ? 'bg-selected' : undefined}
							onClick={() =>
								navigate({
									to: '/w/$workspaceId',
									params: { workspaceId: w.id },
								})
							}
						>
							<span className="flex size-6 shrink-0 items-center justify-center rounded-small bg-fill text-meta font-medium">
								{w.id === current?.id ? (
									<CheckIcon className="size-3.5" />
								) : (
									w.name.charAt(0).toUpperCase()
								)}
							</span>
							<span className="truncate">{w.name}</span>
						</DropdownMenuItem>
					))}
				</DropdownMenuGroup>
				<DropdownMenuSeparator />
				<DropdownMenuItem onClick={() => navigate({ to: '/workspaces/new' })}>
					<PlusIcon />
					{cloud ? 'Abrir otro espacio' : 'Crear un espacio'}
				</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
	)
}

function AccountMenu({
	name,
	email,
	onNavigate,
}: {
	name: string
	email: string
	onNavigate?: () => void
}) {
	const navigate = useNavigate()
	// «Volver» on «Su cuenta» comes back here.
	const here = useRouterState({ select: (s) => s.location.href })
	const router = useRouter()
	const queryClient = useQueryClient()
	// The same mode as the `D` key's (`components/page-shortcuts.tsx`).
	const [theme, chooseTheme] = useTheme()

	async function signOut() {
		await authClient.signOut()
		clearToken()
		queryClient.clear()
		await router.navigate({ to: '/sign-in' })
	}

	return (
		<DropdownMenu>
			<DropdownMenuTrigger
				render={
					<button
						type="button"
						className="flex h-control-lg w-full items-center gap-3 rounded-row px-2 text-left transition-colors duration-fast hover:bg-fill aria-expanded:bg-fill"
					/>
				}
			>
				<span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-fill text-ui font-medium">
					{(name || email).charAt(0).toUpperCase()}
				</span>
				<span className="grid min-w-0 flex-1">
					<span className="truncate text-ui font-medium">{name}</span>
					<span className="truncate text-meta text-muted-foreground">
						{email}
					</span>
				</span>
			</DropdownMenuTrigger>
			<DropdownMenuContent side="top" className="min-w-60">
				<DropdownMenuGroup>
					<DropdownMenuLabel>Tema</DropdownMenuLabel>
					<DropdownMenuRadioGroup
						value={theme}
						aria-keyshortcuts={THEME.aria}
						onValueChange={(value) => chooseTheme(value as Theme)}
					>
						<DropdownMenuRadioItem value="light">
							<SunIcon />
							Claro
						</DropdownMenuRadioItem>
						<DropdownMenuRadioItem value="dark">
							<MoonIcon />
							Oscuro
						</DropdownMenuRadioItem>
						<DropdownMenuRadioItem value="system">
							<MonitorIcon />
							Sistema
						</DropdownMenuRadioItem>
					</DropdownMenuRadioGroup>
				</DropdownMenuGroup>
				<DropdownMenuSeparator />
				<DropdownMenuItem
					onClick={() => {
						onNavigate?.()
						void navigate({ to: '/account', search: { redirect: here } })
					}}
				>
					<UserIcon />
					Su cuenta
				</DropdownMenuItem>
				<DropdownMenuItem onClick={signOut}>
					<LogOutIcon />
					Cerrar sesión
				</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
	)
}

/**
 * The panel's frame: the column is the band (`bg-subtle`), flush with the
 * page and set apart by its tone; the content is the canvas. On a phone the
 * column is a sheet.
 */
export function AppShell({
	workspaceId,
	current,
	workspaces,
	canManage,
	user,
	cloud,
	children,
}: {
	workspaceId: string
	current: WorkspaceSummary | null
	workspaces: WorkspaceSummary[]
	canManage: boolean
	user: { name: string; email: string }
	cloud: boolean
	children: React.ReactNode
}) {
	const [open, setOpen] = React.useState(false)
	const pages = [
		...MAIN,
		...(canManage ? SPACE : SPACE.filter((e) => !MANAGERS_ONLY.includes(e.to))),
	]

	const column = (onNavigate?: () => void) => (
		<div className="flex h-full flex-col gap-6 p-3">
			<WorkspaceSwitcher
				current={current}
				workspaces={workspaces}
				cloud={cloud}
			/>
			<div className="flex-1 overflow-y-auto">
				<Nav
					workspaceId={workspaceId}
					canManage={canManage}
					onNavigate={onNavigate}
				/>
			</div>
			<AccountMenu
				name={user.name}
				email={user.email}
				onNavigate={onNavigate}
			/>
		</div>
	)

	return (
		<div className="flex min-h-svh bg-background">
			<aside className="sticky top-0 hidden h-svh w-64 shrink-0 bg-subtle md:block">
				{column()}
			</aside>
			<Sheet open={open} onOpenChange={setOpen}>
				<SheetContent side="left" className="bg-subtle p-0">
					<SheetTitle className="sr-only">Menú</SheetTitle>
					{column(() => setOpen(false))}
				</SheetContent>
			</Sheet>
			<div className="flex min-w-0 flex-1 flex-col">
				<header className="flex h-16 shrink-0 items-center gap-2 px-4 md:hidden">
					<Button
						variant="ghost"
						size="icon-sm"
						aria-label="Abrir el menú"
						onClick={() => setOpen(true)}
					>
						<MenuIcon />
					</Button>
					<Logo className="h-5" />
				</header>
				<main
					className={cn(
						'mx-auto flex w-full max-w-7xl flex-1 flex-col gap-4 px-4 pt-2 pb-16 md:px-8 md:pt-10 lg:px-12'
					)}
				>
					{children}
				</main>
			</div>
			<ShellShortcuts
				workspaceId={workspaceId}
				pages={pages}
				canManage={canManage}
				workspaces={workspaces}
			/>
		</div>
	)
}
