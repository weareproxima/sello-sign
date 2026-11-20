import * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'

import { cn } from '#/lib/utils.ts'

/*
 * Sello UI · @sello/alert · https://sello.tax/r/registry.json
 * Copyright (c) 2026 Proxima AI LLC. MIT License: https://sello.tax/r/LICENSE.txt
 * Derived from shadcn/ui (https://ui.shadcn.com), Copyright (c) 2023
 * shadcn, MIT License.
 *
 * Generated from Sello's design system. Don't edit it here: update it with
 * `bunx shadcn@latest add @sello/alert --overwrite`.
 */

/**
 * Una alerta en la página: un panel de tono (`rounded-panel`), el relleno o el
 * de un veredicto, con su icono a la izquierda. Lo que salió bien no es un
 * veredicto: va en `default`. Ver «Recetas › Alerta» en `DESIGN.md`.
 */
const alertVariants = cva(
	"group/alert relative grid w-full gap-0.5 rounded-panel px-4 py-3 text-left text-ui has-data-[slot=alert-action]:pr-18 has-[>svg]:grid-cols-[auto_1fr] has-[>svg]:gap-x-3 *:[svg]:row-span-2 *:[svg]:translate-y-0.5 *:[svg:not([class*='size-'])]:size-4",
	{
		variants: {
			variant: {
				default:
					'bg-fill text-foreground *:data-[slot=alert-description]:text-muted-foreground *:[svg]:text-muted-foreground',
				warn: 'bg-warn-fill text-warn',
				destructive: 'bg-bad-fill text-bad',
			},
		},
		defaultVariants: {
			variant: 'default',
		},
	}
)

function Alert({
	className,
	variant,
	...props
}: React.ComponentProps<'div'> & VariantProps<typeof alertVariants>) {
	return (
		<div
			data-slot="alert"
			// Un error se anuncia en cuanto aparece; lo demás, sin interrumpir.
			role={variant === 'destructive' ? 'alert' : 'status'}
			className={cn(alertVariants({ variant }), className)}
			{...props}
		/>
	)
}

function AlertTitle({ className, ...props }: React.ComponentProps<'div'>) {
	return (
		<div
			data-slot="alert-title"
			className={cn(
				'font-medium group-has-[>svg]/alert:col-start-2 [&_a]:underline [&_a]:underline-offset-3',
				className
			)}
			{...props}
		/>
	)
}

function AlertDescription({
	className,
	...props
}: React.ComponentProps<'div'>) {
	return (
		<div
			data-slot="alert-description"
			className={cn(
				'text-pretty group-has-[>svg]/alert:col-start-2 [&_a]:underline [&_a]:underline-offset-3 [&_a]:hover:text-foreground [&_p:not(:last-child)]:mb-4',
				className
			)}
			{...props}
		/>
	)
}

function AlertAction({ className, ...props }: React.ComponentProps<'div'>) {
	return (
		<div
			data-slot="alert-action"
			className={cn('absolute top-2 right-2', className)}
			{...props}
		/>
	)
}

export { Alert, AlertTitle, AlertDescription, AlertAction }
