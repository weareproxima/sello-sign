import { describe, expect, it } from 'vitest'
import { safeRedirect } from './redirect'
import { checkDocument } from './documents'

describe('safeRedirect', () => {
	it('keeps paths of the panel', () => {
		expect(safeRedirect('/w/1/organizations?x=1')).toBe(
			'/w/1/organizations?x=1'
		)
	})
	it('refuses other origins', () => {
		for (const bad of [
			'//evil.example',
			'https://evil.example',
			'/\\evil',
			'',
			undefined,
		])
			expect(safeRedirect(bad)).toBe('/')
	})
})

describe('checkDocument', () => {
	it('wants JSON in El Salvador and XML elsewhere', () => {
		expect(checkDocument('{"a":1}', 'json')).toBeNull()
		expect(checkDocument('{a', 'json')).toMatch(/JSON válido/)
		expect(checkDocument('<a/>', 'xml')).toBeNull()
		expect(checkDocument('a', 'xml')).toMatch(/XML válido/)
		expect(checkDocument('  ', 'xml')).toMatch(/vacío/)
	})
})
