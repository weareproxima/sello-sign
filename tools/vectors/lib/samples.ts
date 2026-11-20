// Pulls the sample documents out of a spec of ../sign-old: everything before
// its first `describe(`, with vitest neutralized and the imports made
// absolute, written under .cache/, imported and deleted. Returns the named exports
// as they are (strings, functions, objects).
import { mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'

export async function specSamples(
	signOld: string,
	cc: string,
	names: string[]
): Promise<Record<string, any>> {
	if (names.length === 0) return {}
	const signers = `${signOld}/server/api/signers`
	const src = readFileSync(`${signers}/${cc}.spec.ts`, 'utf8')
	const cut = src.indexOf('\ndescribe(')
	const prelude =
		src
			.slice(0, cut)
			.replace(
				/import \{[^}]*\} from 'vitest';?/,
				'const beforeAll = (..._a: unknown[]) => {}; void beforeAll;'
			)
			.replace(/from '\.\/([\w-]+)'/g, `from '${signers}/$1'`)
			.replace(/from '\.\.\/([\w-]+)'/g, `from '${signOld}/server/api/$1'`)
			.replace(/from '\$lib\//g, `from '${signOld}/`) +
		`\nexport { ${names.join(', ')} }\n`
	const dir = new URL('../.cache/', import.meta.url).pathname
	mkdirSync(dir, { recursive: true })
	writeFileSync(`${dir}${cc}.ts`, prelude)
	const mod = await import(`${dir}${cc}.ts`).finally(() =>
		rmSync(`${dir}${cc}.ts`, { force: true })
	)
	const out: Record<string, any> = {}
	for (const name of names) out[name] = mod[name]
	return out
}
