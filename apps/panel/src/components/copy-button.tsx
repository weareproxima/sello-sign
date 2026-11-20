import * as React from 'react'
import { CheckIcon, CopyIcon } from 'lucide-react'
import { Button } from '#/components/ui/button'

export function CopyButton({
	value,
	label = 'Copiar',
	size = 'sm',
}: {
	value: string
	label?: string
	size?: 'sm' | 'icon-sm'
}) {
	const [copied, setCopied] = React.useState(false)
	async function copy() {
		await navigator.clipboard.writeText(value)
		setCopied(true)
		setTimeout(() => setCopied(false), 1500)
	}
	return (
		<Button
			type="button"
			variant="secondary"
			size={size}
			onClick={copy}
			aria-label={size === 'icon-sm' ? label : undefined}
		>
			{copied ? <CheckIcon /> : <CopyIcon />}
			{size === 'icon-sm' ? null : copied ? 'Copiado' : label}
		</Button>
	)
}
