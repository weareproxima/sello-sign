/**
 * The plan chosen on Sello's site. Its pricing links to
 * `/register?plan=<bucket>` (now `/sign-up?plan=`); the choice can't ride
 * through signing in with Sello's account and choosing a space, so it waits
 * in a cookie for an hour, as in ../sign-old (`selected_plan`). In the cloud,
 * once the person opens a space they manage, the panel sends them to buy it
 * in Sello's account center, «Su cuenta › Sello Sign › Plan y facturación»,
 * for that space's organization. Self-hosted, the plan is ignored: the
 * server never sets the cookie.
 *
 * The cookie isn't `HttpOnly`: it only names a public plan, and the page that
 * opens the space reads it.
 */

/** Sign's Pro plans, as Sello's site and billing name them. */
export const PRO_BUCKETS = [
	'pro_starter',
	'pro_growth',
	'pro_scale',
	'pro_business',
] as const

export type ProBucket = (typeof PRO_BUCKETS)[number]

export function isProBucket(value: unknown): value is ProBucket {
	return (PRO_BUCKETS as readonly unknown[]).includes(value)
}

export const PLAN_COOKIE = 'selected_plan'

/** The pages that take `?plan=`: the new ones and those of the old Sign. */
const PLAN_PAGES = ['/sign-up', '/sign-in', '/register', '/login']

/**
 * The `Set-Cookie` that keeps the plan of a request to one of those pages, or
 * `null` (no plan, an unknown one, or not the cloud).
 */
export function planCookieFor(url: URL, cloud: boolean): string | null {
	if (!cloud) return null
	const path = url.pathname.replace(/\/+$/, '')
	if (!PLAN_PAGES.includes(path)) return null
	const plan = url.searchParams.get('plan')
	if (!isProBucket(plan)) return null
	const secure = url.protocol === 'https:' ? '; Secure' : ''
	return `${PLAN_COOKIE}=${plan}; Path=/; Max-Age=3600; SameSite=Lax${secure}`
}

/** The plan in a `Cookie` header or `document.cookie`, if it's a known one. */
export function planIn(cookies: string): ProBucket | null {
	for (const part of cookies.split(';')) {
		const [name, ...rest] = part.trim().split('=')
		if (name === PLAN_COOKIE) {
			const value = rest.join('=')
			return isProBucket(value) ? value : null
		}
	}
	return null
}

/**
 * Where a manager buys the plan for a space: the account center's Sign plan
 * page for its organization at Sello (`accountCenterUrl` of ../sign-old).
 */
export function planPageUrl(
	accountCenter: string,
	organization: string
): string {
	const url = new URL('/u/0/sign/plan', `${accountCenter}/`)
	url.searchParams.set('organization', organization)
	return url.toString()
}

/** The plan waiting in this browser, without taking it. */
export function pendingPlan(): ProBucket | null {
	if (typeof document === 'undefined') return null
	return planIn(document.cookie)
}

/** Forgets the plan: it's used once, whatever the person's role. */
export function clearPlan() {
	if (typeof document === 'undefined') return
	document.cookie = `${PLAN_COOKIE}=; Path=/; Max-Age=0; SameSite=Lax`
}

/**
 * Takes the waiting plan for the organization at Sello of a space just
 * opened: the account center's address to buy it if the person manages that
 * organization, or `null` (and the plan is gone either way).
 */
export function takePlanFor(options: {
	accountCenter: string | null
	organization: string | null | undefined
	role: string
}): string | null {
	const plan = pendingPlan()
	if (!plan) return null
	clearPlan()
	if (!options.accountCenter || !options.organization) return null
	if (options.role === 'member') return null
	return planPageUrl(options.accountCenter, options.organization)
}
