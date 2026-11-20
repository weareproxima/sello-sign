import { describe, expect, it } from 'vitest'
import { accountCenterPage, describeUserAgent } from './user-agent'

describe('describeUserAgent', () => {
	it('names the browser and the system', () => {
		expect(
			describeUserAgent(
				'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36'
			)
		).toBe('Chrome en macOS')
		expect(
			describeUserAgent(
				'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1'
			)
		).toBe('Safari en iOS')
		expect(
			describeUserAgent(
				'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0'
			)
		).toBe('Edge en Windows')
		expect(
			describeUserAgent(
				'Mozilla/5.0 (X11; Linux x86_64; rv:132.0) Gecko/20100101 Firefox/132.0'
			)
		).toBe('Firefox en Linux')
	})

	it('says what it can', () => {
		expect(describeUserAgent(null)).toBeNull()
		expect(describeUserAgent('curl/8.0')).toBe('Navegador desconocido')
	})
})

describe('accountCenterPage', () => {
	it('points to the section under Sello Sign, with the way back', () => {
		expect(
			accountCenterPage(
				'https://account.sello.tax',
				'security',
				'https://sign.sello.tax/w/1/account'
			)
		).toBe(
			'https://account.sello.tax/u/0/sign/security?redirect=https%3A%2F%2Fsign.sello.tax%2Fw%2F1%2Faccount'
		)
	})
})
