import { Link } from '@tanstack/react-router'
import { buttonVariants } from '#/lib/button-variants'
import { Logo } from './logo'

export function NotFound() {
	return (
		<div className="flex min-h-svh flex-col items-center justify-center gap-6 bg-subtle px-4 text-center">
			<Logo />
			<div className="space-y-2">
				<h1 className="text-h1">Esta página no existe.</h1>
				<p className="text-ui text-muted-foreground">
					Puede que el enlace esté roto o que la página se haya movido.
				</p>
			</div>
			<Link to="/" className={buttonVariants({ variant: 'secondary' })}>
				Volver al inicio
			</Link>
		</div>
	)
}
