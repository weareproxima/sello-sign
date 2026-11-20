// What the TS signers get when they import 'crypto': Node's module with its
// random functions replaced by the case's stream (lib/random.ts). The
// plugin in lib/determinism.ts rewrites their imports to point here.
import * as real from 'node:crypto'
import { stream } from './random'

export * from 'node:crypto'

export function randomUUID(): string {
	return stream().uuid()
}

export function randomInt(
	min: number,
	max?: number | ((...a: unknown[]) => void),
	cb?: (...a: unknown[]) => void
): number {
	if (typeof max === 'function' || cb)
		throw new Error('vectors: async randomInt is not stubbed')
	return stream().int(min, max)
}

export function randomBytes(n: number, cb?: unknown): Buffer {
	if (cb) throw new Error('vectors: async randomBytes is not stubbed')
	return stream().bytes(n)
}

export function getRandomValues<T extends ArrayBufferView>(view: T): T {
	return stream().fill(view)
}

export default { ...real, randomUUID, randomInt, randomBytes, getRandomValues }
