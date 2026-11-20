# La reconstrucción

Decidida el 2026-11-17 (un monorepo público, el API de firma en Go para todos
los países y el panel en React) y el 2026-11-19 (espacios que guardan
organizaciones, delegaciones, un solo espacio de Sello y sin compatibilidad con
nada de `../sign-old`). El porqué y el modelo, en `../sello/docs/sello-sign.md`;
el contrato, en [`contracts/README.md`](../contracts/README.md).

**El panel es TanStack Start** (decidido el 2026-11-19): el mismo TanStack Router
que las apps de Sello, y un servidor propio donde vive better-auth, que emite el
token con el que el panel llama al API. Así una instalación propia tiene sus
cuentas sin el IdP de Sello, y el API solo verifica tokens con un JWKS, como la
plataforma con el IdP. Creado con
`bunx @tanstack/cli@0.72.16 create panel --framework React --target-dir apps/panel --package-manager bun --deployment nitro --add-ons better-auth,tanstack-query,shadcn --no-examples --no-git --no-toolchain --no-intent --non-interactive`.

## Fase 1: lo que se construye ahora

Cuatro frentes en paralelo, cada uno dueño de sus carpetas:

| Frente                   | Es dueño de                                                                                                                                                         |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Firmadores XMLDSig y JWS | `apps/api/internal/signing` (salvo `countries/{cr,gt,ec,co}` y `xades`), `c14n`, `xmldsig`, los países `sv`, `do`, `pa`, `bo`, `pe`, `uy` y `br`, y `tools/vectors` |
| Firmadores XAdES         | `apps/api/internal/signing/xades` y los países `cr`, `gt`, `ec` y `co`                                                                                              |
| API                      | el resto de `apps/api` y `contracts/openapi.yaml`                                                                                                                   |
| Panel                    | `apps/panel` y `packages/`                                                                                                                                          |

La interfaz `signing.Country` y el registro (`countries.go`) son el punto de
encuentro: se extienden sin romper a quien los usa.

## Los errores de los firmadores de TS que no se copian

- **Bolivia** con `xmlns:xsi` en la raíz, y **República Dominicana** y
  **Uruguay** con `xmlns:ds` en la raíz: firman algo que su propio `verify`
  rechaza (la canonicalización del `SignedInfo` no lleva la declaración que
  hereda).
- Otros que salieron al hacer los vectores (detalle en «Firmadores XMLDSig y
  JWS»): en **Perú**, una raíz sin `xmlns:ext`; en **Bolivia**, un documento
  con comentarios; en todos, un retorno de carro en el texto (`&#13;`) y, con
  `URI=""`, una instrucción de procesamiento antes de la raíz.
- La hora y los `Id` al azar no se pueden fijar desde fuera: `tools/vectors`
  los fija sustituyendo `Date` y el azar de su proceso, sin cambiar
  `../sign-old`.
- Lo que el de Go reproduce tal cual, aunque sea raro, porque es lo que hoy
  acepta cada fisco: el DN del emisor en XAdES (sus atributos al revés, unidos
  por una coma sin espacio), la base64 en líneas de 76 en Ecuador, la
  transformación XPath que Costa Rica declara y no ejecuta, y el contenido del
  JWS de El Salvador, `JSON.stringify(JSON.parse(body))`. Lo que es un error
  de escape (valores de las plantillas sin escapar como XML, `$&` en
  `String.replace`) se corrige, con su prueba.

## Fase 2: después

- El servidor MCP (las herramientas de `../sign-old/docs/api.md`).
- Los avisos y las tareas programadas: certificados por vencer, el uso del
  plan, el reporte de uso a billing cada hora.
- Las invitaciones por correo, la bitácora de auditoría y la exportación.
- Guardar las firmas (`store_signatures`) en un almacén S3.
- Los límites por plan y por minuto.
- Copiar los datos de `../sign-old` (los certificados, sobre todo: la
  plataforma no los guarda), si se decide copiarlos.
