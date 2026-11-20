import { createCn } from 'cn/config'

/*
 * Sello UI · @sello/utils · https://sello.tax/r/registry.json
 * Copyright (c) 2026 Proxima AI LLC. MIT License: https://sello.tax/r/LICENSE.txt
 * Derived from shadcn/ui (https://ui.shadcn.com), Copyright (c) 2023
 * shadcn, MIT License.
 *
 * Generated from Sello's design system. Don't edit it here: update it with
 * `bunx shadcn@latest add @sello/utils --overwrite`.
 */

/**
 * `cn` (clases condicionales + resolución de conflictos) con la escala del
 * sistema de diseño.
 *
 * Sin la escala, `cn` no sabe que `text-ui` es un tamaño: lo toma por un color
 * y, al juntarlo con `text-muted-foreground`, lo descarta como si fueran dos
 * colores. Lo mismo con los radios, las sombras y las alturas con nombre. Si
 * se añade un papel o un token a `styles/tokens.css`, se añade aquí también.
 */
export const cn = createCn({
	extend: {
		theme: {
			text: [
				'label',
				'meta',
				'ui',
				'field',
				'body',
				'lede',
				'h3',
				'h2',
				'h1',
				'd2',
				'd1',
			],
			radius: ['small', 'row', 'panel', 'card'],
			shadow: ['raised', 'float'],
			spacing: ['control-sm', 'control-md', 'control-lg', 'row'],
			ease: ['pop'],
			blur: ['fade', 'glass'],
			tracking: ['label'],
		},
		classGroups: {
			duration: [{ duration: ['fast', 'base', 'slow', 'travel'] }],
		},
	},
})
