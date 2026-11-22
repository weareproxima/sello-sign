/**
 * El modo de color: claro, oscuro o el del sistema.
 *
 * Una sola fuente para el landing y el chat. Viven en orígenes distintos
 * (`sello.tax` y `chat.sello.tax`) y no comparten `localStorage`, así que la
 * elección se guarda en una cookie del sitio (`Domain=sello.tax`), que los dos
 * leen. Sin elección guardada manda el sistema.
 *
 * El modo se aplica con la clase `dark` en `<html>` (los tokens de
 * `styles/tokens.css` cambian con ella). Para que no haya un destello de claro
 * antes del oscuro, cada app pone `THEME_SCRIPT` en línea en su `<head>`: corre
 * antes del primer pintado. Ver «Modo oscuro» en `DESIGN.md`.
 */

import { cookieDomain } from './cookie'
import { THEME_COLOR, THEME_COOKIE } from './theme-script'

export type Theme = 'light' | 'dark' | 'system'

export { THEME_COLOR, THEME_COOKIE, THEME_SCRIPT } from './theme-script'

/** La elección guardada; `system` si no hay ninguna. */
export function readTheme(): Theme {
	if (typeof document === 'undefined') return 'system'
	const match = new RegExp(`(?:^|; )${THEME_COOKIE}=(light|dark)`).exec(
		document.cookie
	)
	return (match?.[1] as Theme | undefined) ?? 'system'
}

/** Si ese modo, aquí y ahora, es oscuro. */
export function isDark(theme: Theme): boolean {
	return (
		theme === 'dark' ||
		(theme === 'system' &&
			window.matchMedia('(prefers-color-scheme: dark)').matches)
	)
}

/**
 * El modo al que lleva la D (`THEME` de `lib/shortcuts.ts`): del sistema, al
 * que el sistema no es, para que la página siempre cambie; de uno elegido, de
 * vuelta al del sistema.
 */
export function toggledTheme(now: Theme): Theme {
	return now === 'system' ? (isDark('system') ? 'light' : 'dark') : 'system'
}

/** Pinta el modo: la clase, el `color-scheme` y el color de la barra. */
export function applyTheme(theme: Theme) {
	const dark = isDark(theme)
	const root = document.documentElement
	root.classList.toggle('dark', dark)
	root.style.colorScheme = dark ? 'dark' : 'light'
	document
		.querySelector('meta[name="theme-color"]')
		?.setAttribute('content', dark ? THEME_COLOR.dark : THEME_COLOR.light)
}

/**
 * Guarda la elección —un año, en todo el sitio— y la pinta. `system` borra la
 * cookie: volver a seguir al sistema es no haber elegido.
 */
export function saveTheme(theme: Theme) {
	const base = `${THEME_COOKIE}=${theme}; path=/; samesite=lax${cookieDomain(location.hostname)}${location.protocol === 'https:' ? '; secure' : ''}`
	document.cookie =
		theme === 'system' ? `${base}; max-age=0` : `${base}; max-age=31536000`
	applyTheme(theme)
}

/**
 * Sigue al sistema mientras la elección sea `system`: si cambia de claro a
 * oscuro con la página abierta, la página cambia con él.
 */
export function followSystem(current: () => Theme): () => void {
	const query = window.matchMedia('(prefers-color-scheme: dark)')
	const onChange = () => {
		if (current() === 'system') applyTheme('system')
	}
	query.addEventListener('change', onChange)
	return () => query.removeEventListener('change', onChange)
}
