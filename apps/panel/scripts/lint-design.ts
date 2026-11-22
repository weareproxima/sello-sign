/**
 * `bun run lint:design`: checks that the panel's own code keeps to Sello's
 * design system.
 *
 * It is Sello's `lint:design` (`../sello/packages/ui/scripts/lint-design.ts`,
 * by way of `../sign-old/scripts/design/lint-design.ts`) reading the panel's
 * TSX and CSS: the rules of Sello's `DESIGN.md` that can be checked by reading
 * the code. No sizes, colours, radii, shadows, durations or opacities outside
 * the tokens, no shadcn aliases, no colours with `dark:`, no keys on view.
 * What can't be checked this way (which role a text takes, which height a
 * control) is in `DESIGN.md` and is reviewed by hand.
 *
 * An exception is marked with a `ds-allow: <why>` comment on the same line or
 * right above it (in CSS it lasts until the `;` that closes the declaration).
 * Without the why it isn't an exception but a shortcut.
 *
 * It reads all of `src/` except what comes from the `@sello` registry and is
 * not edited here (`src/components/ui/`, `src/lib/utils.ts`,
 * `src/lib/button-variants.ts`, `src/lib/shortcuts.ts`, `src/styles/sello/`:
 * they change in
 * `../sello/packages/ui`, which lints them), the generated route tree, the
 * tests, and the e-mails (`src/lib/email.ts`, `src/lib/*-email.ts`, `src/lib/sales-inquiry.ts`),
 * whose HTML carries written colours because no mail client reads variables.
 *
 * The messages are in Spanish, like DESIGN.md's.
 */
/// <reference types="node" />
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'

const ROOT = join(import.meta.dirname, '..')

type Kind = 'code' | 'css' | 'any'

type Rule = {
	id: string
	pattern: RegExp
	message: string
	/** Only in code (classes), only in CSS, or both; code by default. */
	kind?: Kind
	/** Files the rule doesn't apply to (relative to `apps/panel`). */
	except?: RegExp
}

const COLOR_UTILITY =
	'(?:bg|text|border|ring|outline|decoration|fill|stroke|from|via|to|shadow|divide|placeholder|caret)'
const TOKENS =
	'(?:foreground|background|muted(?:-foreground)?|primary(?:-foreground|-hover)?|secondary|accent|border|input|ring|card|popover|fill(?:-hover|-active)?|subtle|surface|raised|faint|selected|glass(?:-page)?|scrim|line(?:-strong)?|ok(?:-fill)?|warn(?:-fill)?|bad(?:-fill)?|destructive|black|white|current|sidebar[\\w-]*)'

