import { afterEach, describe, expect, it } from 'vitest'
import { Database } from './database'
import { onWorkers, serverEnv } from './env'

const global = globalThis as { __env__?: Record<string, unknown> }

describe('serverEnv', () => {
	afterEach(() => {
		delete global.__env__
		delete process.env.PANEL_TEST_VALUE
	})

	it('reads the Worker bindings over process.env, only the strings', () => {
		process.env.PANEL_TEST_VALUE = 'from-process'
		global.__env__ = {
			PANEL_TEST_VALUE: 'from-binding',
			ASSETS: { fetch() {} },
		}
		const env = serverEnv()
		expect(env.PANEL_TEST_VALUE).toBe('from-binding')
		expect(env.ASSETS).toBeUndefined()
	})

	it('is process.env in Node', () => {
		process.env.PANEL_TEST_VALUE = 'from-process'
		expect(serverEnv().PANEL_TEST_VALUE).toBe('from-process')
		expect(onWorkers()).toBe(false)
	})
})

describe('Database per request', () => {
	it('refuses a query outside a request', () => {
		const database = new Database('postgres://x@127.0.0.1:1/x', {
			perRequest: true,
		})
		expect(() => database.connect()).toThrow(/outside a request/)
	})
})