- El cambio en la plataforma de Sello: `X-Organization-Id`, el id de la
  organización en vez de `sign_issuer_id` y la delegación al archivar.
- Las pruebas de punta a punta en la nube.
- Cada país contra el ambiente de pruebas de su fisco antes de ofrecerlo.

## Estado

Cada frente deja aquí, al terminar, qué quedó hecho, qué no y cómo se prueba.

### Despliegue

Preparado el 2026-11-19, sin publicar: [`deploy.md`](deploy.md) tiene las
variables, el orden y cómo volver atrás. El panel se construye para Cloudflare
Workers (`bun run build:cloudflare`, el preset `cloudflare_module` de Nitro, y
`apps/panel/wrangler.jsonc`), con `pg` por TCP y un _pool_ por petición
(`src/lib/database.ts`, desde `src/server.ts`) y las tablas de better-auth
nombradas con su esquema; el API, en un contenedor distroless
(`apps/api/Containerfile`, con `api health` para su `HEALTHCHECK`) con su
Valkey (`apps/api/compose.dokploy.yml`). Probado en local: el panel en workerd
(`wrangler dev`) contra el Postgres de `compose.yml` y un API —crear la cuenta,
entrar, el JWKS, el token, `/api/sign/*` y las páginas con sesión, también con
peticiones en paralelo—, y la imagen del API (migrar, la consola, firmar un
DTE con una clave). Falta: la dirección del panel (decisión del dueño), Neon,
el cliente del IdP y probarlo publicado.

### Firmadores XMLDSig y JWS

Hechos, en Go, El Salvador (JWS RS512), República Dominicana, Panamá,
Bolivia, Perú, Uruguay y Brasil (XMLDSig):
`apps/api/internal/signing/countries/{sv,do,pa,bo,pe,uy,br}`, sobre
`internal/signing/xmldsig` (nuevo) y `c14n`. Cada uno implementa
`signing.Country`.

- **Byte a byte:** los 45 casos de `tools/vectors` de estos países (los
  ejemplos de cada spec; `EDGE`, con referencias, CDATA, un comentario y
  espacios de nombres; la raíz con `xmlns:xsi` o `xmlns:ds`; sin identificador
  del documento; en Perú, cada forma de `ext:UBLExtensions`; en Uruguay, un
  sobre con varios CFE, también con prefijo; en El Salvador, el JSON con
  claves que son índices, claves repetidas, escapes y los números de
  JavaScript) dan los mismos bytes que el firmador de TS con la misma clave,
  hora y azar, con el mismo `document_id` y `document_type`, salvo los 8
  casos que corrigen sus errores (abajo).
- **Verificados:** el `Verify` de cada país acepta las firmas de Go y las de
  TS (las correctas), y rechaza un documento alterado; `xmldsig1` de helium
  (solo en las pruebas) acepta las de Go de los seis países XML: en Panamá,
  sin `gNoFirm`, que la ficha deja fuera sin una transformación, y en Uruguay,
  cada CFE como documento aparte, porque ahí `URI=""` es el CFE y no el
  documento. En cada caso corregido, helium rechaza la firma de TS.
