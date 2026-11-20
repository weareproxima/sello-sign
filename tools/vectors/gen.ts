// Signs reference documents with the TS signers of ../sign-old, with a fixed
// key, clock and randomness, so the Go signers can be compared byte for byte.
// Reads ../sign-old (or SIGN_OLD, its src/lib), writes only
// apps/api/internal/signing/testdata/vectors/<cc>/.
//
//   bun run gen.ts            every country under countries/
//   bun run gen.ts do pa      only those
import {
	mkdirSync,
	readdirSync,
	readFileSync,
	rmSync,
	writeFileSync,
} from 'node:fs'
import { NOW, install } from './lib/determinism'
import { reset } from './lib/random'
import { specSamples } from './lib/samples'
import type { CountryConfig } from './lib/types'

const SIGN_OLD =
	process.env.SIGN_OLD ??
	new URL('../../../sign-old/src/lib', import.meta.url).pathname
const HERE = import.meta.dir
const TESTDATA = new URL(
	'../../apps/api/internal/signing/testdata/',
	import.meta.url
).pathname

install(SIGN_OLD)

const material = {
	privateKeyPem: readFileSync(`${TESTDATA}key.pem`, 'utf8'),
	certificatePem: readFileSync(`${TESTDATA}cert.pem`, 'utf8'),
}

const { getCountrySigner } = await import(
	`${SIGN_OLD}/server/api/signers/index.ts`
)

const wanted = process.argv.slice(2)
const codes = readdirSync(`${HERE}/countries`)
	.filter((f) => f.endsWith('.ts'))
	.map((f) => f.slice(0, -3))
	.filter((cc) => wanted.length === 0 || wanted.includes(cc))
	.sort()

const extension = (contentType: string) =>
	contentType === 'application/json' ? 'json' : 'xml'

for (const cc of codes) {
	const config: CountryConfig = (await import(`${HERE}/countries/${cc}.ts`))
		.default
	const signer = getCountrySigner(config.country)
	const spec = await specSamples(SIGN_OLD, cc, config.spec ?? [])
	const dir = `${TESTDATA}vectors/${cc}/`
	rmSync(dir, { recursive: true, force: true })
	mkdirSync(dir, { recursive: true })

	const cases = []
	const seen = new Set<string>()
	for (const c of config.cases(spec)) {
		if (!/^[a-z0-9-]+$/.test(c.name) || seen.has(c.name))
			throw new Error(`${cc}: bad or repeated case name ${c.name}`)
		seen.add(c.name)
		const contentType = c.contentType ?? config.contentType ?? 'application/xml'
		const input = `${c.name}.in.${extension(contentType)}`
		writeFileSync(dir + input, c.body)

		const random = reset(`${cc}/${c.name}`)
		const entry: Record<string, unknown> = {
			name: c.name,
			note: c.note,
			content_type: contentType,
			input,
		}
		try {
			const out = signer.sign(material, c.body, contentType)
			const output = `${c.name}.out.${config.country === 'SV' ? 'jws' : 'xml'}`
			writeFileSync(dir + output, out.signature)
			const self = signer.verify(material, out.signature)
			Object.assign(entry, {
				output,
				document_id: out.documentId,
				document_type: out.documentType,
				self_verified: self.valid,
			})
			console.log(
				`${cc} ${c.name}: signed (${out.signature.length} bytes), its own verify says ${self.valid}`
			)
		} catch (err) {
			entry.error = (err as Error).message
			console.log(`${cc} ${c.name}: NOT signed: ${entry.error}`)
		}
		entry.random = random.hex()
		entry.random_calls = random.calls
		cases.push(entry)
	}

	const manifest = {
		country: config.country,
		generated_by: 'tools/vectors/gen.ts',
		now: NOW.toISOString(),
		key: '../../key.pem',
		certificate: '../../cert.pem',
		cases,
	}
	writeFileSync(
		`${dir}manifest.json`,
		JSON.stringify(manifest, null, '\t') + '\n'
	)
}
