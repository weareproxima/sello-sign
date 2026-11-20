import * as React from 'react'
import { cn } from '#/lib/utils.ts'

/*
 * Sello UI · @sello/card · https://sello.tax/r/registry.json
 * Copyright (c) 2026 Proxima AI LLC. MIT License: https://sello.tax/r/LICENSE.txt
 * Derived from shadcn/ui (https://ui.shadcn.com), Copyright (c) 2023
 * shadcn, MIT License.
 *
 * Generated from Sello's design system. Don't edit it here: update it with
 * `bunx shadcn@latest add @sello/card --overwrite`.
 */

/**
 * Una tarjeta: una superficie que se apoya en la página (`shadow-raised`), con
 * el radio de tarjeta y 24 px alrededor (16 en `sm`). El relleno es de la
 * tarjeta, no de sus partes: lo de dentro puede ir suelto. Su título es un
 * `h2` en `text-h3`, y su descripción, `text-ui` en el tono secundario. Una
 * tarjeta es un objeto, no cada parte de una página: ver «Superficies» en
 * `DESIGN.md`.
 */
function Card({
	className,
	size = 'default',
	...props
}: React.ComponentProps<'div'> & { size?: 'default' | 'sm' }) {
	return (
		<div
			data-slot="card"
			data-size={size}
			className={cn(
				'group/card flex flex-col gap-6 rounded-card bg-surface p-6 text-foreground shadow-raised data-[size=sm]:gap-4 data-[size=sm]:p-4',
				className
			)}
			{...props}
		/>
	)
}

function CardHeader({ className, ...props }: React.ComponentProps<'div'>) {
	return (
		<div
			data-slot="card-header"
			className={cn(
				'group/card-header @container/card-header grid auto-rows-min items-start gap-1 has-data-[slot=card-action]:grid-cols-[1fr_auto] has-data-[slot=card-description]:grid-rows-[auto_auto]',
				className
			)}
			{...props}
		/>
	)
}

function CardTitle({ className, ...props }: React.ComponentProps<'h2'>) {
	return (
		<h2
			data-slot="card-title"
			className={cn('font-heading text-h3', className)}
			{...props}
		/>
	)
}

function CardDescription({ className, ...props }: React.ComponentProps<'p'>) {
	return (
		<p
			data-slot="card-description"
			className={cn('text-ui text-pretty text-muted-foreground', className)}
			{...props}
		/>
	)
}

function CardAction({ className, ...props }: React.ComponentProps<'div'>) {
	return (
		<div
			data-slot="card-action"
			className={cn(
				'col-start-2 row-span-2 row-start-1 self-start justify-self-end',
				className
			)}
			{...props}
		/>
	)
}

function CardContent({ className, ...props }: React.ComponentProps<'div'>) {
	return <div data-slot="card-content" className={cn(className)} {...props} />
}

function CardFooter({ className, ...props }: React.ComponentProps<'div'>) {
	return (
		<div
			data-slot="card-footer"
			className={cn('flex items-center gap-2', className)}
			{...props}
		/>
	)
}

export {
	Card,
	CardHeader,
	CardFooter,
	CardTitle,
	CardAction,
	CardDescription,
	CardContent,
}