- **Lo que se corrige,** cada uno con su vector:
  - `do/root-ds` y `uy/root-ds`: con `xmlns:ds` en la raíz, xmldom escribía
    `ds:Signature` después de firmar sin prefijo. Go escribe
    `<Signature xmlns="…">`.
  - `bo/root-xsi`: el `SignedInfo` se canonicalizaba sin las declaraciones que
    hereda. Ahora con ellas, como manda C14N inclusiva.
  - `pe/no-ext-prefix`: sin `xmlns:ext` en la raíz, xmldom lo declaraba en
    `ext:UBLExtensions` al serializar, cuando el `SignedInfo` ya estaba
    firmado sin él. Go declara el prefijo en el árbol antes de firmar.
  - `bo/edge`: el comentario entraba en el resumen (C14N WithComments), pero
    una referencia `URI=""` quita los comentarios antes de cualquier
    transformación. Go los deja en el documento y fuera del resumen.
  - `do/doc-pi`: `URI=""` es el documento, con sus instrucciones de
    procesamiento fuera de la raíz (nunca la declaración XML). TS resumía
    solo la raíz.
  - `do/cr-in-text` y `br/cr-in-text`: xmldom escribe tal cual un retorno de
    carro en el texto (solo un `&#13;` lo pone ahí), y quien lo lee ve un
    salto de línea: el resumen ya no coincide. Go lo escribe `&#13;` en todos
    los países (`xmldsig.Serialize`). En Brasil la firma de TS valía, porque
    resumía una copia ya releída, pero cambiaba el contenido del documento.
  - Errores de escape en las plantillas, como en XAdES: en estos países no
    hay. El `Id` de Brasil y el de Perú se validan como NCName antes de
    usarlos, y lo demás se arma con el DOM.
- **Lo raro que se conserva:** el JWS de El Salvador sobre
  `JSON.stringify(JSON.parse(body))` (`countries/sv/json.go` lo reproduce:
  el orden de las claves, la última de las repetidas, los números
  —`1.0`→`1`, `1e21`→`1e+21`, demasiado grande→`null`— y los escapes, también
  los de un surrogate suelto); `typ` solo para JSON; una clave RSA de menos de
  2048 bits no firma, como en jsonwebtoken. Un cuerpo XML en El Salvador se
  firma tal cual, con un identificador al azar. En Panamá, `gNoFirm` fuera del
  resumen, y en Uruguay, una firma por CFE con `URI=""`.
- **El documento:** `xmldsig.Parse` reproduce `@xmldom/xmldom` 0.9 con su
  propio analizador, no con `encoding/xml`: los saltos de línea como su
  `normalizeLineEndings`; los espacios literales en atributos; una referencia
  inválida o un `&` suelto (`AT&T`) quedan como texto, igual que en xmldom;
  CDATA se conserva; el texto contiguo es un nodo; `<?xml` solo al principio.
  Rechaza un DOCTYPE (ningún documento fiscal lo lleva, y así no hay
  expansión de entidades), lo que no es UTF-8 y lo que xmldom no acepta.
  `xmldsig.Serialize` escribe lo que su `XMLSerializer`, salvo el `&#13;`.
  `StripWhitespace` y `JSTrim` siguen el `trim()` de JavaScript. Los
  metadatos (`eNCF`, `cuf`, `dId`, `cbc:ID`…) se buscan como los buscaba
  cada firmador (por nombre calificado o local, del documento original).
- **Certificados:** `signing.ParseKeyAndCertificate` lee un PKCS #12
  (`software.sslmate.com/src/go-pkcs12`: AES y también 3DES/RC2) o un PEM con
  la clave, y el certificado del firmante es el de la clave. El número
  tributario: República Dominicana y Bolivia, la regla genérica de sign-old
  (`signing.SerialOrCNTaxID`); Panamá, el OID 2.16.591.1.1.2.2 o la genérica;
  y Perú, Uruguay y Brasil, las reglas de sus `certs/{pe,uy,br}.ts`, que
  sign-old no usaba al subir (Perú habría leído el DNI del `serialNumber`). El
  Salvador lee el `.crt` del MH (también el de `devcert` de Sello): el NIT,
  la vigencia, la clave PKCS #8, y el sujeto, el emisor y la serie de sus
  bloques.

