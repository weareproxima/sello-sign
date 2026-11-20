import { Checkbox as CheckboxPrimitive } from '@base-ui/react/checkbox'
import { cn } from '#/lib/utils.ts'
import { CheckIcon } from 'lucide-react'

/*
 * Sello UI · @sello/checkbox · https://sello.tax/r/registry.json
 * Copyright (c) 2026 Proxima AI LLC. MIT License: https://sello.tax/r/LICENSE.txt
 * Derived from shadcn/ui (https://ui.shadcn.com), Copyright (c) 2023
 * shadcn, MIT License.
 *
 * Generated from Sello's design system. Don't edit it here: update it with
 * `bunx shadcn@latest add @sello/checkbox --overwrite`.
 */

function Checkbox({ className, ...props }: CheckboxPrimitive.Root.Props) {
	return (
		<CheckboxPrimitive.Root
			data-slot="checkbox"
			className={cn(
				// Como el radio: el borde es `faint`, el gris más claro que pasa 3:1
				// contra el fondo, porque un control tiene que verse sin tocarlo.
				'peer relative flex size-4 shrink-0 items-center justify-center rounded-small border border-faint bg-surface transition-colors duration-fast outline-none after:absolute after:-inset-x-3 after:-inset-y-2 disabled:cursor-not-allowed disabled:border-line-strong aria-invalid:border-bad data-checked:border-primary data-checked:bg-primary data-checked:text-primary-foreground',
				className
			)}
			{...props}
		>
			<CheckboxPrimitive.Indicator
				data-slot="checkbox-indicator"
				className="grid place-content-center text-current transition-none [&>svg]:size-3.5"
			>
				<CheckIcon />
			</CheckboxPrimitive.Indicator>
		</CheckboxPrimitive.Root>
	)
}

export { Checkbox }
