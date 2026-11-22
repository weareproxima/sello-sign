# Instrucciones para agentes

Sello Sign, el firmador: firma documentos tributarios electrónicos (JWS en El
Salvador; XMLDSig o XAdES en los demás países) para proveedores de facturación
electrónica. Open source (AGPL-3.0), con su nube en `sign.sello.tax`. Este
repositorio es su **reconstrucción** (decidida el 2026-11-17 y el 2026-11-19):
el código anterior, SvelteKit con el API en Elysia, todo en TS, está en
`../sign-old`, que se lee pero no se cambia. Sin compatibilidad con nada suyo:
ni datos, ni rutas, ni el contrato. El diseño está en `../sello/docs/sello-sign.md`;
el plan y su estado, en [`docs/rebuild.md`](docs/rebuild.md).

`CLAUDE.md` es un enlace simbólico a este archivo: se edita aquí.

## Mapa

- `apps/api`: el API de firma, en Go (módulo `proxima.la/sello/sign`, Go 1.26),
  en `127.0.0.1:4441` (`api.sign.sello.test`). Solo la biblioteca estándar
  para HTTP (`net/http` con sus patrones), `pgx` para Postgres, un cliente de
  Valkey y, para el servidor MCP, el SDK oficial de Go
  (`github.com/modelcontextprotocol/go-sdk`, el de Sello Chat).
  - `internal/signing`: lo común de los firmadores (`Country`, `Material`,
    `Registry`, la lectura de certificados), sin ningún país.
    `internal/signing/c14n`, la canonicalización propia;
    `internal/signing/xmldsig`, el único DOM compatible con xmldom, la firma
    XMLDSig y su verificación; `internal/signing/xades`, lo de XAdES encima de
    él; `internal/signing/countries/<cc>`, un paquete por país, y
    `countries.go`, el registro con todos; `testdata/vectors/<cc>`, las firmas
    de referencia de `../sign-old`.
  - `internal/config` (el entorno), `internal/model` (roles, scopes,
    ambientes, el alcance de un espacio sobre una organización),
    `internal/auth` (las claves y el token del panel), `internal/secrets`
    (AES-256-GCM), `internal/store` (Postgres y sus migraciones, que se
    aplican al arrancar), `internal/cache` (las claves en Valkey; el material
    descifrado, solo en memoria), `internal/billing` (la nube),
    `internal/ratelimit` (las firmas por minuto), `internal/retention` (lo
    vencido del registro de firmas y de la bitácora de auditoría),
    `internal/audit` (el vocabulario de la bitácora: el actor, las acciones,
    sus 365 días; cada evento lo escribe `internal/store` en la transacción
    de su cambio), `internal/notices` (los avisos por
    correo de cada hora: certificados por vencer y el uso del plan),
    `internal/mail` (el correo: Resend o el log) e `internal/httpapi` (las rutas, con una copia de `contracts/openapi.yaml`
    que refresca `go generate ./internal/httpapi`, y una prueba que compara
    las rutas con el contrato; y el servidor MCP, `POST /mcp`, en `mcp.go`:
    cada herramienta llama en proceso a su ruta con la misma clave, así que
    una ruta que cambia cambia también su herramienta).
  - `cmd/api`: el servidor y, sin panel, una consola (`migrate`,
    `workspace create`, `workspace limits`, `member add`, `key create`).
- `apps/panel`: el panel, TanStack Start (React, TanStack Router y Query,
  Tailwind y shadcn), en `127.0.0.1:4440` (`sign.sello.test`). Su servidor
  lleva better-auth: las cuentas, las sesiones y el token con el que el panel
  llama al API.
- `contracts/`: el contrato del API ([`README.md`](contracts/README.md) y
  `openapi.yaml`).
- [`docs/deploy.md`](docs/deploy.md): el despliegue de la nube (el panel en Workers, el API en Dokploy, Neon), sus variables y su orden.
- `tools/vectors`: genera las firmas de referencia con los firmadores de TS de
  `../sign-old`, con la hora y el azar fijos
  (`cd tools/vectors && bun run gen.ts [países]`; dos corridas dan los mismos
  bytes). Ver su `README.md`.
