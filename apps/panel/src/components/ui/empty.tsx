import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '#/lib/utils.ts'

/*
 * Sello UI · @sello/empty · https://sello.tax/r/registry.json
 * Copyright (c) 2026 Proxima AI LLC. MIT License: https://sello.tax/r/LICENSE.txt
 * Derived from shadcn/ui (https://ui.shadcn.com), Copyright (c) 2023
 * shadcn, MIT License.
 *
 * Generated from Sello's design system. Don't edit it here: update it with
 * `bunx shadcn@latest add @sello/empty --overwrite`.
 */

/**
 * Lo que dice una lista cuando no tiene nada: un panel de tono (`bg-fill`),
 * centrado, sin borde discontinuo. El icono, en un disco en relieve
 * (`bg-raised`), porque se apoya sobre el relleno.
 */
function Empty({ className, ...props }: React.ComponentProps<'div'>) {
	return (
		<div
			data-slot="empty"
			className={cn(
				'flex w-full min-w-0 flex-1 flex-col items-center justify-center gap-4 rounded-panel bg-fill px-6 py-10 text-center text-balance',
				className
			)}
			{...props}
		/>
	)
}

function EmptyHeader({ className, ...props }: React.ComponentProps<'div'>) {
	return (
		<div
			data-slot="empty-header"
			className={cn('flex max-w-sm flex-col items-center gap-1', className)}
			{...props}
		/>
	)
}

const emptyMediaVariants = cva(
	'mb-2 flex shrink-0 items-center justify-center [&_svg]:pointer-events-none [&_svg]:shrink-0',
	{
		variants: {
			variant: {
				default: 'bg-transparent',
				icon: "size-10 rounded-full bg-raised text-muted-foreground shadow-raised [&_svg:not([class*='size-'])]:size-5",
			},
		},
		defaultVariants: {
			variant: 'default',
		},
	}
)

function EmptyMedia({
	className,
	variant = 'default',
	...props
}: React.ComponentProps<'div'> & VariantProps<typeof emptyMediaVariants>) {
	return (
		<div
			data-slot="empty-icon"
			data-variant={variant}
			className={cn(emptyMediaVariants({ variant, className }))}
			{...props}
		/>
	)
}

function EmptyTitle({ className, ...props }: React.ComponentProps<'div'>) {
	return (
		<div
			data-slot="empty-title"
			className={cn('text-ui font-medium', className)}
			{...props}
		/>
	)
}

function EmptyDescription({ className, ...props }: React.ComponentProps<'p'>) {
	return (
		<p
			data-slot="empty-description"
			className={cn(
				'text-ui text-pretty text-muted-foreground [&>a]:underline [&>a]:underline-offset-4 [&>a:hover]:text-foreground',
				className
			)}
			{...props}
		/>
	)
}

function EmptyContent({ className, ...props }: React.ComponentProps<'div'>) {
	return (
		<div
			data-slot="empty-content"
			className={cn(
				'flex w-full max-w-sm min-w-0 flex-col items-center gap-4 text-ui text-balance',
				className
			)}
			{...props}
		/>
	)
}

export {
	Empty,
	EmptyHeader,
	EmptyTitle,
	EmptyDescription,
	EmptyContent,
	EmptyMedia,
}
