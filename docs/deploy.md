# El despliegue de la nube

Cómo se publica Sello Sign en la nube de Sello: el panel en Cloudflare Workers,
el API en el VPS con Dokploy (un contenedor) y una sola base en Neon. Una
instalación propia usa las mismas piezas como quiera (el panel también corre
en Node: `bun run build` y `bun run start`).

| Pieza | Dónde                                  | Qué se publica                                                                             |
| ----- | -------------------------------------- | ------------------------------------------------------------------------------------------ |
| Base  | Neon, base `sign`                      | `public`, del API (sus migraciones, al arrancar); `auth`, del panel (`bun run db:migrate`) |
| API   | Dokploy, `api.sign.sello.tax`          | `apps/api/compose.dokploy.yml`: el API (`apps/api/Containerfile`) y su Valkey              |
| Panel | Cloudflare Workers, nombre por decidir | `apps/panel` con `bun run deploy` (`wrangler.jsonc`)                                       |

## Lo que decide el dueño antes

- **La dirección del panel.** O reemplaza al Worker viejo, `sello-sign`, en
  `sign.sello.tax`, o empieza en un nombre nuevo (por ejemplo,
  `panel.sign.sello.tax`). Se escribe en tres lugares que cambian juntos, al
  principio de `apps/panel/wrangler.jsonc`: `name`, `routes` y
  `vars.BETTER_AUTH_URL`; y, fuera de él, en `PANEL_URL` del API y en el
  cliente del IdP. Mientras `BETTER_AUTH_URL` está vacío, el Worker responde
  `500`.
- **Lo que deja de existir si reemplaza a `sign.sello.tax`:** el Worker viejo
  sirve también su API, `https://sign.sello.tax/api`, con la que firma hoy la
  plataforma de Sello. Mientras la plataforma no firme con el API nuevo
  (`-sign https://api.sign.sello.tax`, con las claves del espacio de la
  plataforma, más abajo), mover el dominio la deja sin firmar. Los datos de
  `../sign-old` no se copian (`docs/rebuild.md`, «Fase 2»).

## Las variables

Las dos piezas comparten dos valores, que tienen que ser iguales byte a byte
(sin barra final): el origen del panel (`BETTER_AUTH_URL` del panel =
`PANEL_URL` del API: el `iss` de los tokens y de dónde el API baja el JWKS,
`<PANEL_URL>/api/auth/jwks`) y el del API (`API_URL` en los dos: el `aud` de
los tokens y adonde el panel pasa `/api/sign/*`). Si no coinciden, cada
llamada del panel al API responde `401`.

### API (Environment del servicio en Dokploy)

| Variable                      | Obligatoria      | Para qué                                                                                                                                                                                                                        |
| ----------------------------- | ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `DATABASE_URL`                | sí               | La cadena **directa** de Neon (sin `-pooler`), con `?sslmode=require`. El API tiene su propio pool y toma _advisory locks_ de sesión (las migraciones, el reporte de uso), que el modo de transacción de PgBouncer no conserva. |
| `SIGN_SECRETS_KEY`            | sí               | 32 bytes en base64, `openssl rand -base64 32`: AES-256-GCM del material de los certificados. Se guarda también fuera del servidor (el gestor de contraseñas del equipo): **perderla es perder todos los certificados**.         |
| `SIGN_SECRETS_KEY_PREVIOUS`   | no               | La clave anterior, solo mientras se rota.                                                                                                                                                                                       |
| `PANEL_URL`                   | sí               | El origen del panel (arriba).                                                                                                                                                                                                   |
| `API_URL`                     | no               | `https://api.sign.sello.tax` por defecto en el compose.                                                                                                                                                                         |
| `SIGN_PLATFORM_WORKSPACE_ID`  | sí (con `CLOUD`) | El espacio de la plataforma de Sello (más abajo). El compose pone `CLOUD=true`.                                                                                                                                                 |
| `BILLING_URL`                 | no               | `https://billing.sello.tax` por defecto.                                                                                                                                                                                        |
| `BILLING_TOKEN`               | sí               | Sign → cobro (activar un espacio, reportar el uso cada hora). Es el valor de `BILLING_SIGN_TOKEN` del cobro de Sello.                                                                                                           |
| `BILLING_PUSH_TOKEN`          | sí               | Cobro → Sign (`PUT /api/billing/organizations/{id}`). Es el valor de `SIGN_PUSH_TOKEN` del cobro. Distinto del anterior: el API no arranca si son iguales.                                                                      |
| `SIGN_COUNTRIES`, `LOG_LEVEL` | no               | Los países que se ofrecen (`SV,DO,CR,PA,GT`) y el nivel de los logs (`info`).                                                                                                                                                   |

