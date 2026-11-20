import { Outlet, createFileRoute, redirect } from '@tanstack/react-router'
import { Logo } from '#/components/logo'
import { getPublicConfig, getSession } from '#/server/session'

/**
 * The screens to sign in: on the band, the form's card rising over it.
 * Someone already signed in goes to the panel.
 */
export const Route = createFileRoute('/_auth')({
	beforeLoad: async () => {
		if (await getSession()) throw redirect({ to: '/' })
		return { config: await getPublicConfig() }
	},
	component: AuthLayout,
})

function AuthLayout() {
	return (
		<div className="flex min-h-svh flex-col bg-subtle">
			<main className="flex flex-1 flex-col items-center justify-center gap-6 px-4 py-8 sm:p-6 md:p-10">
				<Logo className="h-8" />
				<div className="w-full max-w-sm">
					<Outlet />
				</div>
			</main>
			<footer className="px-4 py-6 text-center text-meta text-muted-foreground">
				Sello Sign · AGPL-3.0
			</footer>
		</div>
	)
}
