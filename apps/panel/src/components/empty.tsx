import * as React from 'react'

import {
	Empty as EmptyRoot,
	EmptyContent,
	EmptyDescription,
	EmptyHeader,
	EmptyMedia,
	EmptyTitle,
} from '#/components/ui/empty'

/**
 * What a list says when it has nothing, in one call: the registry's `Empty`
 * (a tonal panel, centred) with its icon, title, description and actions.
 */
export function Empty({
	title,
	description,
	icon,
	children,
	className,
}: {
	title: string
	description?: string
	icon?: React.ReactNode
	children?: React.ReactNode
	className?: string
}) {
	return (
		<EmptyRoot className={className}>
			<EmptyHeader>
				{icon ? <EmptyMedia variant="icon">{icon}</EmptyMedia> : null}
				<EmptyTitle>{title}</EmptyTitle>
				{description ? (
					<EmptyDescription>{description}</EmptyDescription>
				) : null}
			</EmptyHeader>
			{children ? <EmptyContent>{children}</EmptyContent> : null}
		</EmptyRoot>
	)
}