`REDIS_URL` (el Valkey del compose), `CLOUD=true`, `ADDR=0.0.0.0:8080` y
`LOG_FORMAT=json` los fijan el compose y la imagen. `BILLING_TOKEN` y
`BILLING_PUSH_TOKEN` son el par que ya existe en el cobro para el Sign viejo
(`BILLING_TOKEN` y `BILLING_PUSH_TOKEN` del Worker `sello-sign`): los mismos
valores sirven, o se generan nuevos (`openssl rand -hex 32`, uno por sentido) y
se cambian a la vez en el cobro.

### Panel (Worker)

En `vars` de `apps/panel/wrangler.jsonc`, versionados: `BETTER_AUTH_URL`,
`API_URL` (`https://api.sign.sello.tax`), `CLOUD=true`, `OIDC_ISSUER`
(`https://account.sello.tax`), `OIDC_ONLY=true`, `OIDC_PROVIDER_ID=sello`,
`OIDC_PROVIDER_NAME=Sello`, `PUBLIC_SITE_URL` (`https://sello.tax`: el único
origen al que `POST /sales-inquiry` responde con CORS), `SALES_EMAIL`
(`sales@sign.sello.tax`, adonde llegan las solicitudes del diálogo de ventas)
y `EMAIL_FROM` (`Sello Sign <noreply@sign.sello.tax>`, de un dominio
verificado en Resend). Un `wrangler deploy` reemplaza las variables del
panel de Cloudflare por estas.

Secretos, con `bunx wrangler secret put <NOMBRE>` desde `apps/panel` (después
de `bun run build:cloudflare`, que escribe la configuración que wrangler lee):

| Secreto              | Para qué                                                                                                                                                                                           |
| -------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `DATABASE_URL`       | La cadena **con `-pooler`** de Neon, con `?sslmode=require` y sin `channel_binding` (el driver `pg` no lo usa). Cada petición abre su conexión y la cierra al terminar: PgBouncer la hace barata.  |
| `BETTER_AUTH_SECRET` | `openssl rand -base64 32`: firma las cookies y cifra lo que better-auth guarda (las claves privadas del JWKS, en `auth.jwks`). Cambiarlo cierra todas las sesiones y deja inservibles esas claves. |
| `OIDC_CLIENT_ID`     | El cliente del panel en el IdP de Sello (más abajo).                                                                                                                                               |
| `OIDC_CLIENT_SECRET` | Su secreto.                                                                                                                                                                                        |
| `RESEND_API_KEY`     | La clave de Resend con la que sale el correo del diálogo de ventas. Sin ella, la solicitud se escribe en los logs del Worker y nadie la recibe por correo.                                         |

**Por qué `pg` por TCP y no Hyperdrive ni el driver de Neon.** Es el mismo
driver en local, en Node, en la migración y en el Worker (allí, por
`cloudflare:sockets`, que `pg` elige solo con `nodejs_compat`), sin otra
dependencia ni otro recurso de Cloudflare; y Neon ya agrupa las conexiones
con su `-pooler`. El driver de Neon no da lo que better-auth usa en
Postgres (un _pool_ con transacciones) salvo por WebSocket, con el mismo
costo de abrir la conexión. Lo que cuesta: cada petición que toca la base
abre una conexión TLS hasta Neon (unas pocas idas y vueltas). Si eso se nota,
Hyperdrive la mantiene abierta cerca de Neon: se crea con
`bunx wrangler hyperdrive create sello-sign-panel --connection-string=<cadena directa>`,
se declara en `wrangler.jsonc` (`"hyperdrive": [{ "binding": "HYPERDRIVE", "id": "…" }]`)
y el panel tendría que leer su `connectionString` en vez de `DATABASE_URL`
(`src/lib/auth.ts`), un cambio de pocas líneas que hoy no está hecho.

