import { Badge } from '#/components/ui/badge'
import type { Certificate } from '#/lib/api'
import { daysUntil, formatDate } from '#/lib/format'

/** A certificate's state as a verdict: vigente, por vencer, vencido or none. */
export function certificateState(certificate: Certificate | null) {
	if (!certificate) return 'none' as const
	if (!certificate.active) return 'disabled' as const
	if (!certificate.not_after) return 'valid' as const
	const days = daysUntil(certificate.not_after)
	if (days < 0) return 'expired' as const
	if (days <= 30) return 'expiring' as const
	return 'valid' as const
}

export function CertificateBadge({
	certificate,
}: {
	certificate: Certificate | null
}) {
	const state = certificateState(certificate)
	const until = certificate?.not_after ? formatDate(certificate.not_after) : ''
	switch (state) {
		case 'none':
			return <span className="text-meta text-faint">Sin certificado</span>
		case 'disabled':
			return <Badge variant="secondary">Desactivado</Badge>
		case 'expired':
			return <Badge variant="destructive">Vencido · {until}</Badge>
		case 'expiring':
			return <Badge variant="warn">Vence el {until}</Badge>
		case 'valid':
			return <Badge variant="ok">Vigente · {until}</Badge>
	}
}
