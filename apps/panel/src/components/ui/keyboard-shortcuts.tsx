import { useEffect, useRef, useState } from 'react'
import { XIcon } from 'lucide-react'
import { Button } from '#/components/ui/button.tsx'
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogTitle,
} from '#/components/ui/dialog.tsx'
import { Kbd } from '#/components/ui/kbd.tsx'
import { isTyping } from '#/lib/shortcuts.ts'

/*
 * Sello UI · @sello/keyboard-shortcuts · https://sello.tax/r/registry.json
 * Copyright (c) 2026 Proxima AI LLC. MIT License: https://sello.tax/r/LICENSE.txt
 * Derived from shadcn/ui (https://ui.shadcn.com), Copyright (c) 2023
 * shadcn, MIT License.
 *
 * Generated from Sello's design system. Don't edit it here: update it with
 * `bunx shadcn@latest add @sello/keyboard-shortcuts --overwrite`.
 */

/*
 * Los atajos de una sola tecla que comparten las apps de Sello —sus
 * secuencias, como G D para ir a una página, y la lista de todos— (ver
 * «Atajos visibles» en `DESIGN.md`).
 */

/** Una fila de la lista: lo que hace y sus teclas, cada una una alternativa. */
export type ShortcutRow = { label: string; keys: ReadonlyArray<string> }

/** Un grupo de la lista, por dónde funciona. */
export type ShortcutGroup = { title: string; rows: ReadonlyArray<ShortcutRow> }

/**
 * Los atajos, en un diálogo. Se abre con `HELP` (⌘/ o `?`), que escucha cada
 * app. Cada grupo es una lista de descripciones: un lector de pantalla la lee
 * «Nueva conversación, ⇧ ⌘ O», que es lo que hace falta, en vez de una tabla
 * con sus encabezados leídos en cada celda.
 */
export function ShortcutsDialog({
	open,
	onOpenChange,
	groups,
	description,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	groups: ReadonlyArray<ShortcutGroup>
	description: string
}) {
	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent
				showCloseButton={false}
				// Centrado, se aparta de las barras del sistema en un teléfono con
				// la app instalada: sus márgenes toman los insets además de los 16 px.
				className="max-h-[calc(100dvh-(--spacing(8))-env(safe-area-inset-top)-env(safe-area-inset-bottom))] gap-5 overflow-y-auto sm:max-w-2xl"
			>
				<div className="flex items-start justify-between gap-4">
					<div className="flex flex-col gap-1.5">
						<DialogTitle>Atajos de teclado</DialogTitle>
						<DialogDescription>{description}</DialogDescription>
					</div>
					<DialogClose
						render={
							<Button
								variant="ghost"
								size="icon-sm"
								className="-mt-1 -mr-1 shrink-0 text-muted-foreground"
							/>
						}
					>
						<XIcon />
						<span className="sr-only">Cerrar</span>
					</DialogClose>
				</div>

				<div className="grid gap-x-8 gap-y-5 sm:grid-cols-2">
					{groups.map((group) => (
						<section key={group.title} className="flex flex-col gap-1">
							<h3 className="px-3 text-meta font-medium text-muted-foreground">
								{group.title}
							</h3>
							{/* Cada grupo es un bloque tonal, sin líneas entre filas:
							    Capas separa con el tono. Las teclas suben dos pasos de
							    relleno (`fill-active`) para destacar sobre el del bloque:
							    una superficie se hundiría en él de noche. */}
							<dl className="flex flex-col rounded-panel bg-fill px-3 py-1">
								{group.rows.map((row) => (
									<div
										key={row.label}
										className="flex min-h-row items-center justify-between gap-4 py-1"
									>
										<dt className="text-ui text-foreground">{row.label}</dt>
										<dd className="flex shrink-0 items-center gap-1">
											{row.keys.map((k) => (
												<Kbd key={k} className="bg-fill-active">
													{k}
												</Kbd>
											))}
										</dd>
									</div>
								))}
							</dl>
						</section>
					))}
				</div>
			</DialogContent>
		</Dialog>
	)
}

/**
 * Un atajo de una o dos letras, sin modificadores: `['n']`, o `['g', 'd']`
 * —G y, en menos de `SEQUENCE_MS`, D—. Las teclas, en minúsculas.
 */
export type KeySequence = { keys: ReadonlyArray<string>; run: () => void }