**Cambios en lo compartido,** todos aditivos: en `signing`,
`certificates.go` (`ParseKeyAndCertificate`, `Describe`, `SubjectValue(s)`,
`SerialOrCNTaxID`, `Digits`, `Options.Time`, `Options.Random`, `NewUUID`,
`DocumentIDOr`, `Material.PublicKey`), y el comentario de `Country.Verify`:
una firma que no verifica es `Valid: false` sin error. En `c14n`,
`Options.InclusivePrefixes` (la PrefixList de C14N exclusiva; ningún país de
estos la usa). `internal/signing/signingtest` es lo que comparten las pruebas:
el material fijo, los manifiestos, helium y `Run`. El DOM de `xmldsig` es
también el de los países XAdES (ver «Firmadores XAdES»), que usan su
`SerializeAround` y, para ellos, ganó unas búsquedas (`Child`, `ChildrenNS`,
`FirstDescendant`, `ElementByLocalName`, `TrimmedText`).

Diferencias con xml-crypto que los vectores no ejercitan, porque Go sigue la
recomendación: el orden de los prefijos (código, no `localeCompare`), un
comentario con `&` o `<` dentro de un resumen con comentarios, y una
instrucción de procesamiento dentro del elemento firmado (xml-crypto la
convierte en texto).

Pendiente: probar cada país contra el ambiente de pruebas de su fisco; un
`.crt` real del MH (las pruebas usan uno sintético con su forma y el de
`devcert`); certificados reales de cada país (los números de serie
negativos, que Go rechaza desde la 1.23, serían un problema si alguna CA los
emite); las comprobaciones de `exp`/`nbf` que hacía `jwt.verify` no se
reproducen.

Pruebas, desde `apps/api`:
`go test ./internal/signing/c14n ./internal/signing/xmldsig ./internal/signing/countries/{sv,do,pa,bo,pe,uy,br}`.
Los vectores se regeneran con `cd tools/vectors && bun run gen.ts sv do pa bo pe uy br`
(cómo fija la hora y el azar, en `tools/vectors/README.md`).

### Firmadores XAdES

Hechos, en Go, Costa Rica (XAdES-EPES, canonicalización exclusiva), Guatemala
(XAdES-BES, inclusiva), Ecuador (XAdES-BES, inclusiva, SHA-1) y Colombia
(XAdES-EPES con la política v2 de la DIAN, tres referencias, inclusiva):
`apps/api/internal/signing/countries/{cr,gt,ec,co}`, sobre
`apps/api/internal/signing/xades` y `c14n`. Cada uno implementa
`signing.Country` (`ParseCertificate`, `Sign` y `Verify`).

- **Byte a byte:** los 22 casos de `tools/vectors` (`countries/{cr,gt,ec,co}.ts`:
  los ejemplos de cada spec, el fragmento `EDGE`, la raíz con `xmlns:ds`, sin
  `ID`/`id`, sin identificador del documento, ya certificado en Guatemala,
  sin `ext:UBLExtensions` ni `xmlns:ext` en Colombia…) salen iguales a los del
  firmador de TS con la misma clave, hora y azar (`signing.Options`), con el
  mismo `document_id` y `document_type`.
- **Verificados** por el `Verify` de cada país y por un verificador
  independiente, `xmldsig1` de `github.com/lestrrat-go/helium` (solo en las
  pruebas), las firmas de TS y las de Go; en Costa Rica, helium ejecuta la
  transformación XPath que el firmador declara y no ejecuta, y da lo mismo.
- **Lo raro que se conserva:** el DN del emisor de node-forge
  (`xades.IssuerName`: atributos al revés, `,` sin espacio, sin escapar, los
  nombres cortos o largos de forge o el OID); la base64 en
  líneas de 76 y `<x></x>` en Ecuador; los `Id` de cada país; la XPath de
  Costa Rica; las declaraciones redundantes de `xmlns:ds` y `xmlns:xades`.