- `packages/`: lo que compartan las superficies TS.

## Cómo se levanta

- `podman compose up -d`: Postgres en `127.0.0.1:5434` (base, usuario y
  contraseña `sign`) y Valkey en `127.0.0.1:6381`.
- `bun install` en la raíz, `bun run db:migrate` en `apps/panel` (las tablas de
  better-auth y la de los intentos de contraseña) y `bun run dev` (turbo): el panel y el API. El API pide su
  `.env` (copia de `apps/api/.env.example`, con `SIGN_SECRETS_KEY` de
  `openssl rand -base64 32`), y el panel, el suyo.
- `bun run typecheck`, `bun run test` y `bun run format` (Prettier y gofmt).
- En `apps/api`: `go vet ./...` y `go test ./...`; contra Postgres,
  `bun run test:pg`. El recorrido con `curl`, en `docs/rebuild.md` («API»).

Una sola base, `sign`: las tablas del dominio son del API (sus migraciones SQL,
en Go) y las de better-auth, del panel, en el esquema `auth`.

## Variables

- API: `DATABASE_URL`, `REDIS_URL`, `SIGN_SECRETS_KEY` (32 bytes en base64:
  AES-256-GCM para el material de los certificados), `PANEL_URL` (el `iss` del
  token y su JWKS), `API_URL` (su `aud`), `SIGN_COUNTRIES` (los países que se
  ofrecen; por defecto `SV,DO,CR,PA,GT`, los que ya se probaron), `CLOUD`
  (`true` en `sign.sello.tax`), `SIGN_RATE_LIMIT_PER_MINUTE` y
  `SIGN_LOG_RETENTION_DAYS` (los límites de los espacios sin plan; vacías,
  ninguno), `RESEND_API_KEY` y `EMAIL_FROM` (los avisos; sin la clave, van
  al log) y, en la nube, `SIGN_PLATFORM_WORKSPACE_ID` (el
  espacio de Sello), `BILLING_URL`, `BILLING_TOKEN` y `BILLING_PUSH_TOKEN`.
- Panel: `DATABASE_URL`, `BETTER_AUTH_SECRET`, `BETTER_AUTH_URL` (= `PANEL_URL`),
  `API_URL`, `CLOUD` y, para entrar con un proveedor OIDC externo (en la nube,
  el IdP de Sello), `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET` y
  `OIDC_ONLY`; para el diálogo de ventas del sitio de Sello
  (`POST /sales-inquiry`), `PUBLIC_SITE_URL` (el único origen con CORS),
  `RESEND_API_KEY` (sin ella, el correo va a la consola), `SALES_EMAIL` y
  `EMAIL_FROM`.

## Reglas

- La documentación y la interfaz, en español (de usted, neutro, como Sello);
  el código, sus comentarios y sus identificadores, en inglés. Las rutas, en
  inglés.
- El vocabulario: un **espacio** (`workspace`) es el cliente de Sign y guarda
  **organizaciones** (`organization`: los contribuyentes, antes «emisores»);
  una **delegación** (`delegation`) deja a otro espacio firmar con una
  organización que no es suya.
- La firma XML es byte a byte: un firmador de Go reproduce el de TS de
  `../sign-old` salvo en sus errores conocidos (`docs/rebuild.md`), y cada
  firma se verifica también con un verificador independiente en las pruebas.
- Las claves privadas nunca se escriben en un log ni salen del proceso sin
  cifrar.
- El sistema de diseño viene del registro público de shadcn de Sello
  (`@sello`, `https://sello.tax/r/{name}.json`, en `components.json` del
  panel): las piezas de `src/components/ui/`, `src/lib/utils.ts`,
  `src/lib/button-variants.ts`, `src/lib/shortcuts.ts` y los tokens de
  `src/styles/sello/` no se
  editan en Sign. Cambian en `packages/ui` de `../sello` y se reinstalan con
  `bunx shadcn@latest add @sello/<nombre> --overwrite` desde `apps/panel`
  (`--diff` enseña lo que se apartó del registro). Lo propio del panel, como
  `src/components/empty.tsx`, va fuera de `components/ui/`.
- No se hacen commits sin pedirlo.
