import { useEffect, useState, type CSSProperties } from 'react'
import { Kbd } from '#/components/ui/kbd.tsx'
import { HOLD_KEY } from '#/lib/shortcuts.ts'
import { cn } from '#/lib/utils.ts'

/*
 * Sello UI · @sello/shortcut-hints · https://sello.tax/r/registry.json
 * Copyright (c) 2026 Proxima AI LLC. MIT License: https://sello.tax/r/LICENSE.txt
 * Derived from shadcn/ui (https://ui.shadcn.com), Copyright (c) 2023
 * shadcn, MIT License.
 *
 * Generated from Sello's design system. Don't edit it here: update it with
 * `bunx shadcn@latest add @sello/shortcut-hints --overwrite`.
 */

/*
 * Los atajos a la vista mientras se mantiene ⌘ (Ctrl en Windows y Linux), los
 * mismos en el chat, la web de la plataforma, el IdP y el panel de Sello
 * Sign. Ver «Atajos visibles» en `DESIGN.md`.
 */

/**
 * Cuánto hay que mantener ⌘ para que aparezcan. Lo bastante para que un ⌘C o
 * un ⌘K pasen sin que parpadeen; lo bastante poco para que quien los busca no
 * crea que no pasa nada.
 */
const HOLD_MS = 500

/** Los modificadores que no cortan la espera: ⇧ y ⌥ son parte de las combinaciones. */
const MODIFIERS = new Set(['Shift', 'Alt', 'Meta', 'Control'])

/**
 * Dónde va una pista respecto de su control. `end` es dentro, contra su borde
 * derecho, como en las filas de una columna, donde la etiqueta ocupa el ancho.
 */
type Side = 'top' | 'right' | 'bottom' | 'left' | 'end'

type Hint = { keys: Array<string>; style: CSSProperties; side: Side }

/**
 * Las distancias, en pasos de la escala de espaciado y en CSS (`calc` en el
 * estilo), para que lean `--spacing` en vez de repetirlo en píxeles: 6 px
 * entre una pista y su control, y 8 px desde el borde derecho del control para
 * las que van dentro (`end`).
 */
const GAP = 'var(--spacing) * 1.5'
const INSET = 'var(--spacing) * 2'

/**
 * ¿Se ve el control? Se pregunta al punto de su centro: fuera de la pantalla
 * no hay nada, y bajo un diálogo lo que hay es el diálogo.
 */
function inSight(el: Element): boolean {
	const r = el.getBoundingClientRect()
	const hit = document.elementFromPoint(
		r.left + r.width / 2,
		r.top + r.height / 2
	)
	return hit !== null && el.contains(hit)
}

/**
 * Un control enseña su pista si se ve: maquetado, sin un ancestro que lo
 * desvanezca —las etiquetas de una columna plegada, las acciones que solo
 * salen al pasar el cursor—, en pantalla y sin nada encima.
 */
function shown(el: HTMLElement): boolean {
	if (!el.getClientRects().length) return false
	for (let n: HTMLElement | null = el; n; n = n.parentElement) {
		const style = getComputedStyle(n)
		if (style.opacity === '0' || style.visibility === 'hidden') return false
	}
	return inSight(el)
}

/**
 * El lado cuando el control no lo dice: a la derecha de lo que está contra el
 * borde izquierdo (una columna plegada), a la izquierda de lo que está contra
 * el derecho, encima de lo que está abajo y debajo del resto.
 */
function sideFor(r: DOMRect): Side {
	if (r.left < 120) return 'right'
	if (r.right > window.innerWidth - 120) return 'left'
	if (r.bottom > window.innerHeight - 120) return 'top'
	return 'bottom'
}

function place(r: DOMRect, side: Side): CSSProperties {
	const midX = r.left + r.width / 2
	const midY = r.top + r.height / 2
	switch (side) {
		case 'top':
			return {
				left: midX,
				top: `calc(${r.top}px - ${GAP})`,
				translate: '-50% -100%',
			}
		case 'right':
			return {
				left: `calc(${r.right}px + ${GAP})`,
				top: midY,
				translate: '0 -50%',
			}
		case 'left':
			return {
				left: `calc(${r.left}px - ${GAP})`,
				top: midY,
				translate: '-100% -50%',
			}
		case 'end':
			return {
				left: `calc(${r.right}px - ${INSET})`,
				top: midY,
				translate: '-100% -50%',
			}
		default:
			return {
				left: midX,
				top: `calc(${r.bottom}px + ${GAP})`,
				translate: '-50% 0',
			}
	}
}

/**
 * Cada control que declara un atajo (`data-hint`), se ve y respondería a él
 * ahora, con dónde va su pista. No el que ya tiene el foco —la caja en la que
 * se escribe no necesita su tecla para llegar a ella—, ni uno deshabilitado,
 * cuya tecla no hace nada, ni el que la app descarta (`skip`).
 */
