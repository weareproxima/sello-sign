import {
	HeadContent,
	Scripts,
	createRootRouteWithContext,
} from '@tanstack/react-router'
import type { QueryClient } from '@tanstack/react-query'
import { Toaster } from '#/components/ui/toast'
import { TooltipProvider } from '#/components/ui/tooltip'
import { NotFound } from '#/components/not-found'
import { THEME_COLOR, THEME_SCRIPT } from '#/lib/theme-script'
import appCss from '../styles.css?url'

export const Route = createRootRouteWithContext<{ queryClient: QueryClient }>()(
	{
		head: () => ({
			meta: [
				{ charSet: 'utf-8' },
				{ name: 'viewport', content: 'width=device-width, initial-scale=1' },
				{ name: 'theme-color', content: THEME_COLOR.light },
				{ name: 'robots', content: 'noindex' },
				{ title: 'Sello Sign' },
			],
			links: [{ rel: 'stylesheet', href: appCss }],
		}),
		shellComponent: RootDocument,
		notFoundComponent: NotFound,
	}
)

function RootDocument({ children }: { children: React.ReactNode }) {
	return (
		<html lang="es" suppressHydrationWarning>
			<head>
				{/* The colour mode before the first paint: no flash of light. */}
				<script dangerouslySetInnerHTML={{ __html: THEME_SCRIPT }} />
				<HeadContent />
			</head>
			<body>
				<TooltipProvider>
					<Toaster>{children}</Toaster>
				</TooltipProvider>
				<Scripts />
			</body>
		</html>
	)
}
