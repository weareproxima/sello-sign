# El contrato del API de Sello Sign

Lo comparten el API (`apps/api`), el panel (`apps/panel`) y quien integre Sign.
Esta es su forma; `contracts/openapi.yaml` es el detalle, y lo escribe el API
con esta misma forma. Un cambio se hace en los dos a la vez. Sin compatibilidad
con el API de `../sign-old` (decidido el 2026-11-19), salvo donde se dice.

## Base

- Origen propio: `api.sign.sello.tax`; en local, `http://127.0.0.1:4441`
  (nginx manda ahí `api.sign.sello.test`). Las rutas van sin prefijo, salvo las
  que llama billing (abajo).
- JSON en `snake_case`. Los ids son UUIDv7 en texto.
- Errores: `application/problem+json` (RFC 9457) con `type`, `title`,
  `status` y `detail`; `type` es un slug estable
  (`organization-not-found`, `delegation-locked`…; la lista, en
  `openapi.yaml`). La única excepción es el `404` del empuje de billing, cuyo
  `type` es la URI que billing ya reconoce (abajo).
- Toda lista responde `{ "items": [...], "next_cursor": "…" | null }`. Las que
  crecen (organizaciones, claves y registro de firmas) se paginan con
  `?limit=` (1 a 100, 50 por defecto) y `?cursor=`, de la más nueva a la más
  antigua; las demás (miembros, certificados, delegaciones, las organizaciones
  del IdP…) van enteras, con `next_cursor: null`.

## Credenciales

1. **Clave de API**, en `X-API-Key`: `sign_test_…` o `sign_prod_…`. Es de un
   espacio, opcionalmente de una sola organización, con scopes, y su ambiente
   es el de su prefijo. Se guarda solo su SHA-256.
2. **Token del panel**, en `Authorization: Bearer <JWT>`: lo firma better-auth
   del panel (su plugin `jwt`) con EdDSA (Ed25519). El API lo verifica con el
   JWKS de `<PANEL_URL>/api/auth/jwks`, con `iss` = `PANEL_URL` y `aud` =
   `API_URL`, y dura 15 minutos. Sus claims: `sub` (el usuario del panel),
   `email` y `name`; en la nube, además, `sello_sub` (el `sub` de la cuenta
   Sello enlazada) y `sello_workspaces`, las organizaciones del IdP de la
   persona (`[{ "id", "name", "country", "role" }]`), tal como llegan en el ID
   token del IdP, en el claim `https://sello.tax/workspaces`.

Con las dos cabeceras, vale la clave.

**Scopes de una clave:** `sign`, `organizations:read`, `organizations:write`,
`certificates:read`, `certificates:write`, `delegations:read`,
`delegations:write`, `keys:read`, `keys:write` y `logs:read`. Una persona tiene
lo que su rol en el espacio le deja: `owner` y `admin`, todo; `member`, leer y
firmar de prueba desde el panel.

**`self`:** `/workspaces/self/…` es el espacio de la clave.

## Firmar y verificar

- `POST /signatures` (clave con `sign`). Cabecera `X-Organization-Id`: el id
  de la organización o su NIT (sin importar guiones, espacios ni puntos; si
  nombra a más de una, `409 organization-ambiguous`) entre las que alcanza la clave (las de su espacio
  y las que su espacio tiene delegadas; una clave de una sola organización,
  solo esa). Cuerpo: el documento tal cual (`application/json` en El Salvador,
  `application/xml` en los demás países). Firma con el certificado activo del
  ambiente de la clave. Respuesta `200`:
  `{ "signature": "…", "document_id": "…", "document_type": "…" }`.
- `POST /verifications` (clave con `sign`), con `X-Organization-Id`:
  `{ "signature": "…" }` → `{ "valid", "document", "document_id", "document_type" }`,
  con `document` el documento firmado en texto (el JSON de un JWS o el XML).
- `POST /workspaces/{w}/organizations/{o}/signatures?environment=test|prod`
  (token del panel): lo mismo, para firmar de prueba desde el panel.

