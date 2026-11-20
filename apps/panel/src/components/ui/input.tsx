import * as React from 'react'
import { Input as InputPrimitive } from '@base-ui/react/input'
import { cn } from '#/lib/utils.ts'

/*
 * Sello UI · @sello/input · https://sello.tax/r/registry.json
 * Copyright (c) 2026 Proxima AI LLC. MIT License: https://sello.tax/r/LICENSE.txt
 * Derived from shadcn/ui (https://ui.shadcn.com), Copyright (c) 2023
 * shadcn, MIT License.
 *
 * Generated from Sello's design system. Don't edit it here: update it with
 * `bunx shadcn@latest add @sello/input --overwrite`.
 */

function Input({ className, type, ...props }: React.ComponentProps<'input'>) {
	return (
		<InputPrimitive
			type={type}
			data-slot="input"
			className={cn(
				// Un campo es un relleno con radio de panel, a la altura del botón por
				// defecto: una píldora es una acción, y un campo no lo es. Siempre a
				// 16 px (`text-field`): por debajo, iOS amplía la página al
				// enfocarlo. Sin borde; el foco es el de la base. Como el de Sello
				// Sign (ver «Campos» en `DESIGN.md`).
				'h-control-md w-full min-w-0 rounded-panel bg-fill px-3 text-field text-foreground transition-colors placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:text-faint aria-invalid:bg-bad-fill',
				className
			)}
			{...props}
		/>
	)
}

export { Input }
