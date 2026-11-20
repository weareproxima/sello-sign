import { useQuery } from '@tanstack/react-query'
import { useRouteContext } from '@tanstack/react-router'
import { ExternalLinkIcon } from 'lucide-react'
import { FormError } from '#/components/page'
import { ApiError, api, errorMessage } from '#/lib/api'
import { buttonVariants } from '#/lib/button-variants'
import { limitKind } from '#/lib/limits'
import { planPageUrl } from '#/lib/plan'
import { useWorkspace } from '#/lib/workspace'

/**
 * Where whoever manages the space changes its plan: in the cloud, «Su cuenta
 * › Sello Sign › Plan y facturación» of its organization at Sello
 * (`planPageUrl`), the page the panel already sends a plan chosen on Sello's
 * site to. `null` self-hosted, for a member, for a space without an
 * organization at Sello (the platform's), or until the space is read.
 * `enabled` false skips reading it.
 */
export function usePlanPage(enabled = true): {
	url: string | null
	/** In the cloud, a space whose plan is bought at Sello. */
	cloud: boolean
} {
	const { config } = useRouteContext({ from: '/_app' })
	const { workspaceId, canManage } = useWorkspace()
	const workspace = useQuery({
		queryKey: ['workspace', workspaceId],
		queryFn: () => api.workspace(workspaceId),
		enabled: enabled && config.accountCenter !== null,
	})
	const organization = workspace.data?.sello_workspace_id ?? null
	const cloud = config.accountCenter !== null && organization !== null
	return {
		url:
			cloud && canManage
				? planPageUrl(config.accountCenter!, organization!)
				: null,
		cloud,
	}
}

/**
 * A failed action, as `FormError` says it (`errorMessage`, which words the
 * plan's limits: whose per-minute limit, when to retry, when the month
 * starts again). When the limit is the plan's (the month's signatures, the
 * organizations), it adds where to change the plan.
 */
export function ActionError({
	error,
	action,
}: {
	error: unknown
	action: string
}) {
	const kind = error instanceof ApiError ? limitKind(error.problem) : null
	const ofThePlan = kind === 'monthly' || kind === 'organizations'
	const plan = usePlanPage(ofThePlan)
	if (!error) return null
	return (
		<FormError message={errorMessage(error, action)}>
			{!ofThePlan ? null : plan.url ? (
				<a
					href={plan.url}
					className={buttonVariants({ variant: 'outline', size: 'sm' })}
				>
					<ExternalLinkIcon />
					Cambiar el plan
				</a>
			) : plan.cloud ? (
				<span className="text-meta">
					Quien administra el espacio puede cambiar su plan en Sello.
				</span>
			) : null}
		</FormError>
	)
}
