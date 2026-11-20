import type { Environment } from '#/lib/api'
import { ENVIRONMENT_LABELS } from '#/lib/format'
import { SEGMENT, SEGMENTED } from '#/lib/recipes'
import { cn } from '#/lib/utils'

/** Test or production, as a segmented control. */
export function EnvironmentPicker({
	value,
	onChange,
}: {
	value: Environment
	onChange: (value: Environment) => void
}) {
	return (
		<div
			className={cn(SEGMENTED, 'w-fit')}
			role="radiogroup"
			aria-label="Ambiente"
		>
			{(['test', 'prod'] as const).map((env) => (
				<button
					key={env}
					type="button"
					role="radio"
					aria-checked={value === env}
					className={cn(SEGMENT, 'h-8 px-4 text-ui')}
					onClick={() => onChange(env)}
				>
					{ENVIRONMENT_LABELS[env]}
				</button>
			))}
		</div>
	)
}
