/**
 * Saving a file the API made (an export) from the browser: the API answers
 * it to the token in `Authorization`, so it can't be a plain link.
 */

/** The export's name, as the API's `Content-Disposition` says it:
 * `sello-sign-<space>-<YYYY-MM-DD>.json`, the date in UTC. */
export function exportFileName(workspaceId: string, now: Date): string {
	return `sello-sign-${workspaceId}-${now.toISOString().slice(0, 10)}.json`
}

/** Hands the browser `blob` to save as `name`. */
export function saveFile(blob: Blob, name: string) {
	const url = URL.createObjectURL(blob)
	const link = document.createElement('a')
	link.href = url
	link.download = name
	document.body.append(link)
	link.click()
	link.remove()
	// After the click has taken it.
	setTimeout(() => URL.revokeObjectURL(url), 0)
}