export const RULES: Array<Rule> = [
	{
		id: 'tamaño',
		pattern: /\btext-(?:2xs|xs|sm|base|lg|xl|[2-9]xl)\b/,
		message:
			'Tamaño de Tailwind: usa un papel del sistema (text-label, -meta, -ui, -field, -body, -lede, -h3, -h2, -h1, -d2, -d1). DESIGN.md › Tipografía.',
	},
	{
		id: 'tamaño-arbitrario',
		pattern: /\btext-\[(?:\d|\.\d|calc|clamp|var\(--text)/,
		message:
			'Tamaño a mano: si ningún papel sirve, falta un papel en tokens.css, no un valor aquí. DESIGN.md › Tipografía.',
	},
	{
		id: 'interlínea',
		pattern:
			/\bleading-(?:\[|tight\b|snug\b|normal\b|relaxed\b|loose\b|(?:[0-38-9]|1\d)\b)/,
		message:
			'Cada papel trae su interlínea. Solo se permite leading-none y leading-4…7 para ajustar. DESIGN.md › Tipografía.',
	},
	{
		id: 'espaciado-de-letra',
		pattern: /\btracking-(?!label\b|normal\b)[\w[\].-]+/,
		message:
			'Los titulares traen su espaciado; para las versalitas de datos, tracking-label. DESIGN.md › Tipografía.',
	},
	{
		id: 'peso',
		pattern: /\bfont-(?:thin|extralight|light|bold|extrabold|black)\b/,
		message:
			'Pesos del sistema: font-normal, font-medium y font-semibold. DESIGN.md › Tipografía.',
	},
	{
		id: 'opacidad',
		pattern: new RegExp(`\\b${COLOR_UTILITY}-${TOKENS}\\/[\\w.[\\]]+`),
		message:
			'Opacidad a mano sobre un color: usa el token que ya lo dice (fill-hover, fill-active, glass, scrim…) o añade uno a tokens.css. DESIGN.md › Color.',
	},
	{
		id: 'opacidad-de-pieza',
		pattern: /(?<![-\w])opacity-(?!0\b|100\b)\d+\b|\bopacity-\[/,
		message:
			'Una pieza a media opacidad es un color improvisado: lo apagado es text-faint, lo secundario text-muted-foreground. Solo opacity-0 y -100 para aparecer y desaparecer. DESIGN.md › Color.',
	},
	{
		id: 'color-arbitrario',
		pattern: new RegExp(
			`\\b${COLOR_UTILITY}-\\[(?:#|rgb|hsl|oklch|oklab|lab\\(|lch\\(|color-mix|var\\()`
		),
		message: 'Color fuera de los tokens. DESIGN.md › Color.',
	},
	{
		id: 'paleta-de-tailwind',
		pattern:
			/\b(?:bg|text|border|ring|outline|decoration|fill|stroke|from|via|to|divide|placeholder|caret|shadow)-(?:slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-\d{2,3}\b|\b(?:bg|text|border|ring|fill|stroke)-(?:black|white)\b/,
		message:
			'La paleta de Tailwind no existe en Sello: usa los tokens (bg-fill, text-muted-foreground, text-ok…). DESIGN.md › Color.',
	},
	{
		id: 'alias-de-shadcn',
		pattern:
			/\b(?:bg|border|ring|outline|decoration|divide)-(?:muted|secondary|accent|card|popover|destructive|input|sidebar|border)(?:-[\w-]+)?\b|\btext-(?:secondary|accent|card|popover|destructive|sidebar|primary)(?!-)\b|\btext-(?:secondary|accent|card|popover|sidebar)-[\w-]+\b|\btext-muted(?![-\w])/,
		message:
			'Alias de shadcn: se usan los nombres del sistema (bg-fill, bg-surface, border-line, text-bad…). DESIGN.md › Color.',
	},
	{
		id: 'color-de-noche',
		pattern: new RegExp(`(?:^|[\\s"'\`{])dark:(?:[\\w-]+:)*${COLOR_UTILITY}-`),
		message:
			'Los colores no se escriben con dark:: el token ya cambia de noche. Solo cuando una pieza cambia de capa, con ds-allow. DESIGN.md › Modo oscuro.',
	},
	{
		id: 'radio',
		pattern:
			/\brounded(?:-(?:t|r|b|l|s|e|tl|tr|br|bl|ss|se|es|ee))?-(?:xs|sm|md|lg|xl|2xl|3xl|4xl)\b|\brounded(?:-(?:t|r|b|l|s|e|tl|tr|br|bl|ss|se|es|ee))?-\[(?!inherit\])|(?<![-\w])rounded(?:-(?:t|r|b|l|s|e|tl|tr|br|bl|ss|se|es|ee))?(?=[\s"'`]|$)/,
		message:
			'Radios del sistema: rounded-small (6), -row (12), -panel (16), -card (24) y -full. DESIGN.md › Radios.',
	},
	{
		id: 'sombra',
		pattern:
			/\bshadow-(?:2xs|xs|sm|md|lg|xl|2xl|inner|\[)|(?<![-\w])shadow(?=[\s"'`]|$)/,
		message:
			'Elevación del sistema: shadow-raised (lo que se apoya) y shadow-float (lo que flota). DESIGN.md › Superficies.',
	},
	{
		id: 'anillo',
		pattern: /(?<![-\w])ring(?:-[0-8])?(?=[\s"'`]|$)|\bring-\[/,
		message:
			'Un anillo alrededor de una pieza es un borde: se separa con el tono y la sombra. El foco lo dibuja la base. DESIGN.md › Superficies.',
	},
	{
		id: 'duración',
		pattern: /\bduration-(?!fast\b|base\b|slow\b|travel\b|0\b)[\w[\]().-]+/,
		message:
			'Duraciones del sistema: duration-fast (120), -base (200), -slow (320), -travel (560). DESIGN.md › Movimiento.',
	},
	{
		id: 'curva',
		pattern: /\bease-(?:\[|in-out\b)/,
		message:
			'Curvas del sistema: ease-out (entrar), ease-in (salir), ease-pop (confirmar) y ease-linear para bucles. DESIGN.md › Movimiento.',
	},
	{
		id: 'atajo-visible',
		pattern: /<Kbd\b|<kbd\b/,
		message:
			'El panel no enseña teclas a la vista: van en el tooltip de su control. Si es una excepción, márcala con `ds-allow` y di por qué. DESIGN.md › Atajos visibles.',
	},
	{
		id: 'estilo-en-línea',
		pattern:
			/style=\{\{[^}]*(?:#[0-9a-fA-F]{3,8}\b|\b(?:rgba?|hsla?|oklch|oklab)\()/,
		message:
			'Color escrito en un style: usa var(--…) de tokens.css. DESIGN.md › Color.',
	},
	{
		id: 'color-literal',
		kind: 'css',
		pattern: /#[0-9a-fA-F]{3,8}\b|\b(?:rgba?|hsla?|oklch|oklab|lab|lch)\(/,
		message:
			'Color escrito a mano: usa var(--bg), var(--fill), var(--text-2)… de tokens.css. DESIGN.md › Color.',
	},
	{
		id: 'alias-de-shadcn-en-css',
		kind: 'css',
		pattern:
			/var\(--(?:background|foreground|muted|muted-foreground|primary|primary-foreground|secondary|accent|border|input|ring|card|popover|destructive|sidebar[\w-]*)\)/,
		message:
			'Variable de shadcn en CSS: usa las del sistema (--bg, --text, --text-2, --fill, --line, --focus…). DESIGN.md › Color.',
	},
	{
		id: 'retirado',
		kind: 'any',
		pattern: /\b(?:ok|warn|bad)-surface\b|\bmuted-strong\b/,
		message:
			'Token que ya no existe: los fondos de veredicto son ok-fill, warn-fill y bad-fill, y el gris de texto sobre relleno es text-muted-foreground. DESIGN.md › Color.',
	},
	{
		id: 'cn-sin-tema',
		pattern: /from ['"](?:cn|clsx|tailwind-merge)['"]/,
		message:
			'Importa `cn` de `#/lib/utils`: el de `cn` a secas no conoce las escalas del sistema y, al juntar `text-ui` con un color, se queda con el color y borra el tamaño. DESIGN.md › Código.',
	},
]

/** What the panel doesn't own or doesn't style. */
const SKIP = [
	/^src\/components\/ui\//,
	/^src\/lib\/utils\.ts$/,
	/^src\/lib\/button-variants\.ts$/,
	/^src\/lib\/shortcuts\.ts$/,
	/^src\/styles\/sello\//,
	/\.gen\.ts$/,
	/\.test\.tsx?$/,
	/^src\/lib\/(?:[\w-]+-)?email\.ts$/,
	/^src\/lib\/sales-inquiry\.ts$/,
]

function walk(dir: string, out: Array<string> = []) {
	for (const name of readdirSync(join(ROOT, dir))) {
		const rel = join(dir, name)
		if (name === 'node_modules') continue
		if (statSync(join(ROOT, rel)).isDirectory()) walk(rel, out)
		else if (/\.(?:ts|tsx|css)$/.test(name)) out.push(rel)
	}
	return out
}

export type Finding = { file: string; line: number; rule: Rule; text: string }

/** The findings in one file's text; `file` is relative to `apps/panel`. */
export function lintText(file: string, text: string): Array<Finding> {
	const findings: Array<Finding> = []
	const css = file.endsWith('.css')
	let inBlock = false
	let allowNext = false
	let allowUntilEnd = false

	text.split('\n').forEach((raw, i) => {
		const trimmed = raw.trim()
		// Comments aren't checked, but a `ds-allow` in one excuses what follows.
		const startsBlock = /^(?:\{?\/\*)/.test(trimmed)
		const comment =
			inBlock ||
			startsBlock ||
			trimmed.startsWith('//') ||
			trimmed.startsWith('*')
		if (startsBlock && !trimmed.includes('*/')) inBlock = true
		else if (inBlock && trimmed.includes('*/')) inBlock = false
		if (comment) {
			if (raw.includes('ds-allow')) allowNext = true
			return
		}
		if (!trimmed) return

		const allowed = raw.includes('ds-allow') || allowNext || allowUntilEnd
		if (allowNext && css && !/[;{}]/.test(trimmed)) allowUntilEnd = true
		else if (allowUntilEnd && /[;{}]/.test(trimmed)) allowUntilEnd = false
		allowNext = false
		if (allowed) return

		// A comment at the end of the line isn't classes.
		const code = raw.replace(/\s\/\/\s.*$/, '').replace(/\/\*.*?\*\//g, '')
		for (const rule of RULES) {
			const kind = rule.kind ?? 'code'
			if (kind !== 'any' && (kind === 'css') !== css) continue
			if (rule.except?.test(file)) continue
			if (rule.pattern.test(code))
				findings.push({ file, line: i + 1, rule, text: trimmed })
		}
	})
	return findings
}

function main() {
	const files = walk('src')
		.map((f) => relative(ROOT, join(ROOT, f)))
		.filter((f) => !SKIP.some((re) => re.test(f)))
	const findings = files.flatMap((file) =>
		lintText(file, readFileSync(join(ROOT, file), 'utf8'))
	)
	if (findings.length === 0) {
		console.log(
			`lint:design — ${files.length} archivos sin nada fuera del sistema de diseño.`
		)
		return 0
	}
	for (const f of findings)
		console.log(
			`${f.file}:${f.line}  [${f.rule.id}] ${f.rule.message}\n    ${f.text.slice(0, 160)}\n`
		)
	console.log(
		`lint:design — ${findings.length} ${findings.length === 1 ? 'problema' : 'problemas'}. Las reglas están en ../sello/DESIGN.md.`
	)
	return 1
}

if (import.meta.main) process.exit(main())
