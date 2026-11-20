/** What the document has to be, before sending it. `null` if it looks right. */
export function checkDocument(
	text: string,
	format: 'json' | 'xml'
): string | null {
	const body = text.trim()
	if (!body) return 'El documento no puede estar vacío.'
	if (format === 'json') {
		try {
			JSON.parse(body)
			return null
		} catch {
			return 'El documento debe ser un JSON válido. Revise su estructura y vuelva a intentarlo.'
		}
	}
	return body.startsWith('<') ? null : 'El documento debe ser un XML válido.'
}