Las tablas del panel se nombran siempre con su esquema (`auth."user"`,
`schemaName` de better-auth en `src/lib/auth-options.ts`), y no con
`search_path`: PgBouncer no pasa a Postgres las opciones de arranque de una
conexión.

## El orden

### 1. Neon: la base y sus migraciones

1. En el proyecto de Neon (el de Sello u otro), una base `sign` con su rol
   (dueño de la base: crea el esquema `auth`). Antes de cada despliegue que
   migre, una rama de respaldo (`Branches › Create branch`), para volver
   atrás.
2. Las tablas del API, con la imagen que se va a publicar (así migra lo de ese
   commit, no lo de un árbol local con cambios):

   ```sh
   podman build --format docker -f apps/api/Containerfile -t sello-sign-api apps/api
   podman run --rm -e DATABASE_URL='<cadena directa>' sello-sign-api migrate
   ```

   No hace falta: el API migra al arrancar, bajo un _advisory lock_. Hacerlo
   antes solo adelanta un error.

3. Las del panel, desde `apps/panel` (la variable del shell gana sobre
   `.env`; imprime el host y la base antes de tocarla):

   ```sh
   DATABASE_URL='<cadena directa>' bun run db:migrate
   ```

### 2. El API en Dokploy

1. DNS: `api.sign.sello.tax` → la IP del VPS, sin el proxy de Cloudflare
   mientras Let's Encrypt emite el certificado (desafío HTTP).
2. Un servicio Compose desde este repositorio, ruta
   `apps/api/compose.dokploy.yml`. El dominio lo publican las etiquetas de
   Traefik del compose: no se añade en la pestaña Domains.
3. Environment: las variables de arriba. El espacio de la plataforma todavía no
   existe, así que el primer despliegue va con
   `SIGN_PLATFORM_WORKSPACE_ID=pending` (el API arranca y lo avisa en sus
   logs).
4. Deploy, y `curl -s https://api.sign.sello.tax/health` → `{"status":"ok"}`.

**El espacio de la plataforma y sus claves**, con la consola del API dentro
del contenedor (lee su mismo entorno: ningún secreto sale del servidor), en el
VPS:

```sh
C=$(docker ps -qf name=sello-sign-api)
docker exec $C /api workspace create -name "Sello" -country SV   # imprime su id
```

Ese id va en `SIGN_PLATFORM_WORKSPACE_ID` del Environment, y se vuelve a
desplegar. Después, las claves de la plataforma de Sello, que solo se muestran
una vez (van directo a su Environment):

```sh
docker exec $C /api key create -workspace <id> -environment test -name "Plataforma (pruebas)" -scopes sign
docker exec $C /api key create -workspace <id> -environment prod -name "Plataforma" -scopes sign
docker exec $C /api key create -workspace <id> -environment prod -name "Plataforma (gestión)" \
  -scopes organizations:read,organizations:write,certificates:read,certificates:write,delegations:read,delegations:write
```

Son `SIGN_API_KEY_TEST`, `SIGN_API_KEY_PROD` y `SIGN_MANAGEMENT_KEY` de la
plataforma (`cmd/api` de Sello), cuando firme con este API. Quien entre al
panel con su cuenta de Sello y tenga que administrar ese espacio se agrega con
`docker exec $C /api member add -workspace <id> -user <sub del panel> -role owner`
(el `sub` del panel es el `id` de su fila en `auth."user"`).

### 3. El panel en Workers

Con la dirección decidida en `wrangler.jsonc` (`name`, `routes` y
`BETTER_AUTH_URL`), desde `apps/panel`, con una sesión de wrangler
(`bunx wrangler login`) en la cuenta de Cloudflare de `sello.tax`:

