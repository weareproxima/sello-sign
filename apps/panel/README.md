# El panel de Sello Sign

TanStack Start (React, TanStack Router y Query, Tailwind v4 y las piezas de
base-ui del sistema de diseño de Sello) con Nitro, en `127.0.0.1:4440`
(`sign.sello.test`). Su servidor lleva better-auth: las cuentas, las sesiones y
el token con el que el panel llama al API. El estado y lo que falta, en
«Panel» de [`docs/rebuild.md`](../../docs/rebuild.md).

## Levantarlo

```bash
podman compose up -d          # en la raíz: Postgres en 127.0.0.1:5434
cp .env.example .env          # y BETTER_AUTH_SECRET (openssl rand -base64 32)
bun run db:migrate            # las tablas de better-auth, en el esquema `auth`
bun run dev                   # http://127.0.0.1:4440
```

El API (`apps/api`) tiene que tener `PANEL_URL` igual a `BETTER_AUTH_URL` y
`API_URL` igual al de aquí: son el `iss` y el `aud` del token.

## Cómo llega el token al API

1. better-auth firma, con su plugin `jwt`, un token EdDSA (Ed25519) de 15
   minutos: `iss` = `BETTER_AUTH_URL`, `aud` = `API_URL`, y `sub`, `email` y
   `name`; con la cuenta de Sello, también `sello_sub` y `sello_workspaces`.
   Sus claves públicas, en `/api/auth/jwks`.
2. El navegador lo pide a `/api/auth/token` con su sesión y lo guarda hasta un
   minuto antes de que venza (`src/lib/api.ts`, `tokenCache`).
3. Cada llamada va a `/api/sign/*` con `Authorization: Bearer <token>`, y el
   servidor del panel la pasa tal cual a `API_URL`
   (`src/routes/api/sign/$.ts`): así el API no tiene que responder CORS al
   panel. No añade credenciales.

## Comprobaciones

`bun run typecheck`, `bun run test` (vitest) y `bun run build`.

## En Cloudflare Workers

`bun run build:cloudflare` construye para Workers (`wrangler.jsonc`),
`bun run preview:cloudflare` lo corre en workerd en `127.0.0.1:4442` (con
`.dev.vars`, copia de `.dev.vars.example`) y `bun run deploy` lo publica. Ahí
cada petición abre su propia conexión a la base (`src/lib/database.ts`): un
socket de una petición no sirve a otra. Las variables, el orden y cómo volver
atrás, en [`docs/deploy.md`](../../docs/deploy.md).

## Dónde está cada cosa

- `src/lib/config.ts`: las variables, sin secretos para las páginas.
- `src/lib/auth-options.ts` y `src/lib/auth.ts`: better-auth; `scripts/migrate.ts`.
- `src/lib/api.ts`: el cliente del contrato (`contracts/`), con los errores
  dichos con los moldes de Sello.
- `src/styles.css`: importa Tailwind y el tema de Sello, `src/styles/sello/`
  (los tokens, las piezas de CSS y su conexión con Tailwind).
- `src/components/ui/`, `src/lib/utils.ts` y `src/lib/button-variants.ts`: las
  piezas de Sello. Las dos cosas se instalan desde su registro público de
  shadcn (`@sello` en `components.json`) y no se editan aquí: se reinstalan
  con `bunx shadcn@latest add @sello/<nombre> --overwrite` (ver «Reglas» en
  `AGENTS.md`).
- `src/routes/`: `_auth.*` (entrar), `_app.*` (con sesión), `_app.w.$workspaceId.*`
  (un espacio).
