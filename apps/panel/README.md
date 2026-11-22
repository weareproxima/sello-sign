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
bun run db:migrate            # better-auth y password_attempt, en el esquema `auth`
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

## El correo

Todo lo que el panel envía (el restablecimiento de la contraseña, el enlace
para cambiar el correo y el aviso al correo anterior, las invitaciones y
«Hablar con ventas») sale por `src/lib/email.ts`: Resend con
`RESEND_API_KEY`, desde `EMAIL_FROM` (en la nube, una dirección de
`@sign.sello.tax`, ya verificada en Resend). Sin la clave, el correo se
escribe en la consola del servidor, con su enlace. La plantilla de los
correos de la cuenta está en `src/lib/account-email.ts`.

## Comprobaciones

`bun run typecheck`, `bun run test` (vitest; con
`POSTGRES_TEST_URL=postgres://sign:sign@127.0.0.1:5434/sign`, también la
tabla de los intentos de contraseña), `bun run lint:design` y
`bun run build`. `lint:design` (`scripts/lint-design.ts`) aplica las reglas de
`../sello/DESIGN.md` que se leen en el código (ni tamaños, colores, radios,
sombras, duraciones ni opacidades fuera de los tokens, ni alias de shadcn, ni
colores con `dark:`) al código propio del panel; lo del registro `@sello` no
se revisa aquí. Una excepción lleva `ds-allow: <por qué>`.

## En Cloudflare Workers

`bun run build:cloudflare` construye para Workers (`wrangler.jsonc`),
`bun run preview:cloudflare` lo corre en workerd en `127.0.0.1:4442` (con
`.dev.vars`, copia de `.dev.vars.example`) y `bun run deploy` lo publica (en
la nube de Sello, lo hace Workers Builds con cada push a `main`). Ahí
cada petición abre su propia conexión a la base (`src/lib/database.ts`): un
socket de una petición no sirve a otra. Las variables, el orden y cómo volver
atrás, en [`docs/deploy.md`](../../docs/deploy.md).

## Dónde está cada cosa

- `src/lib/config.ts`: las variables, sin secretos para las páginas (que
  sí reciben `API_URL`: el origen público del API, donde está su servidor
  MCP).
- `src/lib/auth-options.ts` y `src/lib/auth.ts`: better-auth; `scripts/migrate.ts`
  (sus tablas y la del panel, `auth.password_attempt`).
- `src/lib/password-attempts.ts`: el límite de intentos con la contraseña
  donde la comprueba el servidor del panel (cambiar el correo, borrar la
  cuenta): 5 fallos por cuenta en 15 minutos, contados en
  `auth.password_attempt` (no en memoria: en Workers cada petición puede caer
  en otro _isolate_). Su prueba usa Postgres con `POSTGRES_TEST_URL`.
- `src/lib/audit.ts`: «Actividad» (`_app.w.$workspaceId.activity.tsx`), la
  bitácora de auditoría del espacio en español; su prueba compara las
  acciones con las de `apps/api/internal/audit/audit.go`.
- `src/lib/api.ts`: el cliente del contrato (`contracts/`), con los errores
  dichos con los moldes de Sello.
- `src/lib/limits.ts`: los límites del plan como los aplica el API (el tope
  del mes, las firmas por minuto) y lo que dice el panel cuando uno detiene
  una firma (`429`); `src/components/limit-alert.tsx`, el error con
  «Cambiar el plan» en la nube.
- `src/lib/mcp.ts`: cómo conectar un agente al servidor MCP del API («Claves
  de API»); su prueba compara las herramientas con las de
  `apps/api/internal/httpapi/mcp.go`.
- `src/styles.css`: importa Tailwind y el tema de Sello, `src/styles/sello/`
  (los tokens, las piezas de CSS y su conexión con Tailwind).
- `src/components/ui/`, `src/lib/utils.ts`, `src/lib/button-variants.ts` y
  `src/lib/shortcuts.ts`: las piezas de Sello. Las dos cosas se instalan desde su registro público de
  shadcn (`@sello` en `components.json`) y no se editan aquí: se reinstalan
  con `bunx shadcn@latest add @sello/<nombre> --overwrite` (ver «Reglas» en
  `AGENTS.md`).
- `src/components/page-shortcuts.tsx`, en la raíz, y
  `src/components/shell-shortcuts.tsx`, en `AppShell`: el teclado que el panel
  comparte con el chat, la web y el centro de cuenta de Sello. En cualquier
  página, la D cambia el modo de color (`toggleTheme` de
  `src/lib/use-theme.ts`, el mismo modo que elige el menú de la cuenta) y, al
  mantener ⌘, se ven las teclas de los controles a la vista que las declaran
  con `data-hint` (`ShortcutHints`, del registro). En las páginas de un
  espacio, como en el primer Sello Sign: G y una letra van a cada página de
  la columna (`PAGE_KEYS`: G D Resumen, G O Organizaciones, G F Registro de
  firmas, G V Verificar una firma, G K Claves de API, G M Miembros, G H
  Actividad, G S General) y G A a «Su cuenta»; N, a una organización nueva,
  para quien la puede crear; W y un número, a otro espacio, en el orden del
  selector; y ⌘/ o `?` abre la lista de todos (`KeySequences` y
  `ShortcutsDialog`, del registro). Cada fila de la columna enseña su
  secuencia al mantener ⌘; `aria-keyshortcuts` no sabe decir secuencias.
  Una página nueva de la columna entra en `PAGE_KEYS`, que su prueba revisa.
  Encima de la cuenta, «Ayuda» (`HelpMenu` de `app-shell.tsx`; «El menú de
  ayuda» de `../sello/DESIGN.md`): «Especificación del API» (el
  `openapi.yaml` del API, en otra pestaña) y «Atajos de teclado», la misma
  lista que ⌘/; «Preguntar a Sello AI», el centro de ayuda y el contacto con
  soporte esperan como `TODO(ayuda)`.
- `src/routes/`: `_auth.*` (entrar), `_app.*` (con sesión: también «Su
  cuenta», `_app.account.tsx`, en `/account`, fuera de cualquier espacio),
  `_app.w.$workspaceId.*` (un espacio, también «Verificar una firma»,
  `verify`). `/w/<id>/account`, donde estaba «Su cuenta» hasta el
  2026-11-20, responde `308` a `/account` (`src/lib/moved.ts`).
- `src/server/`: lo que hace el servidor del panel con la sesión de la
  persona: invitar y reenviar una invitación con su correo
  (`invitations.ts`), y en una instalación propia, borrar la cuenta
  (`account.ts`: primero `DELETE /me` en el API, después better-auth) y
  cambiar su correo (`requestEmailChange`, con la contraseña: better-auth
  manda el enlace al correo nuevo y se avisa al anterior;
  `src/lib/account-email-change.ts`). `/change-email`,
  `/send-verification-email` y `/delete-user` de better-auth están apagados
  por HTTP, para que nada pase por encima de estas funciones.
- `src/components/delete-workspace.tsx`: «Borrar el espacio», al final de
  «General» (`DELETE /workspaces/{w}`, con el nombre del espacio escrito).
- `src/lib/verify-client.ts`: el verificador de firmas que corre en el
  navegador, copia exacta del de `../sign-old` (y del de `sello.tax/sign/verify`):
  no se edita aquí ni pasa por Prettier.
- `src/styles/cards.css` y `src/components/table-cards.tsx`: en el móvil, una
  tabla larga como tarjetas; cada celda dice su columna en `data-label`, la
  del título va sin él y la de las acciones con `data-actions`.