/** Cuánto espera la segunda tecla de una secuencia antes de olvidar la primera. */
const SEQUENCE_MS = 1500

/** Cuánto se ven las teclas de un atajo que ya hizo lo suyo. */
const SHOWN_MS = 1000

/**
 * Donde una letra es de otro: un campo en el que se escribe, un menú (sus
 * filas se eligen con su letra, `DropdownMenuItem`), una lista de opciones o
 * un diálogo, que tiene encima lo suyo.
 */
function taken(target: EventTarget | null): boolean {
	if (isTyping(target)) return true
	return (
		target instanceof Element &&
		target.closest(
			'[role="menu"], [role="listbox"], [role="dialog"], [role="alertdialog"]'
		) !== null
	)
}

/**
 * Las secuencias de la página, desde cualquier sitio de ella salvo donde la
 * letra es de otro (`taken`), y las teclas pulsadas en una píldora en la
 * esquina, que dice lo que hizo el teclado y no enseña nada.
 *
 * Escucha en la fase de captura, antes que el resto de la página: la D de
 * G D no cambia además el modo de color (`THEME`), que mira si ya se usó la
 * tecla (`defaultPrevented`). Una tecla que no sigue ninguna secuencia
 * empezada la olvida y sigue su camino; Esc también la olvida.
 */
export function KeySequences({
	sequences,
}: {
	sequences: ReadonlyArray<KeySequence>
}) {
	const [pressed, setPressed] = useState<ReadonlyArray<string>>([])
	const latest = useRef(sequences)
	useEffect(() => {
		latest.current = sequences
	})

	useEffect(() => {
		let first: string | null = null
		let waiting: number | undefined
		let hiding: number | undefined
		const forget = () => {
			first = null
			window.clearTimeout(waiting)
			window.clearTimeout(hiding)
			setPressed([])
		}
		const done = (keys: ReadonlyArray<string>) => {
			first = null
			window.clearTimeout(waiting)
			window.clearTimeout(hiding)
			setPressed(keys)
			hiding = window.setTimeout(() => setPressed([]), SHOWN_MS)
		}
		const onKeyDown = (e: KeyboardEvent) => {
			if (
				e.defaultPrevented ||
				e.metaKey ||
				e.ctrlKey ||
				e.altKey ||
				e.shiftKey ||
				e.repeat ||
				taken(e.target)
			) {
				if (first !== null) forget()
				return
			}
			const key = e.key.toLowerCase()
			const sequences = latest.current
			if (first !== null) {
				const prefix = first
				const found = sequences.find(
					(s) =>
						s.keys.length === 2 && s.keys[0] === prefix && s.keys[1] === key
				)
				if (!found) {
					if (key === 'escape') e.preventDefault()
					forget()
					return
				}
				e.preventDefault()
				done([prefix, key])
				found.run()
				return
			}
			const single = sequences.find(
				(s) => s.keys.length === 1 && s.keys[0] === key
			)
			if (single) {
				e.preventDefault()
				done([key])
				single.run()
				return
			}
			if (sequences.some((s) => s.keys.length === 2 && s.keys[0] === key)) {
				e.preventDefault()
				first = key
				window.clearTimeout(hiding)
				setPressed([key])
				window.clearTimeout(waiting)
				waiting = window.setTimeout(forget, SEQUENCE_MS)
			}
		}
		window.addEventListener('keydown', onKeyDown, true)
		window.addEventListener('blur', forget)
		return () => {
			window.removeEventListener('keydown', onKeyDown, true)
			window.removeEventListener('blur', forget)
			window.clearTimeout(waiting)
			window.clearTimeout(hiding)
		}
	}, [])

	if (pressed.length === 0) return null
	return (
		// Un aviso de Capas —tinta, píldora, la sombra que flota—, con cada
		// tecla un paso por encima de la tinta, como una tecla dentro de un
		// tooltip.
		<div
			aria-hidden="true"
			className="pointer-events-none fixed right-6 bottom-6 z-50 flex animate-in items-center gap-1 rounded-full bg-primary p-1 text-primary-foreground shadow-float duration-fast ease-out fade-in-0 print:hidden"
		>
			{pressed.map((key, i) => (
				<Kbd
					key={`${i}-${key}`}
					className="h-control-sm min-w-control-sm rounded-full bg-primary-hover px-2 text-ui text-primary-foreground"
				>
					{key.toUpperCase()}
				</Kbd>
			))}
		</div>
	)
}
