/**
 * Where to land after signing in: a path of this panel, never another
 * origin (`//evil.example` or `https://…` would leave it).
 */
export function safeRedirect(value: string | undefined | null): string {
	if (!value || !value.startsWith('/') || value.startsWith('//')) return '/'
	if (value.startsWith('/\\')) return '/'
	return value
}
