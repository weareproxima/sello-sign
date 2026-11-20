import { cva } from 'class-variance-authority'

/*
 * Sello UI · @sello/button-variants · https://sello.tax/r/registry.json
 * Copyright (c) 2026 Proxima AI LLC. MIT License: https://sello.tax/r/LICENSE.txt
 * Derived from shadcn/ui (https://ui.shadcn.com), Copyright (c) 2023
 * shadcn, MIT License.
 *
 * Generated from Sello's design system. Don't edit it here: update it with
 * `bunx shadcn@latest add @sello/button-variants --overwrite`.
 */

/**
 * El botón de Capas: una píldora. Tres alturas (`sm` 32, por defecto 40, `lg`
 * 48) y sus versiones cuadradas para un icono. `xs` (24) solo va dentro de
 * otra pieza, como las flechas de las ediciones de una pregunta.
 *
 * El foco no se dibuja aquí: es el de la base (`:focus-visible` en
 * `globals.css`), igual para todo. Deshabilitado es texto terciario, sin
 * transparencias. Ver «Botones» en `DESIGN.md`.
 *
 * Vive aquí, sin React, para que el landing (Astro) pinte sus enlaces con las
 * mismas clases que el `Button` del chat: `class={buttonVariants({ size: 'lg' })}`.
 */
export const buttonVariants = cva(
	"group/button inline-flex shrink-0 items-center justify-center rounded-full border border-transparent bg-clip-padding text-ui font-medium whitespace-nowrap transition-colors select-none active:not-aria-[haspopup]:translate-y-px disabled:pointer-events-none aria-invalid:border-bad [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
	{
		variants: {
			variant: {
				/** La acción principal: tinta. */
				default:
					'bg-primary text-primary-foreground hover:bg-primary-hover disabled:bg-fill disabled:text-faint',
				/** Sobre un relleno o flotando: el relieve, que se apoya encima. */
				outline:
					'bg-raised text-foreground shadow-raised hover:bg-raised-hover disabled:text-faint disabled:shadow-none aria-expanded:bg-raised-hover',
				/** La acción de al lado: relleno. */
				secondary:
					'bg-fill text-foreground hover:bg-fill-hover disabled:text-faint aria-expanded:bg-fill-hover',
				/** Sin fondo hasta que se pasa por encima. */
				ghost:
					'text-foreground hover:bg-fill disabled:text-faint aria-expanded:bg-fill',
				/** Lo que borra o no se puede deshacer. */
				destructive:
					'bg-bad-fill text-bad hover:bg-bad-fill-hover disabled:bg-fill disabled:text-faint',
				link: 'text-foreground decoration-line-strong underline-offset-4 hover:underline',
			},
			size: {
				default:
					'h-control-md gap-2 px-4 has-data-[icon=inline-end]:pr-3 has-data-[icon=inline-start]:pl-3',
				xs: "h-6 gap-1 px-2 text-meta has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*='size-'])]:size-3",
				sm: 'h-control-sm gap-1.5 px-3 has-data-[icon=inline-end]:pr-2 has-data-[icon=inline-start]:pl-2',
				lg: 'h-control-lg gap-2 px-5 text-field has-data-[icon=inline-end]:pr-4 has-data-[icon=inline-start]:pl-4',
				icon: 'size-control-md',
				'icon-xs': "size-6 [&_svg:not([class*='size-'])]:size-3",
				'icon-sm': 'size-control-sm',
				'icon-lg': 'size-control-lg',
			},
		},
		defaultVariants: {
			variant: 'default',
			size: 'default',
		},
	}
)
