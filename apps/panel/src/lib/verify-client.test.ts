import { existsSync, readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

// The verifier runs in the browser (DOMParser, Web Crypto) and is tested in
// ../sign-old against the real signers; here only that the copy is exact,
// when ../sign-old is next to this repository.
const here = fileURLToPath(new URL('./verify-client.ts', import.meta.url))
const original = fileURLToPath(
	new URL(
		'../../../../../sign-old/src/lib/crypto/verify-client.ts',
		import.meta.url
	)
)

describe('verify-client', () => {
	it.skipIf(!existsSync(original))(
		'is a byte-for-byte copy of ../sign-old',
		() => {
			expect(readFileSync(here, 'utf8')).toBe(readFileSync(original, 'utf8'))
		}
	)
})
