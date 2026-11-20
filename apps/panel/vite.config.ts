import { defineConfig, loadEnv } from 'vite'
import { tanstackStart } from '@tanstack/react-start/plugin/vite'
import viteReact from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { nitro } from 'nitro/vite'

export default defineConfig(({ mode }) => {
	// The server reads its configuration from `process.env` (`lib/config.ts`);
	// in development it comes from `.env`, without overriding the shell.
	for (const [key, value] of Object.entries(loadEnv(mode, process.cwd(), ''))) {
		process.env[key] ??= value
	}
	return {
		resolve: { tsconfigPaths: true },
		server: { port: 4440, host: '127.0.0.1', strictPort: true },
		plugins: [nitro(), tailwindcss(), tanstackStart(), viteReact()],
	}
})
