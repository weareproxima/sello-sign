// Fixes what the TS signers take from the world, inside this process only and
// without touching ../sign-old: the clock (Date), the Web Crypto randomness on
// globalThis.crypto, Math.random, and — through a loader plugin that rewrites
// their imports — the random functions of Node's 'crypto' module.
import { plugin } from 'bun'
import { stream } from './random'

/** The instant every signature is made at. */
export const NOW = new Date('2026-11-19T15:04:05.123Z')

const RealDate = Date
const fixed = NOW.getTime()

class FixedDate extends RealDate {
	constructor(...args: unknown[]) {
		if (args.length === 0) super(fixed)
		else super(...(args as [number]))
	}
	static now(): number {
		return fixed
	}
}

let installed = false

export function install(signOld: string): void {
	if (installed) return
	installed = true
	globalThis.Date = FixedDate as DateConstructor
	Math.random = () => stream().float()
	Object.defineProperty(globalThis.crypto, 'randomUUID', {
		configurable: true,
		value: () => stream().uuid(),
	})
	Object.defineProperty(globalThis.crypto, 'getRandomValues', {
		configurable: true,
		value: <T extends ArrayBufferView>(view: T) => stream().fill(view),
	})

	const fake = new URL('./fake-crypto.ts', import.meta.url).pathname
	const signers = `${signOld}/server/api/signers/`
	plugin({
		name: 'vectors-crypto',
		setup(build) {
			build.onLoad(
				{ filter: /\/sign-old\/src\/lib\/.*\.ts$/ },
				async ({ path }) => {
					let contents = await Bun.file(path).text()
					if (path.startsWith(signers)) {
						contents = contents.replace(
							/from ['"](?:node:)?crypto['"]/g,
							`from '${fake}'`
						)
					}
					return { contents, loader: 'ts' }
				}
			)
		},
	})
}
