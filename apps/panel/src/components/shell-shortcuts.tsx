import * as React from 'react'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import {
	KeySequences,
	ShortcutsDialog,
	type KeySequence,
	type ShortcutGroup,
} from '#/components/ui/keyboard-shortcuts'
import type { WorkspaceSummary } from '#/lib/api'
import { holdMoves } from '#/lib/moves'
import { HELP, isHelp, MOD, sequenceText, THEME } from '#/lib/shortcuts'

/**
 * The keys that go to each page of a space, as the panel of the first Sello
 * Sign went (`../sign-old`) and as Sello's web goes to its sections: G and
 * a letter. Shown while ⌘ is held on each row of the column (`data-hint`).
 */
export const PAGE_KEYS = {
	'/w/$workspaceId': ['g', 'd'],
	'/w/$workspaceId/organizations': ['g', 'o'],
	'/w/$workspaceId/signatures': ['g', 'f'],
	'/w/$workspaceId/verify': ['g', 'v'],
	'/w/$workspaceId/api-keys': ['g', 'k'],
	'/w/$workspaceId/members': ['g', 'm'],
	'/w/$workspaceId/activity': ['g', 'h'],
	'/w/$workspaceId/settings': ['g', 's'],
} as const satisfies Record<string, readonly string[]>

export type PagePath = keyof typeof PAGE_KEYS

/** G A, «Su cuenta», outside any space; N, a new organization. */
export const ACCOUNT_KEYS = ['g', 'a'] as const
export const NEW_ORGANIZATION_KEYS = ['n'] as const

/** W and the number of a space, in the switcher's order (nine at most). */
export const SWITCH_KEY = 'w'

/**
 * The column's keys, from anywhere on a page of a space: its pages (only
 * those its column shows: `pages`), «Su cuenta», a new organization for
 * whoever can create one, another space, and the list of them all with
 * `HELP` (⌘/ or `?`).
 */
export function ShellShortcuts({
	workspaceId,
	pages,
	canManage,
	workspaces,
}: {
	workspaceId: string
	pages: ReadonlyArray<{ to: PagePath; label: string }>
	canManage: boolean
	workspaces: ReadonlyArray<WorkspaceSummary>
}) {
	const navigate = useNavigate()
	const here = useRouterState({ select: (s) => s.location.href })
	const [helping, setHelping] = React.useState(false)
	const spaces = workspaces.slice(0, 9)

	React.useEffect(() => holdMoves(), [])

	React.useEffect(() => {
		const onKeyDown = (e: KeyboardEvent) => {
			if (e.defaultPrevented || !isHelp(e)) return
			e.preventDefault()
			setHelping((was) => !was)
		}
		window.addEventListener('keydown', onKeyDown)
		return () => window.removeEventListener('keydown', onKeyDown)
	}, [])

	const sequences: KeySequence[] = [
		...pages.map((p) => ({
			keys: PAGE_KEYS[p.to],
			run: () => void navigate({ to: p.to, params: { workspaceId } }),
		})),
		{
			keys: ACCOUNT_KEYS,
			run: () => void navigate({ to: '/account', search: { redirect: here } }),
		},
		...(canManage
			? [
					{
						keys: NEW_ORGANIZATION_KEYS,
						run: () =>
							void navigate({
								to: '/w/$workspaceId/organizations/new',
								params: { workspaceId },
							}),
					},
				]
			: []),
		...spaces.map((w, i) => ({
			keys: [SWITCH_KEY, String(i + 1)],
			run: () =>
				void navigate({ to: '/w/$workspaceId', params: { workspaceId: w.id } }),
		})),
	]

	const groups: ShortcutGroup[] = [
		{
			title: 'General',
			rows: [
				{ label: 'Cambiar entre modo claro y oscuro', keys: [THEME.text] },
				{ label: 'Ver los atajos de teclado', keys: [HELP.text] },
				...(canManage
					? [
							{
								label: 'Nueva organización',
								keys: [sequenceText(NEW_ORGANIZATION_KEYS)],
							},
						]
					: []),
			],
		},
		{
			title: 'Ir a',
			rows: [
				...pages.map((p) => ({
					label: p.label,
					keys: [sequenceText(PAGE_KEYS[p.to])],
				})),
				{ label: 'Su cuenta', keys: [sequenceText(ACCOUNT_KEYS)] },
			],
		},
		...(spaces.length > 1
			? [
					{
						title: 'Espacios',
						rows: [
							{
								label: 'Cambiar a otro, en el orden del selector',
								keys: [`${SWITCH_KEY.toUpperCase()} 1–${spaces.length}`],
							},
						],
					},
				]
			: []),
	]

	return (
		<>
			<KeySequences sequences={sequences} />
			<ShortcutsDialog
				open={helping}
				onOpenChange={setHelping}
				groups={groups}
				description={`Mantenga presionada la tecla ${MOD} para ver los atajos de los controles en pantalla. Las secuencias se escriben una tecla tras otra: G y luego D.`}
			/>
		</>
	)
}
