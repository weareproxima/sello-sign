import { useSyncExternalStore } from 'react'

/**
 * Whether a page with the column is mounted (`AppShell`), the one place
 * where there are pages to move to (`components/shell-shortcuts.tsx`). The
 * root's `PageShortcuts` reads it to say, at the top of the held ⌘, the list
 * of shortcuts there, and the `D` everywhere else.
 */
let mounted = 0
const listeners = new Set<() => void>()

function changed() {
	for (const l of listeners) l()
}

/** Marks a page with the column while the caller is mounted. */
export function holdMoves(): () => void {
	mounted++
	changed()
	return () => {
		mounted--
		changed()
	}
}

export function useMoves(): boolean {
	return useSyncExternalStore(
		(listener) => {
			listeners.add(listener)
			return () => listeners.delete(listener)
		},
		() => mounted > 0,
		() => false
	)
}
