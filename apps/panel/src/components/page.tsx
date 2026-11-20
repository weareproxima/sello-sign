import * as React from 'react'
import { Link, type LinkProps } from '@tanstack/react-router'
import { ArrowLeftIcon } from 'lucide-react'
import { buttonVariants } from '#/lib/button-variants'
import { cn } from '#/lib/utils'

/** A page's column (`page-container` of ../sign-old). */
export function Page({
	size = 'default',
	className,
	children,
}: {
	size?: 'narrow' | 'default' | 'wide'
	className?: string
	children: React.ReactNode
}) {
	return (
		<div
			className={cn(
				'w-full space-y-6',
				size === 'narrow' && 'max-w-3xl',
				size === 'default' && 'max-w-5xl',
				size === 'wide' && 'max-w-7xl',
				className
			)}
		>
			{children}
		</div>
	)
}

/** A page's title, with what it says below and its actions to the side. */
export function PageHeader({
	title,
	description,
	meta,
	actions,
	back,
}: {
	title: string
	description?: string
	meta?: React.ReactNode
	actions?: React.ReactNode
	back?: { label: string } & Pick<LinkProps, 'to' | 'params'>
}) {
	return (
		<div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
			<div className="flex min-w-0 items-start gap-3">
				{back ? (
					<Link
						to={back.to}
						params={back.params}
						aria-label={back.label}
						className={cn(
							buttonVariants({ variant: 'ghost', size: 'icon-sm' }),
							'-ms-2 mt-0.5'
						)}
					>
						<ArrowLeftIcon />
					</Link>
				) : null}
				<div className="min-w-0">
					<h1 className="text-h1 break-words sm:truncate">{title}</h1>
					{meta ? (
						<div className="mt-1 flex flex-wrap items-center gap-2">{meta}</div>
					) : description ? (
						<p className="mt-1 text-ui text-pretty text-muted-foreground">
							{description}
						</p>
					) : null}
				</div>
			</div>
			{actions ? <div className="flex shrink-0 gap-2">{actions}</div> : null}
		</div>
	)
}

/** A form's field: its label, the control, a help line and an error. */
export function FormField({
	label,
	htmlFor,
	help,
	error,
	aside,
	children,
}: {
	label: string
	htmlFor?: string
	help?: React.ReactNode
	error?: string | null
	aside?: React.ReactNode
	children: React.ReactNode
}) {
	return (
		<div className="flex flex-col gap-2">
			<div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
				<label htmlFor={htmlFor} className="text-ui font-medium">
					{label}
				</label>
				{aside}
			</div>
			{children}
			{help ? <p className="text-meta text-muted-foreground">{help}</p> : null}
			{error ? <p className="text-meta text-bad">{error}</p> : null}
		</div>
	)
}

/** A problem as a line on the page. */
export function FormError({ message }: { message: string | null }) {
	if (!message) return null
	return (
		<div
			role="alert"
			className="rounded-panel bg-bad-fill px-4 py-3 text-ui text-bad"
		>
			{message}
		</div>
	)
}

/** Data in mono: ids, tax ids, key prefixes. */
export function Mono({
	children,
	className,
}: {
	children: React.ReactNode
	className?: string
}) {
	return (
		<span className={cn('font-mono text-label', className)}>{children}</span>
	)
}

export function CountryCode({ code }: { code: string }) {
	return <span className="country-code">{code}</span>
}
