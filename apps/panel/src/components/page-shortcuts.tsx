import { useEffect } from 'react'
import { ShortcutHints } from '#/components/ui/shortcut-hints'
import { useMoves } from '#/lib/moves'
import { HELP, isTheme, THEME } from '#/lib/shortcuts'
import { toggleTheme } from '#/lib/use-theme'

/**
 * What every page shares with the keyboard of the rest of Sello —its chat, its
 * web and its account center— (`DESIGN.md`, «Atajos visibles»): `D` switches
 * the color mode, from anywhere and not while typing, and holding ⌘ (Ctrl
 * elsewhere) shows the keys of the controls in view that declare one with
 * `data-hint`. At the top, the pill says the list of shortcuts on a page of
 * a space (`components/shell-shortcuts.tsx`), and the `D`, the one key that
 * doesn't belong to a control in view, everywhere else.
 */
export function PageShortcuts() {
	// On a page of a space, the list of every shortcut (`ShellShortcuts`).
	const moves = useMoves()
	useEffect(() => {
		const onKeyDown = (e: KeyboardEvent) => {
			if (e.defaultPrevented || !isTheme(e)) return
			e.preventDefault()
			toggleTheme()
		}
		window.addEventListener('keydown', onKeyDown)
		return () => window.removeEventListener('keydown', onKeyDown)
	}, [])
	return (
		<ShortcutHints
			summary={
				moves
					? { keys: HELP.text, label: 'Todos los atajos' }
					: { keys: THEME.text, label: 'Cambiar entre modo claro y oscuro' }
			}
		/>
	)
}
