import { cn } from '#/lib/utils.ts'

/*
 * Sello UI · @sello/kbd · https://sello.tax/r/registry.json
 * Copyright (c) 2026 Proxima AI LLC. MIT License: https://sello.tax/r/LICENSE.txt
 * Derived from shadcn/ui (https://ui.shadcn.com), Copyright (c) 2023
 * shadcn, MIT License.
 *
 * Generated from Sello's design system. Don't edit it here: update it with
 * `bunx shadcn@latest add @sello/kbd --overwrite`.
 */

/**
 * Una tecla. Solo tres controles la enseñan a la vista —Nueva consulta, Buscar
 * y la caja en reposo—; el resto de atajos va en el tooltip de su control o en
 * el diálogo de atajos (ver «Atajos visibles» en `DESIGN.md`).
 *
 * En sans y no en mono, aunque `text-label` sea de datos en mono: la Geist Mono
 * que se carga (`@fontsource-variable/geist-mono`) no trae ⌘ ⇧ ⌥ ↵ ← →, solo
 * ↑ ↓. En mono salían de Menlo o Consolas, más pequeños y bajos que las letras,
 * y ↑ ↓ no casaban con ← →; en sans los pinta la fuente del sistema, que los
 * dibuja para esto.
 */
function Kbd({ className, ...props }: React.ComponentProps<'kbd'>) {
	return (
		<kbd
			data-slot="kbd"
			className={cn(
				"pointer-events-none inline-flex h-5 w-fit min-w-5 items-center justify-center gap-0.5 rounded-small bg-fill-hover px-1.5 font-sans text-label font-medium text-muted-foreground select-none in-data-[slot=tooltip-content]:bg-primary-hover in-data-[slot=tooltip-content]:text-primary-foreground [&_svg:not([class*='size-'])]:size-3",
				className
			)}
			{...props}
		/>
	)
}

function KbdGroup({ className, ...props }: React.ComponentProps<'div'>) {
	return (
		<kbd
			data-slot="kbd-group"
			className={cn('inline-flex items-center gap-1', className)}
			{...props}
		/>
	)
}

export { Kbd, KbdGroup }
