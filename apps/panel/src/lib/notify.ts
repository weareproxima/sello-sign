import { toast } from '#/components/ui/toast'

/** A toast: what happened, in the participle («Clave creada.»). */
export function notify(title: string, type: 'success' | 'error' = 'success') {
	toast.add({ title, type })
}