```sh
bun run build:cloudflare
bunx wrangler secret put DATABASE_URL
bunx wrangler secret put BETTER_AUTH_SECRET
bunx wrangler secret put OIDC_CLIENT_ID       # del paso 4; antes, el panel no deja entrar
bunx wrangler secret put OIDC_CLIENT_SECRET
bunx wrangler secret put RESEND_API_KEY       # el correo del diálogo de ventas
bun run deploy                                # build:cloudflare y wrangler deploy
```

Si reemplaza a `sign.sello.tax`, antes se quita ese Custom Domain del Worker
`sello-sign` (Workers › `sello-sign` › Settings › Domains & Routes): un
dominio es de un solo Worker. Se comprueba con
`curl -s https://<panel>/api/auth/jwks` (las claves EdDSA) y entrando con la
cuenta de Sello.

### 4. El IdP: el cliente del panel

El panel es un cliente de primera parte del IdP de Sello. Se registra uno
nuevo aunque el panel reemplace a `sign.sello.tax` (el del Sign viejo vuelve a
lo de antes al volver atrás), con el script del IdP, desde
`../sello/apps/auth`:

```sh
DATABASE_URL='<la directa de la base identity>' bun scripts/oauth-client.ts \
  --name "Sello Sign" \
  --redirect https://<panel>/api/auth/callback/sello \
  --logout https://<panel>/sign-in \
  --first-party --secret-file ./sign-panel-client-secret
```

- La dirección de vuelta es `/api/auth/callback/<OIDC_PROVIDER_ID>` (better-auth);
  la de cierre de sesión, `/sign-in` (`postLogoutRedirectURI`). Sin
  `--backchannel`: el panel nuevo no tiene _back-channel logout_.
- Su `client_id` va en `TRUSTED_CLIENTS` del `wrangler.jsonc` del IdP (en lugar
  del de `sello-sign`, si lo reemplaza), y se publica el IdP. El `client_id` y
  el secreto, en los secretos del panel (paso 3).

### 5. El cobro: `-sign`

En `../sello/apps/api/compose.dokploy.yml`, `sello-billing` lleva
`-sign=https://api.sign.sello.tax` en su comando, con `BILLING_SIGN_TOKEN` y
`SIGN_PUSH_TOKEN` en su Environment (los valores de `BILLING_TOKEN` y
`BILLING_PUSH_TOKEN` del API), y se despliega. El cobro empuja ahí
`PUT /api/billing/organizations/{id}`, y el API le reporta el uso cada hora.

## Antes de mover sign.sello.tax

Lo que hay que tener listo para que el panel nuevo tome `sign.sello.tax` en
lugar del Worker viejo, `sello-sign`. El sitio de Sello
(`../sello/apps/marketing`, `src/productos/sign/`) enlaza a las direcciones
del panel viejo y le manda su diálogo de ventas: lo de su landing cambia **en
el mismo despliegue** que el dominio, ni antes (enlazaría a un panel que
todavía no está ahí) ni después (describiría un API que ya no responde).

- [ ] **Las redirecciones** (`src/lib/moved.ts`, `308`, con la búsqueda):
      `/register` → `/sign-up`, `/login` (y `/login/two-factor`) →
      `/sign-in`, `/dashboard` → `/`, `/organizations/new` →
      `/workspaces/new`, y `/settings/…`, `/issuers/…`, `/invite/…` y
      `/switch-workspace` → `/`, que abre el último espacio. `/api/openapi` →
      `<API_URL>/openapi.yaml`. `/forgot-password`, `/reset-password` y
      `/workspaces/new` conservan su dirección (los enlaces de restablecer
      del Sign viejo no sirven aquí: sus tokens eran de su base). Se
      comprueba con
      `curl -sI https://sign.sello.tax/register?plan=pro_growth`: `308` a
      `/sign-up?plan=pro_growth`, con la cookie `selected_plan`.
- [ ] **El plan elegido en el sitio** (`?plan=`, `src/lib/plan.ts`): la
      cookie dura una hora; al abrir un espacio que administra, la persona va
      a «Su cuenta › Sello Sign › Plan y facturación» de su organización
      (`<OIDC_ISSUER>/u/0/sign/plan?organization=<id>`).