function collect(skip?: (el: HTMLElement) => boolean): Array<Hint> {
	const hints: Array<Hint> = []
	for (const el of document.querySelectorAll<HTMLElement>('[data-hint]')) {
		if (!shown(el) || el.contains(document.activeElement)) continue
		if (el.matches(':disabled, [aria-disabled="true"]')) continue
		if (skip?.(el)) continue
		const r = el.getBoundingClientRect()
		const side = (el.dataset.hintSide as Side | undefined) ?? sideFor(r)
		// Varios atajos en un control, uno por línea, se apilan en vez de ir
		// uno al lado del otro.
		const keys = (el.dataset.hint ?? '').split('\n').filter(Boolean)
		hints.push({ keys, style: place(r, side), side })
	}
	return hints
}

/**
 * Los atajos de la página mientras se mantiene ⌘ (Ctrl en Windows y Linux).
 *
 * Mantenerlo es pedirlos: a diferencia de las tres teclas siempre a la vista,
 * aquí cada control con atajo lo enseña. Cada uno lo declara con `data-hint`
 * (su texto, como lo enseña el tooltip) y, si el lado por defecto no le sirve,
 * `data-hint-side`.
 *
 * Las pistas no se pintan dentro de cada control sino en una capa propia,
 * medida al aparecer: dentro, las filas de una columna, que recortan lo que
 * sobresale, las cortarían, y habría tantos temporizadores como controles.
 * Arriba, `summary`: lo que no depende de un control a la vista —en el chat,
 * el atajo de la lista entera; donde no la hay, la D del modo—, salvo si un
 * control a la vista ya lo lleva.
 *
 * Aparecen tras `HOLD_MS` y se van en cuanto se hace algo: soltar la tecla,
 * pulsar cualquier otra —el atajo que se usa—, un clic, la rueda (Ctrl+rueda
 * es el zoom), cambiar el tamaño de la ventana o salir de ella, cuyo `keyup`
 * nunca llega tras un ⌘Tab. Solo para la vista: cada control ya anuncia su
 * atajo con `aria-keyshortcuts`.
 */
export function ShortcutHints({
	summary,
	skip,
}: {
	summary: { keys: string; label: string }
	/** Un control con atajo que ahora no respondería a él, por una razón que solo la app sabe. */
	skip?: (el: HTMLElement) => boolean
}) {
	const [hints, setHints] = useState<Array<Hint> | null>(null)

	useEffect(() => {
		let timer: number | undefined
		const hide = () => {
			window.clearTimeout(timer)
			timer = undefined
			setHints(null)
		}
		const onKeyDown = (e: KeyboardEvent) => {
			if (e.key === HOLD_KEY) {
				if (e.repeat || timer !== undefined) return
				timer = window.setTimeout(() => setHints(collect(skip)), HOLD_MS)
			} else if (!MODIFIERS.has(e.key)) {
				hide()
			}
		}
		const onKeyUp = (e: KeyboardEvent) => {
			if (e.key === HOLD_KEY) hide()
		}
		window.addEventListener('keydown', onKeyDown)
		window.addEventListener('keyup', onKeyUp)
		window.addEventListener('pointerdown', hide)
		window.addEventListener('wheel', hide, { passive: true })
		window.addEventListener('resize', hide)
		window.addEventListener('blur', hide)
		document.addEventListener('visibilitychange', hide)
		return () => {
			hide()
			window.removeEventListener('keydown', onKeyDown)
			window.removeEventListener('keyup', onKeyUp)
			window.removeEventListener('pointerdown', hide)
			window.removeEventListener('wheel', hide)
			window.removeEventListener('resize', hide)
			window.removeEventListener('blur', hide)
			document.removeEventListener('visibilitychange', hide)
		}
	}, [skip])

	if (!hints) return null
	// Con un control a la vista que ya lo lleva, su pista ya lo dice.
	const summaryInView = hints.some((h) => h.keys.includes(summary.keys))
	const appear = 'animate-in fade-in-0 duration-fast ease-out'

	return (
		<div
			aria-hidden="true"
			className="pointer-events-none fixed inset-0 z-[60] print:hidden"
		>
			{!summaryInView && (
				<div
					className={cn(
						appear,
						'absolute top-4 left-1/2 flex -translate-x-1/2 items-center gap-2 rounded-full bg-primary py-1 pr-3 pl-1 text-meta text-primary-foreground shadow-float'
					)}
				>
					<Kbd className="bg-primary-hover text-primary-foreground">
						{summary.keys}
					</Kbd>
					{summary.label}
				</div>
			)}
			{hints.map((hint, i) => (
				<div
					key={i}
					style={hint.style}
					// Apiladas contra el control: a su izquierda se alinean a la
					// derecha, y al revés.
					className={cn(
						appear,
						'absolute flex flex-col gap-1',
						hint.side === 'left' || hint.side === 'end'
							? 'items-end'
							: hint.side === 'right'
								? 'items-start'
								: 'items-center'
					)}
				>
					{hint.keys.map((k) => (
						<Kbd
							key={k}
							className="bg-primary whitespace-nowrap text-primary-foreground shadow-float"
						>
							{k}
						</Kbd>
					))}
				</div>
			))}
		</div>
	)
}
