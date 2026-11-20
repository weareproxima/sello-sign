import { useQuery } from '@tanstack/react-query'
import { useParams } from '@tanstack/react-router'
import { api } from './api'

export const organizationKey = (workspaceId: string, organizationId: string) =>
	['organization', workspaceId, organizationId] as const

/** The organization the address names. */
export function useOrganization() {
	const { workspaceId, organizationId } = useParams({
		from: '/_app/w/$workspaceId/organizations/$organizationId',
	})
	const query = useQuery({
		queryKey: organizationKey(workspaceId, organizationId),
		queryFn: () => api.organization(workspaceId, organizationId),
	})
	return { workspaceId, organizationId, ...query }
}
