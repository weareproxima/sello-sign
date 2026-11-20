import { mergeProps } from '@base-ui/react/merge-props'
import { useRender } from '@base-ui/react/use-render'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '#/lib/utils.ts'

/*
 * Sello UI · @sello/badge · https://sello.tax/r/registry.json
 * Copyright (c) 2026 Proxima AI LLC. MIT License: https://sello.tax/r/LICENSE.txt
 * Derived from shadcn/ui (https://ui.shadcn.com), Copyright (c) 2023
 * shadcn, MIT License.
 *
 * Generated from Sello's design system. Don't edit it here: update it with
 * `bunx shadcn@latest add @sello/badge --overwrite`.
 */

const badgeVariants = cva(
	'group/badge inline-flex h-5 w-fit shrink-0 items-center justify-center gap-1 overflow-hidden rounded-full border border-transparent px-2 text-meta font-medium whitespace-nowrap transition-colors has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 aria-invalid:border-bad [&>svg]:pointer-events-none [&>svg]:size-3!',
	{
		variants: {
			variant: {
				default:
					'bg-primary text-primary-foreground [a]:hover:bg-primary-hover',
				secondary: 'bg-fill text-foreground [a]:hover:bg-fill-hover',
				destructive: 'bg-bad-fill text-bad [a]:hover:bg-bad-fill-hover',
				/** Veredictos: lo que se sostiene y lo que advierte. Nunca decoran. */
				ok: 'bg-ok-fill text-ok [a]:hover:bg-ok-fill-hover',
				warn: 'bg-warn-fill text-warn [a]:hover:bg-warn-fill-hover',
				outline:
					'border-line-strong text-foreground [a]:hover:bg-fill [a]:hover:text-muted-foreground',
				ghost: 'hover:bg-fill hover:text-muted-foreground',
				link: 'text-foreground underline-offset-4 hover:underline',
			},
		},
		defaultVariants: {
			variant: 'default',
		},
	}
)

function Badge({
	className,
	variant = 'default',
	render,
	...props
}: useRender.ComponentProps<'span'> & VariantProps<typeof badgeVariants>) {
	return useRender({
		defaultTagName: 'span',
		props: mergeProps<'span'>(
			{
				className: cn(badgeVariants({ variant }), className),
			},
			props
		),
		render,
		state: {
			slot: 'badge',
			variant,
		},
	})
}

export { Badge, badgeVariants }