Lo que se firma con una delegación cuenta en el espacio de la clave que firma,
no en el del dueño de la organización.

## Espacios

- `GET /me` (token) → `{ "user": { "id", "email", "name" }, "workspaces": [{ "id", "name", "role", "plan", "default_country", … }] }`:
  cada espacio como lo da `GET /workspaces/{w}` (también `sello_workspace_id`,
  `closed`, `limits` y `created_at`).
- `POST /workspaces` (token). En una instalación propia, `{ "name", "default_country" }`,
  y quien lo crea es su `owner`. En la nube, `{ "sello_workspace_id" }`: abre el
  espacio de esa organización del IdP (solo quien la administra), como
  `openSpace` de `../sign-old/src/lib/server/billing.ts`.
- `GET /me/sello-organizations` (token, nube): las organizaciones del IdP de la
  persona, cada una con su estado: `open` (tiene espacio y la persona es
  miembro), `available` (puede abrirlo), `ask` (es `member`: lo abre quien la
  administra), `full` o `closed` (como `organizationOptions` de sign-old),
  con `workspace_id` (si es `open`) y `max_members` (si es `full`).
- `GET /workspaces/{w}` y `PATCH /workspaces/{w}` (`{ "name", "default_country" }`).
- `GET /workspaces/{w}/members` → `[{ "user_id", "email", "name", "role" }]`.
- `GET /workspaces/{w}/usage?from=AAAA-MM&to=AAAA-MM` →
  `{ "plan", "limits", "items": [{ "month", "environment", "signatures" }] }`:
  lo que firmó el espacio, por mes y ambiente (por defecto, los últimos doce
  meses), y los límites del plan.

## Organizaciones (los contribuyentes)

- `GET /workspaces/{w}/organizations?relation=owned|delegated` →
  `[{ "id", "workspace_id", "name", "tax_id", "country", "contact_email", "relation", "sello_workspace_id", "delegation", "certificates": { "test": {…} | null, "prod": {…} | null }, "created_at" }]`,
  con `workspace_id` el espacio dueño y `delegation`
  (`{ "id", "scopes", "locked" }`) la que da el alcance a una delegada.
- `POST /workspaces/{w}/organizations`
  `{ "name", "tax_id", "country", "contact_email"?, "sello_workspace_id"? }`.
  Idempotente por espacio, país y NIT (sin guiones, espacios ni puntos): si
  ya existe, `200` con la que hay; si no, `201`.
  `sello_workspace_id` (el vínculo con la organización del IdP que es ese
  contribuyente) solo lo pone el espacio de la plataforma
  (`SIGN_PLATFORM_WORKSPACE_ID`) o una persona que administra esa organización
  del IdP; es único (`409 sello-workspace-linked`). La excepción es el orden
  inverso: si el espacio de la plataforma nombra uno que ya vincula una
  organización del espacio de una empresa (la empresa contrató Sign antes de
  que la plataforma la registrara), responde `200` con esa organización
  (`relation: delegated`) y se da, si no la tiene, la delegación `locked`
  (`sign`, `certificates:read` y `certificates:write`); nunca crea una
  segunda; si esa organización tiene otro NIT u otro país que los pedidos,
  `409 organization-mismatch`. Desde cualquier otro espacio sigue siendo `409`.
- `GET`, `PATCH` (`{ "name", "contact_email", "sello_workspace_id" }`; el
  país y el NIT no cambian) y `DELETE /workspaces/{w}/organizations/{o}`. Solo
  el dueño cambia o borra; borrar con delegaciones vivas es
  `409 organization-has-delegations`.
- **Certificados:**
  - `GET /workspaces/{w}/organizations/{o}/certificates`.
  - `POST …/certificates`
    `{ "environment", "file" (base64), "password"?, "activate" (true por defecto) }`
    → `{ "id", "environment", "tax_id", "subject", "issuer", "serial", "not_before", "not_after", "active", "created_at" }`.
    `422` si el país no lee el archivo o su NIT no es el de la organización.
  - `PATCH …/certificates/{c}` `{ "active" }`: activar uno desactiva el otro
    activo del mismo ambiente.
  - `DELETE …/certificates/{c}`.
