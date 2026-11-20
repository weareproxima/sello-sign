import { useQuery } from '@tanstack/react-query'
import { useParams } from '@tanstack/react-router'
import { api, type Role, type WorkspaceSummary } from './api'

export const meQuery = { queryKey: ['me'], queryFn: () => api.me() } as const

export function useMe() {
	return useQuery(meQuery)
}

/** The space the address names, as `/me` lists it, with the person's role. */
export function useWorkspace(): {
	workspaceId: string
	workspace: WorkspaceSummary | null
	role: Role | null
	canManage: boolean
} {
	const { workspaceId } = useParams({ from: '/_app/w/$workspaceId' })
	const me = useMe()
	const workspace =
		me.data?.workspaces.find((w) => w.id === workspaceId) ?? null
	const role = workspace?.role ?? null
	return {
		workspaceId,
		workspace,
		role,
		canManage: role === 'owner' || role === 'admin',
	}
}

const LAST = 'sign.workspace'

/** The last space opened in this browser: a convenience, nothing depends on it. */
export function rememberWorkspace(id: string) {
	try {
		localStorage.setItem(LAST, id)
	} catch {
		/* private mode */
	}
}

export function lastWorkspace(): string | null {
	try {
		return localStorage.getItem(LAST)
	} catch {
		return null
	}
}
