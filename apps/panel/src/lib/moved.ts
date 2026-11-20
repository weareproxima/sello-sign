/**
 * The addresses of the old Sign (../sign-old, SvelteKit) that this panel
 * replaced in `sign.sello.tax`. Sello's site, the e-mails already sent and
 * bookmarks still point at them, so each one answers with a permanent
 * redirect (308) to where it lives now, keeping its query (`?plan=`,
 * `?redirect=`). Pages of the old panel that have no address here without a
 * space (`/settings/…`, `/issuers/…`) go to `/`, which opens the last space.
 *
 * `/forgot-password`, `/reset-password` and `/workspaces/new` kept their
 * address: they aren't here. The pages of this panel that moved are
 * below (`ACCOUNT_IN_A_SPACE`).
 */
const MOVED: ReadonlyArray<readonly [from: string, to: string]> = [
	['/register', '/sign-up'],
	['/login', '/sign-in'],
	['/dashboard', '/'],
	['/organizations/new', '/workspaces/new'],
	['/settings', '/'],
	['/issuers', '/'],
	['/invite', '/'],
	['/switch-workspace', '/'],
]

/** «Su cuenta» hung from a space (`/w/<id>/account`) until 2026-11-20; it
 * lives at `/account`, outside any space. */
const ACCOUNT_IN_A_SPACE = /^\/w\/[^/]+\/account$/

/** Whether `pathname` is `prefix` or under it (`/login/two-factor`). */
function under(pathname: string, prefix: string): boolean {
	return pathname === prefix || pathname.startsWith(`${prefix}/`)
}

/**
 * Where an old address lives now, absolute or a path of this panel with the
 * request's query, or `null` if `url` isn't one. `apiUrl` is the API's origin:
 * the specification moved there (`/api/openapi` → `<API_URL>/openapi.yaml`).
 */
export function movedTarget(url: URL, apiUrl: string): string | null {
	const path = url.pathname.replace(/\/+$/, '') || '/'
	if (path === '/api/openapi') return `${apiUrl}/openapi.yaml`
	if (ACCOUNT_IN_A_SPACE.test(path)) return `/account${url.search}`
	for (const [from, to] of MOVED) {
		if (under(path, from)) return `${to}${url.search}`
	}
	return null
}

/** The redirect for an old address, or `null`. */
export function movedResponse(
	request: Request,
	apiUrl: string
): Response | null {
	if (request.method !== 'GET' && request.method !== 'HEAD') return null
	const target = movedTarget(new URL(request.url), apiUrl)
	if (!target) return null
	return new Response(null, { status: 308, headers: { location: target } })
}
