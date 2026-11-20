import { createServerFn } from '@tanstack/react-start'
import { getRequestHeaders } from '@tanstack/react-start/server'
import { getAuth } from '#/lib/auth'
import { publicConfig, readConfig } from '#/lib/config'
import { serverEnv } from '#/lib/env'

export type SessionUser = {
	id: string
	name: string
	email: string
	image: string | null
}

/** The signed-in person, or `null`; read on the server for route guards. */
export const getSession = createServerFn({ method: 'GET' }).handler(
	async (): Promise<{ user: SessionUser } | null> => {
		const session = await getAuth().api.getSession({
			headers: new Headers(getRequestHeaders() as unknown as HeadersInit),
		})
		if (!session) return null
		const { id, name, email, image } = session.user
		return { user: { id, name, email, image: image ?? null } }
	}
)

/** What the pages may know of the configuration. */
export const getPublicConfig = createServerFn({ method: 'GET' }).handler(
	async () => publicConfig(readConfig(serverEnv()))
)
