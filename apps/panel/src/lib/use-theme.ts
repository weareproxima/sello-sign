import { useSyncExternalStore } from 'react'
import {
	applyTheme,
	followSystem,
	readTheme,
	saveTheme,
	toggledTheme,
	type Theme,
} from '#/lib/theme'

/**
 * The chosen mode, one for the whole panel: the account menu's choice and
 * the `D` key (`toggleTheme`, from `components/page-shortcuts.tsx`) read and
 * change the same one, so choosing in one place moves the other. The cookie
 * `sello-theme` is shared with the rest of Sello, as in its web
 * (`../sello/apps/web/src/lib/theme.ts`).
 *
 * The `<head>` script already painted it before React (`THEME_SCRIPT`); here
 * it is only kept in step. While something reads it, it follows the system
 * live with `system`, and on coming back to the tab it is read again: the
 * choice may have changed on another site of Sello in the meantime. On the
 * server there is no cookie to read: it renders `system`, and the client
 * takes over on hydrating.
 */
let current: Theme = readTheme()
const listeners = new Set<() => void>()

function set(next: Theme) {
	current = next
	for (const l of listeners) l()
}

function onVisible() {
	if (document.visibilityState !== 'visible') return
	const saved = readTheme()
	if (saved === current) return
	applyTheme(saved)
	set(saved)
}

let stop: (() => void) | null = null

function subscribe(listener: () => void): () => void {
	listeners.add(listener)
	if (!stop) {
		const unfollow = followSystem(() => current)
		document.addEventListener('visibilitychange', onVisible)
		stop = () => {
			unfollow()
			document.removeEventListener('visibilitychange', onVisible)
		}
	}
	return () => {
		listeners.delete(listener)
		if (listeners.size === 0) {
			stop?.()
			stop = null
		}
	}
}

export function chooseTheme(next: Theme) {
	saveTheme(next)
	set(next)
}

/**
 * `THEME` (`D`): between the system's mode and the other one
 * (`toggledTheme`). The cookie is read and not `current`, which nobody keeps
 * in step while the menu is closed.
 */
export function toggleTheme() {
	chooseTheme(toggledTheme(readTheme()))
}

export function useTheme(): [Theme, (theme: Theme) => void] {
	const theme = useSyncExternalStore(
		subscribe,
		() => current,
		(): Theme => 'system'
	)
	return [theme, chooseTheme]
}