- **Delegaciones** (solo el dueño las da y las quita):
  - `GET …/delegations` → `[{ "id", "workspace_id", "workspace_name", "scopes", "locked", "created_at" }]`.
  - `POST …/delegations` `{ "workspace_id", "scopes" }`, con scopes de
    `sign`, `certificates:read` y `certificates:write`.
  - `DELETE …/delegations/{d}`: `409 delegation-locked` para la que conservó
    el espacio de la plataforma al traer la organización. El espacio delegado
    renuncia a la suya, también a una bloqueada, con la misma ruta bajo su
    propio espacio (`/workspaces/{delegado}/organizations/{o}/delegations/{d}`):
    así la quita la plataforma cuando la organización deja de emitir con ella.
- **«Usar en Sello Sign»** (token, nube):
  - `GET /workspaces/{w}/claimable-organizations`: las del espacio de la
    plataforma vinculadas a organizaciones del IdP que la persona administra.
  - `POST /workspaces/{w}/organization-claims` `{ "organization_ids": [...] }`:
    las pasa al espacio `w` con sus certificados y su historial, y deja una
    delegación `locked` al espacio de la plataforma, en la misma transacción.

## Claves de API

- `GET /workspaces/{w}/api-keys`.
- `POST /workspaces/{w}/api-keys`
  `{ "name", "environment", "scopes", "organization_id"? }` → la clave con
  `"key"`, que se muestra una sola vez. Una clave que crea otra no le da más
  de lo que tiene: ni otros scopes, ni otro ambiente, ni otra organización.
- `DELETE /workspaces/{w}/api-keys/{k}`: la revoca; deja de valer al momento.

## Registro de firmas

- `GET /workspaces/{w}/signatures?organization_id=&environment=&from=&to=&limit=&cursor=`
  → `{ "items": [{ "id", "organization_id", "organization_name", "environment", "document_id", "document_type", "api_key_id", "user_id", "signed_by_workspace_id", "duration_ms", "created_at" }], "next_cursor" }`.
  El dueño de una organización ve también lo que firmó con ella otro espacio
  por delegación (`signed_by_workspace_id`, sin su `api_key_id` ni su
  `user_id`); `user_id` es quien firmó desde el panel, y `organization_name`
  es `null` si la organización se borró.
- `GET /workspaces/{w}/signatures/{s}`.

## Billing (solo en la nube)

Las rutas y los cuerpos que ya usan `cmd/billing` de Sello y `../sign-old`, para
que Sello solo cambie el origen:

- `PUT /api/billing/organizations/{sello_workspace_id}`, con
  `Authorization: Bearer <BILLING_PUSH_TOKEN>`: el estado de un espacio
  (versión, plan, límites, nombre, país y miembros), igual que `applyState` de
  `../sign-old/src/lib/server/billing.ts`. `204` aplicado (o con una versión
  que no es mayor que la aplicada); `404` con `type`
  `https://sello.tax/problems/sign-unknown-organization` si Sign no tiene un
  espacio de esa organización. Un estado cerrado (`closed: true`) deja de
  firmar en el espacio y, en la misma transacción, devuelve al espacio de la
  plataforma cada organización suya de la que la plataforma tiene la
  delegación `locked`: la plataforma vuelve a ser su dueña, con sus
  certificados y su historial; se quitan esa delegación y las que el espacio
  que cierra había dado de ella a otros espacios, y se revocan las claves de
  ese espacio que eran solo de ella. Las demás se quedan en el espacio
  cerrado, y reabrirlo no las trae de vuelta: la empresa las vuelve a traer
  («Usar en Sello Sign»). Empujar otra vez un estado cerrado no cambia nada.
- Sign llama a billing (`BILLING_URL`, con `BILLING_TOKEN`): activar un espacio
  (`PUT /billing/sign/workspaces/{id}`) y reportar el uso cada hora
  (`POST /billing/sign/usage`).

## Salud

- `GET /health` → `{ "status": "ok" }`.
- `GET /openapi.yaml`.
