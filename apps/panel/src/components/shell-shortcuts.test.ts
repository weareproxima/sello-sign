import { describe, expect, it } from 'vitest'
import {
	ACCOUNT_KEYS,
	NEW_ORGANIZATION_KEYS,
	PAGE_KEYS,
	SWITCH_KEY,
} from './shell-shortcuts'

describe('the keys that go to each page', () => {
	it('every page and «Su cuenta» have their own, and none starts another', () => {
		const sequences = [...Object.values(PAGE_KEYS), ACCOUNT_KEYS].map((k) =>
			k.join(' ')
		)
		expect(new Set(sequences).size).toBe(sequences.length)
		const [single] = NEW_ORGANIZATION_KEYS
		expect(sequences.some((k) => k.startsWith(`${single} `))).toBe(false)
		expect(sequences.some((k) => k.startsWith(`${SWITCH_KEY} `))).toBe(false)
		// The theme's key alone stays the theme's.
		expect(single).not.toBe('d')
	})
})