- **Lo que se corrige** (`TestEscapesTemplateValues` en cada país): los
  valores que van en las plantillas se escapan como XML (un emisor con `&` o
  `<` daba XML roto) y las plantillas no pasan por `String.replace` (`$&`, `$'`
  y `` $` `` en el DN insertaban otra cosa). La firma se inserta con un
  marcador al serializar, no buscando texto, y los resúmenes se calculan sobre
  la plantilla ya escapada.
- **El documento:** un solo DOM para los once países, el de `xmldsig`
  (descrito en «Firmadores XMLDSig y JWS»): `Parse`, `Serialize` y
  `SerializeAround` (la firma entra donde está el marcador), `EscapeText` y
  `EscapeAttr` (también los de las plantillas de `xades`), `StripWhitespace`,
  `JSTrim` y las búsquedas (`ElementByID`, `ElementByLocalName`,
  `Descendants`, `FirstDescendant`, `ChildrenNS`, `Child`, `Attr`,
  `TextContent`, `TrimmedText`). `xades` tuvo el suyo, sobre `encoding/xml`,
  hasta el 2026-11-19; al dejarlo, XAdES recibe las dos correcciones que ya
  tenía `xmldsig`:
  - un retorno de carro en el texto (solo un `&#13;` lo pone ahí) se escribe
    `&#13;`. xmldom lo escribía tal cual y quien lo lee ve un salto de línea:
    en Costa Rica el resumen dejaba de coincidir, y en Guatemala, Ecuador y
    Colombia, que resumen una copia ya releída, la firma valía pero el
    documento decía otra cosa;
  - un `&` suelto (`AT&T`) o una entidad que no existe quedan como texto,
    como en xmldom; el analizador anterior rechazaba el documento.

  Los 22 vectores dan los mismos bytes que antes; `TestOddText` de cada país
  firma un documento con los dos casos y comprueba que su `Verify` y helium
  aceptan la firma y que el texto llega igual. Como en los demás países, un
  DOCTYPE ahora se rechaza (antes se copiaba).

- **Certificados:** `.p12` con su PIN (`software.sslmate.com/src/go-pkcs12`;
  el certificado es el de la clave, no el primero del archivo) o PEM con la
  clave. El NIT: Guatemala y Colombia, con las reglas de `certs/gt.ts` y
  `certs/co.ts` de sign-old (Colombia no las usaba al subir; ahora sí), y si
  no hay, como Costa Rica y Ecuador, los dígitos del `serialNumber` del sujeto
  o 9 a 11 dígitos seguidos del CN.

Sin cambios en `c14n`: alcanzó lo que tenía. Diferencias con xml-crypto que
los vectores no ejercitan: Go ordena los prefijos por código (xml-crypto usa
`localeCompare`), pasa al ápice los `xml:*` de los ancestros (como manda
C14N 1.0) y no convierte una instrucción de procesamiento en texto.

Pendiente: probar cada país contra el ambiente de pruebas de su fisco; la
cadena de certificados en Colombia (DIAN repite `xades:Cert` por cada uno; TS
solo ponía el del firmante, y Go igual).

**Los nombres no ASCII del DN se corrigen** (decidido el 2026-11-19): node-forge
leía cada valor byte a byte, así que «Cía» salía «CÃ­a». Go decodifica cada
valor por su tipo ASN.1 (UTF8String, BMPString, UniversalString; los demás, un
carácter por byte), y un DN ASCII, como los de las autoridades certificadoras
conocidas, sale igual que antes (`TestIssuerName`).

Pruebas, desde `apps/api`:
`go test ./internal/signing/xades/... ./internal/signing/countries/{cr,gt,ec,co}`.
Los vectores se regeneran con `cd tools/vectors && bun run gen.ts cr gt ec co`.

### Panel

Hecho en `apps/panel` (TanStack Start con Nitro, en `127.0.0.1:4440`), con el
aspecto de `../sign-old`, que es el de Sello: sus tokens y sus piezas de
base-ui vienen del registro público de shadcn de Sello (`@sello`, en
`src/components/ui/`, `src/lib/` y `src/styles/sello/`; ver «Reglas» en
`AGENTS.md`), y las piezas que el panel adaptó a los tokens se llevaron a
`../sello/packages/ui` antes de publicarlas. Textos de usted.

- **Cuentas:** better-auth en el esquema `auth` de la base `sign`
  (`bun run db:migrate`, `scripts/migrate.ts`; nunca en `public`, que es del
  API). Correo y contraseña, con el restablecimiento (el enlace sale por la
  consola del servidor: todavía no hay proveedor de correo). Un proveedor
  OIDC opcional con `genericOAuth` (`OIDC_ISSUER`, `OIDC_CLIENT_ID`,
  `OIDC_CLIENT_SECRET`; `OIDC_ONLY` deja solo «Continuar con …» y apaga el
  registro); con `CLOUD=true` pide también el alcance `workspaces`, y en cada
  inicio de sesión guarda en el usuario el `sub` de Sello y el claim
  `https://sello.tax/workspaces` de su ID token (`selloSub`,
  `selloWorkspaces`).
- **El token para el API:** el plugin `jwt`, EdDSA (Ed25519), 15 minutos,
  `iss` = `BETTER_AUTH_URL`, `aud` = `API_URL`, con `sub`, `email` y `name`, y
  en la nube `sello_sub` y `sello_workspaces`; JWKS en `/api/auth/jwks`. El
  navegador lo pide a `/api/auth/token` y llama a `/api/sign/*`, que el
  servidor del panel pasa tal cual a `API_URL` (sin CORS en el API).
- **Páginas:** entrar, crear una cuenta, recuperar y restablecer la
  contraseña; el selector de espacios y crear uno (o, con `CLOUD=true`, elegir
  la organización de Sello y después «Traer sus organizaciones»); el resumen
  (uso del mes contra el plan, certificados por vencer, actividad reciente);
  las organizaciones (lista con filtro propias/delegadas, crear, detalle con
  sus certificados —subir con PIN, activar, desactivar, borrar, vigencia—,
  sus delegaciones —la de la plataforma de Sello, `locked`, se ve y no se
  quita—, firmar de prueba y sus datos); las claves de API (ambiente,
  permisos, una sola organización opcional, la clave se muestra una vez,
  revocar); el registro de firmas con filtros y su detalle; «General» del
  espacio (con su id, para recibir delegaciones) y sus miembros.
- **Probado de punta a punta** contra el API de `apps/api` con la base de
  `compose.yml`: crear la cuenta e iniciar sesión, el token verificado con
  `jose` contra el JWKS, crear dos espacios, una organización de Costa Rica con
  su `.p12` de desarrollo, firmar de prueba desde el panel y con una clave,
  el registro, el uso, delegar a otro espacio (y que borrar la organización
  con la delegación dé `409`), quitar la delegación, crear y revocar la clave,
  y los miembros; en un navegador, las páginas de día, de noche y en el móvil.
- **Sin probar:** El Salvador (su firmador todavía no estaba en el API:
  `country-not-implemented`), el OIDC contra un proveedor real, la nube
  (`/me/sello-organizations`, abrir el espacio de una organización de Sello y
  traer sus organizaciones), una delegación `locked` y el correo del
  restablecimiento más allá de la consola.
- **Falta:** las invitaciones y el cambio de rol de los miembros (el contrato
  aún no los tiene), la verificación de una firma desde el panel, la cuenta
  (nombre, contraseña, sesiones), un proveedor de correo, la tabla del móvil
  como tarjetas (hoy se desplaza) y el `lint:design` de `../sign-old`.

Cómo se levanta y sus variables, en `apps/panel/README.md` y
`apps/panel/.env.example`: `DATABASE_URL`, `BETTER_AUTH_SECRET`,
`BETTER_AUTH_URL` (= `PANEL_URL` del API), `API_URL`, `CLOUD`, `OIDC_ISSUER`,
`OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `OIDC_ONLY` y, opcionales,
`OIDC_PROVIDER_ID` y `OIDC_PROVIDER_NAME`. Pruebas, desde `apps/panel`:
`bun run typecheck`, `bun run test` y `bun run build`.

### API

Hecho en `apps/api` (Go, solo la biblioteca estándar para HTTP, `pgx` y
`go-redis` para Valkey), con todas las rutas de
[`contracts/README.md`](../contracts/README.md), que `contracts/openapi.yaml`
detalla y el API sirve en `GET /openapi.yaml` (una copia en
`internal/httpapi`, que `go generate ./internal/httpapi` pone al día y una
prueba compara byte a byte; otra compara las rutas que el servidor registra
con las del contrato).

- **Paquetes:** `cmd/api` (el servidor y la consola), `internal/config` (las
  variables), `internal/model` (el vocabulario y sus reglas, sin E/S: roles,
  scopes, ambientes, el alcance de un espacio sobre una organización, el NIT
  sin separadores), `internal/auth` (las claves `sign_test_…`/`sign_prod_…` y
  el verificador del token del panel, el de `pkg/httpkit/jwt.go` de Sello sin
  los tokens OAuth), `internal/secrets` (AES-256-GCM con el byte de versión de
  la clave y el id del certificado como dato asociado), `internal/store`
  (Postgres: las migraciones SQL embebidas, que se aplican al arrancar bajo un
  _advisory lock_ y quedan en `schema_migrations`, y todas las consultas),
  `internal/cache` (las claves en Valkey, por su SHA-256; el material
  descifrado, solo en la memoria del proceso, LRU de 1000 por 10 minutos),
  `internal/billing` (el estado que empuja billing, la activación y el reporte
  de uso cada hora, con un _advisory lock_ para que lo haga una instancia) e
  `internal/httpapi` (las rutas, quién puede cada una, los errores y el
  `Signer`, que abre el material y llama al país).
- **El modelo:** tablas `workspaces`, `workspace_members` (por el `sub` del
  panel), `billing_members` (en la nube, los asientos que empuja billing, por
  el `sub` de Sello: son miembros los primeros `max_members` en el orden de
  billing, sin copiarlos a `workspace_members`), `users` (quién es cada `sub`,
  según su último token, para nombrar a los miembros), `organizations`,
  `certificates`, `delegations`, `api_keys`, `signing_log` (sin el documento,
  y sin llave foránea a la organización: sobrevive a su borrado) y
  `usage_counters` (por espacio que firma, organización, ambiente y mes).
- **Lo que se decidió al hacerlo** (y está en el contrato): toda lista
  responde `{ items, next_cursor }`, y solo se paginan las que crecen; el NIT
  se compara sin separadores (en `X-Organization-Id` y en la idempotencia);
  el espacio delegado renuncia a su delegación, también a una `locked` (así la
  quita la plataforma); una clave no crea otra con más de lo que tiene; un
  espacio cuya organización de Sello se cerró no firma
  (`403 workspace-closed`); un certificado vencido no firma
  (`409 certificate-expired`); en el orden inverso (la empresa contrató Sign
  antes de que la plataforma registrara su organización), el `POST` de la
  plataforma con ese `sello_workspace_id` responde `200` con la organización
  de la empresa, delegada, y se da la delegación `locked`, sin crear otra
  (desde otro espacio sigue siendo `409 sello-workspace-linked`), salvo si esa
  organización tiene otro NIT u otro país: `409 organization-mismatch`, porque
  la plataforma firmaría con el certificado de otro contribuyente; un estado
  cerrado de billing devuelve al espacio de la plataforma, en su misma
  transacción, las organizaciones del espacio de las que ella tiene la
  delegación `locked`, con sus certificados y su historial, sin sus
  delegaciones ni las claves del espacio que eran solo de ellas (las demás
  se quedan, y reabrir no las trae).
- **Probado:** las pruebas unitarias (el token, las claves, el cifrado y su
  rotación, la paginación, el estado de billing, la configuración, el
  `Signer` con un país falso) y, con `POSTGRES_TEST_URL`, las de punta a punta
  contra Postgres, cada una en su propia base, que crea y borra: el recorrido
  de una instalación propia, los roles y las claves de una organización, las
  delegaciones (lo firmado cuenta en el espacio que firma; el dueño lo ve en
  su registro), los países, y la nube con un billing falso (abrir el espacio
  de una organización de Sello, sus estados, el empuje, traer organizaciones
  del espacio de Sello con su certificado, su historial y la delegación
  `locked`, el orden inverso, y un espacio cerrado que devuelve sus
  organizaciones a la plataforma, también con el estado repetido). Además, a mano, firmar y verificar un DTE
  con el firmador de El Salvador y un certificado de
  `../sello/apps/api/tools/devcert`.
- **Falta** (fase 2): los límites por plan y por minuto (se guardan y se
  muestran, pero no se aplican), la retención del registro, las invitaciones,
  los avisos, el MCP y la bitácora de auditoría.

**En local**, desde `apps/api`, con `podman compose up -d` en la raíz:

```sh
cp .env.example .env          # y SIGN_SECRETS_KEY=$(openssl rand -base64 32)
go run ./cmd/api              # o bun run dev en la raíz; migra al arrancar
curl -s 127.0.0.1:4441/health
```

Crear un espacio necesita el token del panel; para probar sin él, la consola
(lee el mismo `.env`) crea un espacio y una clave, y también el espacio de la
plataforma de la nube (`SIGN_PLATFORM_WORKSPACE_ID`):

```sh
WS=$(go run ./cmd/api workspace create -name "Pruebas" -country SV)
KEY=$(go run ./cmd/api key create -workspace $WS -environment test)   # todos los scopes
go run ./cmd/api member add -workspace $WS -user <sub del panel> -role owner

curl -s -X POST 127.0.0.1:4441/workspaces/self/organizations -H "X-API-Key: $KEY" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Empresa de prueba","tax_id":"0614-010101-101-1","country":"SV"}'
ORG=…   # su id
curl -s -X POST 127.0.0.1:4441/workspaces/self/organizations/$ORG/certificates \
  -H "X-API-Key: $KEY" -H 'Content-Type: application/json' \
  -d "{\"environment\":\"test\",\"file\":\"$(base64 < dev.crt | tr -d '\n')\"}"
curl -s -X POST 127.0.0.1:4441/signatures -H "X-API-Key: $KEY" \
  -H 'X-Organization-Id: 06140101011011' -H 'Content-Type: application/json' \
  -d '{"identificacion":{"tipoDte":"01","codigoGeneracion":"6A1D2F3E-1111-4222-8333-944455556666"}}'
curl -s 127.0.0.1:4441/workspaces/self/signatures -H "X-API-Key: $KEY"
```

(`dev.crt`, de `cd ../sello/apps/api && go run ./tools/devcert -nit 06140101011011 -out <ruta>`.)

Variables, en `apps/api/.env.example`: las de «Variables» de `AGENTS.md`, más
`ADDR`, `SIGN_SECRETS_KEY_PREVIOUS` (la clave anterior mientras se rota),
`CORS_ORIGINS`, `LOG_FORMAT` y `LOG_LEVEL`. `PANEL_URL` y `API_URL` valen por
defecto `http://127.0.0.1:4440` y `http://127.0.0.1:4441`, los del panel. Los
logs nunca llevan una clave, un token, un documento ni material de un
certificado: de cada petición, el método, el patrón de la ruta (no el camino,
que puede llevar un NIT), el estado y la duración.

Pruebas, desde `apps/api`: `go vet ./...` y `go test ./...`; con Postgres,
`bun run test:pg` (`POSTGRES_TEST_URL`, por defecto la base de
`compose.yml`).
