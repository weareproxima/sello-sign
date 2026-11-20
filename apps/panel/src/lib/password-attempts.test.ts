import { randomBytes } from 'node:crypto'
import pg from 'pg'
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest'
import {
	PASSWORD_ATTEMPTS,
	PASSWORD_WINDOW_MS,
	checkPassword,
	passwordAttemptTable,
	passwordAttemptsMessage,
	postgresPasswordAttempts,
	type PasswordAttempts,
} from './password-attempts'

describe('checkPassword', () => {
	const attempts = (
		ok = true
	): PasswordAttempts & {
		take: ReturnType<typeof vi.fn>
		clear: ReturnType<typeof vi.fn>
	} => ({
		take: vi.fn(async () =>
			ok
				? { ok: true as const }
				: { ok: false as const, retryAt: new Date('2026-11-20T15:42:00Z') }
		),
		clear: vi.fn(async () => {}),
	})
	const verify = vi.fn(async (p: string) => p === 'secreto123')

	it('takes an attempt, and forgets the failures when it is right', async () => {
		const a = attempts()
		expect(await checkPassword('secreto123', verify, a)).toEqual({ ok: true })
		expect(a.take).toHaveBeenCalledTimes(1)
		expect(a.clear).toHaveBeenCalledTimes(1)
	})

	it('keeps a wrong one', async () => {
		const a = attempts()
		expect(await checkPassword('otra', verify, a)).toEqual({
			ok: false,
			reason: 'invalid-password',
		})
		expect(a.clear).not.toHaveBeenCalled()
	})

	it('takes none for an empty one', async () => {
		const a = attempts()
		expect(await checkPassword('', verify, a)).toMatchObject({
			reason: 'invalid-password',
		})
		expect(await checkPassword(null, verify, a)).toMatchObject({
			reason: 'invalid-password',
		})
		expect(a.take).not.toHaveBeenCalled()
	})

	it("doesn't check past the limit", async () => {
		const a = attempts(false)
		verify.mockClear()
		expect(await checkPassword('secreto123', verify, a)).toEqual({
			ok: false,
			reason: 'too-many-attempts',
			retryAt: '2026-11-20T15:42:00.000Z',
		})
		expect(verify).not.toHaveBeenCalled()
	})
})

describe('passwordAttemptsMessage', () => {
	it('says when to try again, in the reader’s time', () => {
		const at = '2026-11-20T15:42:00Z'
		const time = new Date(at).toLocaleTimeString('es-419', {
			hour: '2-digit',
			minute: '2-digit',
		})
		const message = passwordAttemptsMessage(at)
		expect(message).toBe(
			`Alcanzó el límite de intentos con la contraseña. Vuelva a intentarlo después de las ${time}${time.endsWith('.') ? '' : '.'}`
		)
		expect(message).not.toMatch(/\.\.$/)
		expect(passwordAttemptsMessage(undefined)).toBe(
			'Alcanzó el límite de intentos con la contraseña. Vuelva a intentarlo en 15 minutos.'
		)
	})

	it('is 5 attempts in 15 minutes', () => {
		expect(PASSWORD_ATTEMPTS).toBe(5)
		expect(PASSWORD_WINDOW_MS).toBe(15 * 60_000)
	})
})

/**
 * The table itself, against POSTGRES_TEST_URL (the root's compose.yml:
 * postgres://sign:sign@127.0.0.1:5434/sign), in a schema of its own that it
 * drops at the end. Without the variable, skipped.
 */
const url = process.env.POSTGRES_TEST_URL
describe.skipIf(!url)('postgresPasswordAttempts', () => {
	const schema = `pa_test_${randomBytes(4).toString('hex')}`
	const table = `${schema}.password_attempt`
	let pool: pg.Pool

	beforeAll(async () => {
		pool = new pg.Pool({ connectionString: url })
		await pool.query(`create schema ${schema}`)
		await pool.query(`create table ${schema}."user" (id text primary key)`)
		await pool.query(passwordAttemptTable(schema))
		// Twice: it's idempotent.
		await pool.query(passwordAttemptTable(schema))
		await pool.query(
			`insert into ${schema}."user" (id) values ('ana'), ('beto'), ('carla')`
		)
	})
	afterAll(async () => {
		await pool?.query(`drop schema if exists ${schema} cascade`)
		await pool?.end()
	})

	const clock = (start: string) => {
		let now = new Date(start)
		return {
			now: () => now,
			advance: (ms: number) => {
				now = new Date(now.getTime() + ms)
			},
		}
	}

	it('allows five in a window, then none until it ends', async () => {
		const c = clock('2026-11-20T15:00:00Z')
		const ana = postgresPasswordAttempts(pool, 'ana', { now: c.now, table })
		for (let i = 0; i < PASSWORD_ATTEMPTS; i++) {
			expect(await ana.take()).toEqual({ ok: true })
			c.advance(60_000)
		}
		expect(await ana.take()).toEqual({
			ok: false,
			retryAt: new Date('2026-11-20T15:15:00Z'),
		})
		c.advance(9 * 60_000 + 59_000)
		expect((await ana.take()).ok).toBe(false)
		// The window started at 15:00: at 15:15 a new one starts.
		c.advance(1_000)
		expect(await ana.take()).toEqual({ ok: true })
		const row = await pool.query(
			`select failures, window_start from ${table} where user_id = 'ana'`
		)
		expect(row.rows[0].failures).toBe(1)
		expect(new Date(row.rows[0].window_start).toISOString()).toBe(
			'2026-11-20T15:15:00.000Z'
		)
	})

	it('forgets the failures when the password is right', async () => {
		const c = clock('2026-11-20T16:00:00Z')
		const beto = postgresPasswordAttempts(pool, 'beto', { now: c.now, table })
		for (let i = 0; i < PASSWORD_ATTEMPTS; i++) await beto.take()
		expect((await beto.take()).ok).toBe(false)
		await beto.clear()
		expect(await beto.take()).toEqual({ ok: true })
	})

	it('lets no more than the limit through at once', async () => {
		const c = clock('2026-11-20T17:00:00Z')
		const carla = postgresPasswordAttempts(pool, 'carla', {
			now: c.now,
			table,
		})
		const results = await Promise.all(
			Array.from({ length: 12 }, () => carla.take())
		)
		expect(results.filter((r) => r.ok)).toHaveLength(PASSWORD_ATTEMPTS)
	})

	it('drops windows that ended, and goes with the account', async () => {
		const c = clock('2026-11-21T09:00:00Z')
		const ana = postgresPasswordAttempts(pool, 'ana', { now: c.now, table })
		await ana.take()
		// Beto's and Carla's windows, of yesterday, go at anyone's next check.
		const left = await pool.query(
			`select user_id from ${table} order by user_id`
		)
		expect(left.rows.map((r) => r.user_id)).toEqual(['ana'])
		await pool.query(`delete from ${schema}."user" where id = 'ana'`)
		const after = await pool.query(
			`select count(*)::int as n from ${table} where user_id = 'ana'`
		)
		expect(after.rows[0].n).toBe(0)
	})
})
