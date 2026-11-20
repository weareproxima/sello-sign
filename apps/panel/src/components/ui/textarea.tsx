import * as React from 'react'
import { cn } from '#/lib/utils.ts'

/*
 * Sello UI · @sello/textarea · https://sello.tax/r/registry.json
 * Copyright (c) 2026 Proxima AI LLC. MIT License: https://sello.tax/r/LICENSE.txt
 * Derived from shadcn/ui (https://ui.shadcn.com), Copyright (c) 2023
 * shadcn, MIT License.
 *
 * Generated from Sello's design system. Don't edit it here: update it with
 * `bunx shadcn@latest add @sello/textarea --overwrite`.
 */

function Textarea({ className, ...props }: React.ComponentProps<'textarea'>) {
	return (
		<textarea
			data-slot="textarea"
			className={cn(
				// Un campo suelto es un relleno de panel; el foco es el de la base.
				// Siempre a 16 px: por debajo, iOS amplía la página al enfocarlo.
				'flex field-sizing-content min-h-16 w-full resize-none rounded-panel bg-fill px-3 py-3 text-field transition-colors placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:text-faint aria-invalid:bg-bad-fill',
				className
			)}
			{...props}
		/>
	)
}

export { Textarea }
