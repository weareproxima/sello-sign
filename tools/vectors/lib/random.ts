// A deterministic byte stream that stands in for the randomness of the TS
// signers, and the functions built on it. Each case gets its own stream; the
// bytes it consumed go into the manifest, so a Go test replays them through
// signing.Options.Rand and derives the same values the same way:
//
// - randomUUID(): 16 bytes; b[6] = (b[6] & 0x0f) | 0x40, b[8] = (b[8] & 0x3f)
//   | 0x80; lowercase 8-4-4-4-12 hex.
// - randomInt(min, max): Node's own algorithm. range = max - min, RAND_MAX =
//   2^48 - 1, limit = RAND_MAX - (RAND_MAX % range); repeat { x = next 6 bytes,
//   big-endian; if x < limit return min + x % range }.
// - randomBytes(n), getRandomValues(a): the next n (a.byteLength) bytes.
// - Math.random(): 8 bytes, big-endian, (x >> 11) / 2^53.
import { createHash } from 'node:crypto'

export interface RandomCall {
	fn: string
	args?: unknown[]
	result: unknown
}

export class Stream {
	private block = new Uint8Array(0)
	private offset = 0
	private counter = 0
	readonly consumed: number[] = []
	readonly calls: RandomCall[] = []

	constructor(private readonly seed: string) {}

	next(n: number): Uint8Array {
		const out = new Uint8Array(n)
		for (let i = 0; i < n; i++) {
			if (this.offset === this.block.length) {
				const counter = Buffer.alloc(4)
				counter.writeUInt32BE(this.counter++)
				this.block = new Uint8Array(
					createHash('sha256').update(this.seed).update(counter).digest()
				)
				this.offset = 0
			}
			out[i] = this.block[this.offset++]
			this.consumed.push(out[i])
		}
		return out
	}

	hex(): string {
		return Buffer.from(this.consumed).toString('hex')
	}

	uuid(): string {
		const b = this.next(16)
		b[6] = (b[6] & 0x0f) | 0x40
		b[8] = (b[8] & 0x3f) | 0x80
		const h = Buffer.from(b).toString('hex')
		const id = `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`
		this.calls.push({ fn: 'randomUUID', result: id })
		return id
	}

	int(min: number, max?: number): number {
		if (max === undefined) {
			max = min
			min = 0
		}
		const range = max - min
		const RAND_MAX = 2 ** 48 - 1
		const limit = RAND_MAX - (RAND_MAX % range)
		for (;;) {
			const x = Buffer.from(this.next(6)).readUIntBE(0, 6)
			if (x < limit) {
				const result = min + (x % range)
				this.calls.push({ fn: 'randomInt', args: [min, max], result })
				return result
			}
		}
	}

	bytes(n: number): Buffer {
		const b = Buffer.from(this.next(n))
		this.calls.push({ fn: 'randomBytes', args: [n], result: b.toString('hex') })
		return b
	}

	fill<T extends ArrayBufferView>(view: T): T {
		const b = this.next(view.byteLength)
		new Uint8Array(view.buffer, view.byteOffset, view.byteLength).set(b)
		this.calls.push({
			fn: 'getRandomValues',
			args: [view.byteLength],
			result: Buffer.from(b).toString('hex'),
		})
		return view
	}

	float(): number {
		const x = Buffer.from(this.next(8)).readBigUInt64BE(0)
		const result = Number(x >> 11n) / 2 ** 53
		this.calls.push({ fn: 'Math.random', result })
		return result
	}
}

let current: Stream | null = null

/** Starts a fresh stream for one case. */
export function reset(seed: string): Stream {
	current = new Stream(seed)
	return current
}

export function stream(): Stream {
	if (!current) throw new Error('vectors: randomness used outside a case')
	return current
}
