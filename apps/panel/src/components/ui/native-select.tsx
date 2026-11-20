import * as React from 'react'
import { cn } from '#/lib/utils.ts'
import { ChevronDownIcon } from 'lucide-react'

/*
 * Sello UI · @sello/native-select · https://sello.tax/r/registry.json
 * Copyright (c) 2026 Proxima AI LLC. MIT License: https://sello.tax/r/LICENSE.txt
 * Derived from shadcn/ui (https://ui.shadcn.com), Copyright (c) 2023
 * shadcn, MIT License.
 *
 * Generated from Sello's design system. Don't edit it here: update it with
 * `bunx shadcn@latest add @sello/native-select --overwrite`.
 */

/**
 * El `<select>` del sistema operativo con el aspecto de un campo (`Input`):
 * un relleno con radio de panel, a 40 px y en `text-field`, para que iOS no
 * amplíe la página al tocarlo. La lista la dibuja el sistema, que en un
 * teléfono es su propia rueda o su hoja: por eso sirve donde un control
 * segmentado no cabe. Sin borde; el foco es el de la base. La lista sigue el
 * modo de la página por `color-scheme`, que pone `THEME_SCRIPT`.
 */
function NativeSelect({ className, ...props }: React.ComponentProps<'select'>) {
	return (
		<div
			className={cn('group/native-select relative w-fit', className)}
			data-slot="native-select-wrapper"
		>
			<select
				data-slot="native-select"
				className="h-control-md w-full min-w-0 appearance-none rounded-panel bg-fill pr-10 pl-3 text-field text-foreground transition-colors hover:bg-fill-hover disabled:cursor-not-allowed disabled:text-faint aria-invalid:bg-bad-fill"
				{...props}
			/>
			<ChevronDownIcon
				className="pointer-events-none absolute top-1/2 right-3 size-4 -translate-y-1/2 text-muted-foreground group-has-[select:disabled]/native-select:text-faint"
				aria-hidden="true"
				data-slot="native-select-icon"
			/>
		</div>
	)
}

function NativeSelectOption(props: React.ComponentProps<'option'>) {
	return <option data-slot="native-select-option" {...props} />
}

function NativeSelectOptGroup(props: React.ComponentProps<'optgroup'>) {
	return <optgroup data-slot="native-select-optgroup" {...props} />
}

export { NativeSelect, NativeSelectOptGroup, NativeSelectOption }
