import { Separator as SeparatorPrimitive } from '@base-ui/react/separator'
import { cn } from '#/lib/utils.ts'

/*
 * Sello UI · @sello/separator · https://sello.tax/r/registry.json
 * Copyright (c) 2026 Proxima AI LLC. MIT License: https://sello.tax/r/LICENSE.txt
 * Derived from shadcn/ui (https://ui.shadcn.com), Copyright (c) 2023
 * shadcn, MIT License.
 *
 * Generated from Sello's design system. Don't edit it here: update it with
 * `bunx shadcn@latest add @sello/separator --overwrite`.
 */

function Separator({
	className,
	orientation = 'horizontal',
	...props
}: SeparatorPrimitive.Props) {
	return (
		<SeparatorPrimitive
			data-slot="separator"
			orientation={orientation}
			className={cn(
				'shrink-0 bg-line data-horizontal:h-px data-horizontal:w-full data-vertical:w-px data-vertical:self-stretch',
				className
			)}
			{...props}
		/>
	)
}

export { Separator }
