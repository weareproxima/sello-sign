import { describe, expect, it } from 'vitest'
import { safeRedirect } from './redirect'
import { checkDocument } from './documents'
import { exportFileName } from './download'

describe('exportFileName', () => {
	it("is the API's name, with the date in UTC", () => {
		expect(exportFileName('w1', new Date('2026-11-20T23:30:00-06:00'))).toBe(
			'sello-sign-w1-2026-11-21.json'
		)
	})
})

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
