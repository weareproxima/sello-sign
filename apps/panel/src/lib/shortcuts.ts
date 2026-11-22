/*
 * Sello UI · @sello/shortcuts · https://sello.tax/r/registry.json
 * Copyright (c) 2026 Proxima AI LLC. MIT License: https://sello.tax/r/LICENSE.txt
 * Derived from shadcn/ui (https://ui.shadcn.com), Copyright (c) 2023
 * shadcn, MIT License.
 *
 * Generated from Sello's design system. Don't edit it here: update it with
 * `bunx shadcn@latest add @sello/shortcuts --overwrite`.
 */

/**
 * Las teclas que comparten las apps de Sello —el chat, la web de la
 * plataforma, el IdP y el panel de Sello Sign—, para que quien las tiene en
 * los dedos en una las encuentre en las otras: el modificador con el nombre de su teclado, la tecla que, mantenida,
 * enseña los atajos de la página (`components/shortcut-hints.tsx`), y la D,
 * que cambia el modo de color.
 *
 * Sin DOM ni React al importarlo: lo importan los tests de las apps.
 */

/**
 * La tecla de los atajos (⌘K), con el nombre que tiene en el teclado de quien
 * lee: en un Mac es ⌘, en los demás Ctrl. Las dos sirven en todas partes.
 */
export const MOD =
	typeof navigator !== 'undefined' &&
	/Mac|iPhone|iPad/.test(navigator.userAgent)
		? '⌘'
		: 'Ctrl'

/** Lo que espera `aria-keyshortcuts` para la misma combinación. */
export const MOD_ARIA = MOD === '⌘' ? 'Meta' : 'Control'

/**
 * La tecla que, mantenida sola, enseña los atajos de la página: el modificador
 * con el que empiezan casi todos, ⌘ en un Mac y Ctrl en los demás.
 */
export const HOLD_KEY = MOD_ARIA

/**
 * Un atajo de la página: cómo se lee en el teclado de quien lo usa y cómo lo
 * nombra `aria-keyshortcuts`.
 */
export type Shortcut = { text: string; aria: string }

/**
 * Cambia entre el modo del sistema y el otro (`toggledTheme` de
 * `lib/theme.ts`). D de «dark». Una sola tecla, así que no mientras se escribe.
 */
export const THEME: Shortcut = { text: 'D', aria: 'D' }

/** Lo que una pulsación dice de sí misma: un `KeyboardEvent`, o su igual en un test. */
type Keypress = {
	key: string
	metaKey: boolean
	ctrlKey: boolean
	altKey: boolean
	shiftKey: boolean
	repeat?: boolean
	target?: EventTarget | null
}

export function isTheme(e: Keypress): boolean {
	if (e.metaKey || e.ctrlKey || e.altKey || e.shiftKey || e.repeat) return false
	return e.key.toLowerCase() === 'd' && !isTyping(e.target ?? null)
}

/** ¿El foco está en algo en lo que se escribe? Ahí no vale un atajo de una sola tecla. */
export function isTyping(target: EventTarget | null): boolean {
	if (typeof HTMLElement === 'undefined' || !(target instanceof HTMLElement))
		return false
	return (
		target.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(target.tagName)
	)
}