- [ ] **El diálogo de ventas** (`POST /sales-inquiry`): `PUBLIC_SITE_URL`,
      `SALES_EMAIL` y `EMAIL_FROM` en `vars` y `RESEND_API_KEY` como secreto
      (paso 3). Se comprueba enviándolo desde `https://sello.tax/sign`.
- [ ] **Los textos del landing que describen el API viejo**, que cambian en
      el landing de Sello con el dominio:
  - el ejemplo de `PETICION_FIRMA` en `sign.ts`, `Hero.astro` y los dos
    `i18n` (`es.ts` y `en.ts`: el paso «Haga POST a /api/signatures» y la
    pregunta del cliente HTTP) dicen `POST /api/signatures` con
    `X-Issuer-Id` y una clave `prod_…`: el API nuevo firma con
    `POST https://api.sign.sello.tax/signatures`, la organización en
    `X-Organization-Id` y la clave `sign_prod_…` en `X-API-Key`
    (`contracts/README.md`);
  - `panelSign.openapi` (`/api/openapi`) sigue llegando por la redirección,
    pero el enlace debería ir directo a
    `https://api.sign.sello.tax/openapi.yaml`;
  - «puede probar los endpoints desde el navegador» (`i18n`, «you can try
    the endpoints from the browser»): el API nuevo no tiene esa referencia
    interactiva; se quita;
  - «Errores RFC 7807» (`feat2` en los dos `i18n` y la misma pregunta):
    el API nuevo responde según RFC 9457, que la reemplaza;
  - `panelSign.registro`, `.sesion` y `.dashboard` siguen llegando por la
    redirección; cambiarlos a `/sign-up`, `/sign-in` y `/` ahorra un salto.
- [ ] **El cliente del IdP** (paso 4) con la dirección de cierre de sesión
      nueva, `https://sign.sello.tax/sign-in` (el Sign viejo volvía a
      `/login`), y la de vuelta `https://sign.sello.tax/api/auth/callback/sello`;
      su `client_id` en `TRUSTED_CLIENTS` del IdP.
- [ ] **El cobro**: en `../sello/apps/api/compose.dokploy.yml`,
      `sello-billing` lleva `-sign=https://api.sign.sello.tax` todavía
      comentado; se activa (paso 5), con `BILLING_SIGN_TOKEN` y
      `SIGN_PUSH_TOKEN`, antes de vender planes en el panel nuevo: sin él, el
      enlace del plan elegido lleva a «Plan y facturación», pero el cobro no
      sabe de Sign.

## Volver atrás

- **Panel:** `bunx wrangler rollback` desde `apps/panel` vuelve a la versión
  anterior. Si había reemplazado a `sign.sello.tax`: se quitan sus `routes` (o
  el Custom Domain en el panel de Cloudflare) y se devuelve el dominio a
  `sello-sign`, que sigue publicado tal como estaba; en el IdP, se restituye
  `TRUSTED_CLIENTS`.
- **API:** en Dokploy, desplegar de nuevo el commit anterior. Las migraciones
  no se deshacen: si una dañó datos, se restaura la rama de respaldo de Neon
  (paso 1). Las tablas de `public` y `auth` son de este Sign y nadie más las
  lee: borrar la base entera es volver al principio.
- **Cobro:** quitar `-sign` de su comando lo deja sin hablar con Sign.

## En local, como en Workers

`bun run preview:cloudflare` (en `apps/panel`, después de
`bun run build:cloudflare`) corre el build en workerd con `wrangler dev`, en
`127.0.0.1:4442`, con `.dev.vars` (copia de `.dev.vars.example`) sobre los
`vars`. Contra el Postgres de `compose.yml` y un API cuyo `PANEL_URL` sea
`http://127.0.0.1:4442`. El contenedor del API, igual:

```sh
podman build --format docker -f apps/api/Containerfile -t sello-sign-api apps/api
podman run --rm -p 127.0.0.1:4541:8080 \
  -e DATABASE_URL=postgres://sign:sign@host.containers.internal:5434/sign \
  -e SIGN_SECRETS_KEY="$(openssl rand -base64 32)" \
  -e API_URL=http://127.0.0.1:4541 sello-sign-api
```

(`--format docker`: Podman guarda el `HEALTHCHECK` solo en ese formato.)
