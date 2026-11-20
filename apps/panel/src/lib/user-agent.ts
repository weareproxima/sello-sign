/**
 * A session's browser and system, from its `User-Agent`, as a person reads
 * them: «Chrome en macOS». Rough on purpose: it names the session, it doesn't
 * identify the device. `null` without one.
 */
export function describeUserAgent(
	ua: string | null | undefined
): string | null {
	if (!ua) return null
	const browser = /Edg\//.test(ua)
		? 'Edge'
		: /OPR\/|Opera/.test(ua)
			? 'Opera'
			: /Firefox\//.test(ua)
				? 'Firefox'
				: /Chrome\/|CriOS\//.test(ua)
					? 'Chrome'
					: /Safari\//.test(ua)
						? 'Safari'
						: null
	const system = /iPhone|iPad|iPod/.test(ua)
		? 'iOS'
		: /Android/.test(ua)
			? 'Android'
			: /Mac OS X|Macintosh/.test(ua)
				? 'macOS'
				: /Windows/.test(ua)
					? 'Windows'
					: /CrOS/.test(ua)
						? 'ChromeOS'
						: /Linux/.test(ua)
							? 'Linux'
							: null
	if (browser && system) return `${browser} en ${system}`
	return browser ?? system ?? 'Navegador desconocido'
}

/**
 * Sello's account center page for one of the panel's account sections
 * (`/u/0/sign/<section>`), coming back to `back`.
 */
export function accountCenterPage(
	accountCenter: string,
	section: 'profile' | 'security',
	back?: string
): string {
	const url = new URL(`/u/0/sign/${section}`, `${accountCenter}/`)
	if (back) url.searchParams.set('redirect', back)
	return url.toString()
}
