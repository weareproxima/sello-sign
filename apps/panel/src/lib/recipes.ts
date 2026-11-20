/**
 * Recipes of ../sign-old's `recipes.ts` that the panel uses. Strings are
 * written whole: Tailwind finds classes by reading this file.
 */

/** The segmented control: a pill of fill whose pressed segment rises. */
export const SEGMENTED =
	'inline-flex items-center gap-0.5 rounded-full bg-fill p-0.5'

export const SEGMENT =
	// ds-allow: by night the pressed segment changes layer, not colour (DESIGN.md › Modo oscuro)
	'rounded-full text-muted-foreground transition-colors duration-fast hover:text-foreground aria-checked:bg-raised aria-checked:text-foreground aria-checked:shadow-raised aria-pressed:bg-raised aria-pressed:text-foreground aria-pressed:shadow-raised aria-selected:bg-raised aria-selected:text-foreground aria-selected:shadow-raised'

/** A link inside running text. */
export const TEXT_LINK =
	'font-medium text-foreground underline decoration-line-strong underline-offset-4 transition-colors duration-fast hover:decoration-foreground'

/** A block of code: a fill, in mono. */
export const CODE_BLOCK =
	'overflow-x-auto rounded-panel bg-fill p-4 font-mono text-meta whitespace-pre-wrap break-all'
