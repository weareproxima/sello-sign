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

- El servidor MCP (las herramientas de `../sign-old/docs/api.md`): hecho en
  el API y, cómo conectarlo con una clave, en el panel el 2026-11-20 («API»
  y «Panel»). El landing de Sello lo cuenta desde el mismo día, en las
  preguntas frecuentes de `/sign`.
- Los avisos y las tareas programadas: hechos en el API el 2026-11-20
  («API», «Los avisos»): los certificados por vencer y el uso del plan, por
  correo cada hora; el reporte de uso a billing cada hora ya estaba
  (`billing.RunReporter`).
- La bitácora de auditoría: hecha en el API y en el panel («Actividad») el
  2026-11-20 («API», «La bitácora de auditoría», y «Panel»). (Las
  invitaciones por correo, el cambio de rol
  de los miembros, quitarlos y salir de un espacio, reenviar una invitación,
  verificar una firma por el API desde el panel, exportar los datos de un
  espacio y borrar la cuenta en una instalación propia, hechos el
  2026-11-20: «API» y «Panel».)
- Borrar un espacio (`DELETE /workspaces/{w}`), «Su cuenta» fuera de un
  espacio (`/account`) y cambiar el correo de la cuenta en una instalación
  propia: hechos el 2026-11-20 («API» y «Panel»).
- Guardar las firmas (`store_signatures`) en un almacén S3.
- Los límites por plan y por minuto, y la retención del registro de firmas:
  hechos en el API el 2026-11-20 («API»), y en el panel el mismo día (sus
  errores, los límites del plan junto al uso y los límites propios del
  espacio y de cada clave: «Panel»).
- Copiar los datos de `../sign-old` (los certificados, sobre todo: la
  plataforma no los guarda), si se decide copiarlos.
- El cambio en la plataforma de Sello: `X-Organization-Id`, el id de la
  organización en vez de `sign_issuer_id` y la delegación al archivar.
  Hecho el 2026-11-19 (`../sello`, migración `0024` de la plataforma y
  `internal/emission/infrastructure/sign`).
- Las pruebas de punta a punta en la nube.
- Cada país contra el ambiente de pruebas de su fisco antes de ofrecerlo.

## Estado

Cada frente deja aquí, al terminar, qué quedó hecho, qué no y cómo se prueba.

### Despliegue

Publicado el 2026-11-19 y 20: el panel, en `sign.sello.tax` (el Worker
`sello-sign-panel`, en lugar del viejo `sello-sign`), con su cliente en el
IdP de Sello; el API, en `api.sign.sello.tax` (el servicio Compose «Sign API»
de Dokploy); y la base `sign`, en el proyecto `sello-sign` de Neon, con su rol
`sign_app`. [`deploy.md`](deploy.md) tiene las variables, el orden, cómo se
publica cada cambio y cómo volver atrás. El panel se construye para Cloudflare
Workers (`bun run build:cloudflare`, el preset `cloudflare_module` de Nitro, y
`apps/panel/wrangler.jsonc`), con `pg` por TCP y un _pool_ por petición
(`src/lib/database.ts`, desde `src/server.ts`) y las tablas de better-auth
nombradas con su esquema; el API, en un contenedor distroless
(`apps/api/Containerfile`, con `api health` para su `HEALTHCHECK`) con su
Valkey (`apps/api/compose.dokploy.yml`). Probado en local: el panel en workerd
(`wrangler dev`) contra el Postgres de `compose.yml` y un API —crear la cuenta,
entrar, el JWKS, el token, `/api/sign/*` y las páginas con sesión, también con
peticiones en paralelo—, y la imagen del API (migrar, la consola, firmar un
DTE con una clave). Falta: las pruebas de punta a punta en la nube («Fase
2»).

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
  API). Correo y contraseña, con el restablecimiento, cuyo correo sale por
  Resend (`src/lib/email.ts`, con `RESEND_API_KEY` y `EMAIL_FROM`, en la nube
  de `@sign.sello.tax`; sin la clave, por la consola del servidor) con la
  plantilla de `src/lib/account-email.ts`; better-auth no verifica el correo
  al crear la cuenta ni al entrar: solo manda un enlace al cambiarlo («La
  cuenta», abajo). Un proveedor
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
- **Miembros e invitaciones** (2026-11-20): quien administra el espacio
  invita por correo (el correo y el rol; un administrador no ofrece el de
  propietario), ve las invitaciones pendientes y las vencidas, las revoca y
  cambia el rol de cada miembro con un selector (solo los roles que el API
  le deja dar; cambiar el suyo pide confirmación). Invitar lo hace el
  servidor del panel (`server/invitations.ts`, `lib/invitations.ts`): pide
  al API la invitación con el token de la persona, pone el token que el API
  responde una vez en el enlace (`/invitation?token=…`) y manda el correo con
  `src/lib/email.ts`; si el correo no sale, la invitación queda y la página
  da el enlace para copiarlo, que también se muestra, una vez, tras enviarlo.
  `/invitation` está fuera de `_auth` y de `_app`: sin sesión pide iniciar
  sesión o crear la cuenta y vuelve ahí (`?redirect=`); con sesión muestra el
  espacio, quién invitó y el rol, y la acepta, o dice que venció, que ya se
  aceptó, que se revocó o que es de otro correo (con «Cerrar sesión»). En un
  espacio de una organización de Sello (`sello_workspace_id`) no se invita
  ni se cambia un rol: la página enlaza a «Su cuenta › Organización ›
  Miembros y acceso» del centro de cuenta (`lib/members.ts`).
- **Quitar a un miembro, salir y reenviar** (2026-11-20): cada fila de
  «Miembros» lleva «Quitar» (lo que el API deja: un administrador no quita a
  un propietario, `canRemove` de `lib/members.ts`) o, en la propia, «Salir
  del espacio», con su confirmación («¿Quitar a …?», «¿Salir del
  espacio?»); al último propietario no se le ofrece ninguno. Quien sale
  vuelve al selector de espacios. Cada invitación pendiente, también una
  vencida, lleva «Reenviar»: el servidor del panel le pide al API que la
  renueve y manda el correo con el enlace nuevo
  (`resendInvitation` de `server/invitations.ts`, `resendAndSend` de
  `lib/invitations.ts`); el diálogo dice que el enlace anterior ya no
  funciona y lo da para copiarlo, como al invitar.
- **La cuenta** («Su cuenta», `/account`, fuera de cualquier espacio desde
  el 2026-11-20, para que también llegue quien no está en ninguno): se abre
  desde el menú de la cuenta de cada espacio (con `?redirect=` a la página
  de la que viene, que es adonde vuelve «Volver») y desde «Crear un
  espacio»/«Elija su organización», y lleva «Cerrar sesión». La dirección
  de antes, `/w/<id>/account`, responde `308` a `/account`
  (`lib/moved.ts`). El nombre, la contraseña (pide la actual y puede cerrar
  las demás sesiones; si la cuenta solo entra con el proveedor OIDC, lo
  dice) y las sesiones activas, con cerrar una o las demás, todo con
  better-auth. Con `OIDC_ONLY` el nombre y el correo se ven y no se cambian
  (el proveedor los reescribe en cada inicio de sesión) y no hay contraseña:
  lo dice, y con `CLOUD=true` enlaza a «Datos personales» y «Seguridad» del
  centro de cuenta de Sello (`/u/0/sign/profile` y `/u/0/sign/security`,
  con `?redirect=`); en la nube el correo es siempre el de la cuenta de
  Sello, y enlaza ahí.
- **Cambiar el correo** (2026-11-20, solo en una instalación propia y sin
  `OIDC_ONLY`, `emailChange` de la configuración pública; no a una cuenta
  que entra con el proveedor, que reescribe su correo en cada inicio de
  sesión): «Cambiar el correo» pide el nuevo y la contraseña actual. Lo
  hace el servidor del panel (`requestEmailChange` de `server/account.ts`,
  `lib/account-email-change.ts`): verifica la contraseña, pide a
  better-auth su cambio de correo (`changeEmail`), que manda al correo
  nuevo un enlace de una hora (`changeEmailEmail` de
  `lib/account-email.ts`, por `lib/email.ts`), y avisa al actual
  (`emailChangeNoticeEmail`). El correo cambia al abrir el enlace
  (`/api/auth/verify-email` de better-auth, que lo da por verificado y
  vuelve a `/account?email=changed`, o con `&error=` si venció, no vale o es
  de otra cuenta); entonces la página pide un token nuevo y llama a
  `GET /me`, así que el API guarda el correo nuevo en `users` al momento.
  `/change-email` y `/send-verification-email` de better-auth están
  apagados por HTTP (`disabledPaths`), para que nada cambie el correo sin la
  contraseña, y no se manda ningún correo de verificación al crear la
  cuenta ni al entrar. Si el correo nuevo ya es de otra cuenta, better-auth
  responde igual y no manda nada, para no decir qué correos existen. Las
  invitaciones que esperan siguen siendo para el correo al que se mandaron.
- **Borrar el espacio** (2026-11-20, `components/delete-workspace.tsx`, al
  final de «General», solo para sus propietarios): «¿Borrar el espacio?»,
  con «Esta acción no se puede deshacer.», lo que se borra, «Exportar los
  datos» ahí mismo y el nombre del espacio escrito para confirmarlo
  (`DELETE /workspaces/{w}` con `confirmation`). Si otro espacio usa una de
  sus organizaciones, lo dice con el enlace a «Organizaciones». Al
  borrarlo, vuelve a la entrada, que abre otro de sus espacios, o a «Crear
  un espacio» si no le queda ninguno. En el espacio de una organización de
  Sello (la nube) no se ofrece: dice que se cierra cuando la organización
  se elimina en Sello.
- **Borrar la cuenta** (2026-11-20, solo en una instalación propia: no con
  `CLOUD`, donde se pide por correo, ni con `OIDC_ONLY`, donde el proveedor
  la volvería a crear; `accountDeletion` de la configuración pública): al
  final de «Su cuenta», «Borrar la cuenta» abre «¿Borrar su cuenta?», que
  dice antes qué pasa con cada espacio (`GET /me/account-deletion`): de los
  que es el único propietario con otros miembros, o el único miembro de uno
  cuyas organizaciones usan otros espacios, pide resolverlo primero (con el
  enlace a «Miembros» u «Organizaciones») y no ofrece borrar; si no, lista
  los espacios que se borrarán con la cuenta (aquellos de los que es el
  único miembro, con la sugerencia de exportarlos antes) y de los que
  saldrá, y pide la contraseña (sin ella, si la cuenta solo entra con un
  proveedor, una sesión de menos de 10 minutos). Lo hace el servidor del
  panel (`server/account.ts`, `lib/account-deletion.ts`): verifica la
  contraseña, llama a `DELETE /me` con el token de la persona y solo
  después borra en better-auth el usuario, sus sesiones y sus cuentas.
  `/delete-user` de better-auth sigue apagado, para que nada borre una
  cuenta sin que el API se entere.
- **Verificar una firma** (`/w/<id>/verify`), de dos maneras. **En Sello
  Sign** (por defecto, si el espacio tiene una organización propia, o
  delegada con `sign` o `certificates:read`): se elige la organización y el
  ambiente, y el API verifica con su certificado activo
  (`POST /verifications` con el token, desde el 2026-11-20), en memoria y sin
  guardar nada; así un JWS de El Salvador ya no pide pegar el certificado.
  **En su navegador**, sin enviar nada: el verificador del sitio de Sello
  (`sello.tax/sign/verify`), que es una copia exacta del de `../sign-old`
  (`src/lib/verify-client.ts`, fuera de Prettier; `verify-client.test.ts`
  compara las dos copias si `../sign-old` está al lado); ahí un JWS de El
  Salvador pide el certificado del emisor (pegado o subido).
- **Exportar los datos** (2026-11-20): en «General», para quien administra
  el espacio, «Exportar los datos» descarga el JSON de
  `GET /workspaces/{w}/export` (`lib/download.ts`; el proxy de `/api/sign`
  pasa también `Content-Disposition`).
- **Conectar un agente por MCP** (2026-11-20): en «Claves de API», debajo
  de la lista, cómo conectar el servidor MCP del API con una clave del
  espacio (`components/mcp-connect.tsx`, `lib/mcp.ts`): la dirección
  (`<API_URL>/mcp`; el panel da `API_URL` a sus páginas en
  `PublicConfig.apiUrl`), la cabecera `X-API-Key` (o
  `Authorization: Bearer`), el comando de Claude Code
  (`claude mcp add --transport http sello-sign …`) y el JSON `mcpServers`
  de los demás clientes, para copiar, y qué herramientas ve un agente según
  los permisos de la clave, sin subir certificados ni crear claves. La
  página nunca muestra una clave: lleva `sign_test_…`; solo el diálogo de
  la clave recién creada, que ya la muestra una vez, da el comando con
  ella. `lib/mcp.test.ts` compara las herramientas y sus scopes con las que
  registra `apps/api/internal/httpapi/mcp.go` (y `model.go`), así que una
  herramienta nueva en el API rompe esa prueba hasta que el panel la
  nombre.
- **Los límites del plan** (2026-11-20; `lib/limits.ts`, con la semántica
  de `apps/api/internal/httpapi/limits.go`): en el resumen, las firmas del
  mes de cada ambiente dicen su cuota y, si el tope es otro (el excedente
  de un plan de pago, el margen de prueba), «con tope de …», y pasan a
  `text-bad` al alcanzarlo; debajo, «Límites del plan»
  (`components/plan-limits.tsx`): las firmas por minuto (el techo del plan
  o el que se puso el espacio), las organizaciones propias, los miembros y
  los días del registro, con «Cambiar el plan» en la nube para quien
  administra. En «General», «Firmas por minuto»
  (`components/signing-rate.tsx`): quien administra (`owner`, `admin`,
  como pide el `PATCH`) pone un límite del espacio por debajo del techo o
  lo quita (`null`); el campo solo deja entrar dígitos, dice su error al
  salir y el `400` del API (un techo que bajó) va bajo el campo, y la
  página vuelve a leer el techo. Al crear una clave, «Firmas por minuto»
  opcional, y la lista de claves dice el de cada una («Las del espacio»
  sin el suyo). Los `429` (`rate-limited`, `monthly-limit-reached`) y el
  `409 organization-limit-reached` se dicen con el molde «Alcanzó el
  límite de…» (`limitMessage`, desde `errorMessage`): de quién es el límite
  por minuto y en cuántos segundos volver a intentarlo (`Retry-After`, que
  `ApiError.retryAfter` conserva), o el ambiente y el día, en UTC, en que
  la cuenta del mes empieza de nuevo; `ActionError`
  (`components/limit-alert.tsx`) añade, en la nube y para quien administra,
  «Cambiar el plan» («Su cuenta › Sello Sign › Plan y facturación»,
  `planPageUrl`). Lo usan firmar de prueba, verificar (el API no limita
  `POST /verifications` hoy: queda por si lo hace), crear una organización
  y traer las de Sello.
- **Actividad** (2026-11-20, `/w/<id>/activity`, en la columna del espacio
  entre «Miembros» y «General», solo para quien lo administra; a un
  `member` que llega por la dirección se lo dice, sin llamar al API): la
  bitácora de auditoría del espacio (`GET /workspaces/{w}/audit-events`), del
  cambio más nuevo al más antiguo, de a 50 con «Cargar más». Cada fila dice
  qué pasó en una frase cuyo sujeto es quien lo hizo («Subió un certificado
  de prueba a «Empresa»», «Delegó «Empresa» a «Otro»» o, en el espacio que la
  recibe, «… a este espacio»), con lo que cambió debajo («Nombre: «A» →
  «B»»); quién (la persona con su correo, «Clave «…»» con «Por el API» o
  «Por el servidor MCP», «Espacio «…»», «Sello», «Consola del servidor») y
  cuándo (`es-419`). Filtros: qué (el espacio, miembros, invitaciones,
  organizaciones, certificados, delegaciones, claves), quién (cada miembro,
  las claves, otros espacios, Sello, la consola) y las fechas, en la
  dirección (`?type=`, `?actor=`, `?from=`, `?to=`). Una fila abre «Detalle
  del cambio» (`?event=`), con el objeto y su id, cada cambio y los datos del
  evento. Las frases están en `lib/audit.ts`, y `lib/audit.test.ts` compara
  sus acciones con las de `apps/api/internal/audit/audit.go`: una acción
  nueva en el API rompe esa prueba hasta que el panel la diga.
- **El límite de intentos con la contraseña** (2026-11-20,
  `lib/password-attempts.ts`): cambiar el correo y borrar la cuenta, que
  comprueban la contraseña en el servidor del panel (y no por una ruta de
  better-auth, cuyo límite no los cubre), admiten 5 intentos fallidos por
  cuenta en una ventana de 15 minutos que empieza en el primero; después, la
  contraseña no se comprueba (tampoco la buena) hasta que la ventana
  termina, y la página dice «Alcanzó el límite de intentos con la
  contraseña. Vuelva a intentarlo después de las 09:57 a.m.», con la hora de
  quien la lee. Los dos comparten la cuenta de intentos. Vive en la base del
  panel, no en la memoria del proceso (en Workers cada petición puede caer
  en otro _isolate_): la tabla `auth.password_attempt`, que crea
  `bun run db:migrate` junto a las de better-auth, con una fila por cuenta
  que falló (su id, cuántos fallos y cuándo empezó la ventana; nunca la
  contraseña ni la dirección), que se borra con la contraseña correcta, con
  la cuenta (`on delete cascade`) o, terminada su ventana, en la siguiente
  comprobación de cualquiera. El intento se toma antes de comprobar la
  contraseña, en una sola sentencia (`insert … on conflict … where`), así que
  peticiones a la vez no pasan del límite; una contraseña vacía no gasta
  ninguno. Un bloqueo se escribe en el log del servidor (la cuenta y la
  acción). Probado: la lógica y los dos flujos (`password-attempts.test.ts`,
  `account-email-change.test.ts`, `account-deletion.test.ts`) y, con
  `POSTGRES_TEST_URL`, la tabla misma (cinco y no seis, la ventana que
  termina, la contraseña correcta que lo borra, doce intentos a la vez que
  dejan pasar cinco, las ventanas viejas, la cuenta borrada).
- **En el móvil**, las tablas largas (registro de firmas, organizaciones,
  certificados, delegaciones, claves y la actividad del resumen) son una
  tarjeta por fila (`src/styles/cards.css` y `TableCards`, con `data-label`
  en cada celda), sin desplazarse de lado.
- **`bun run lint:design`** (en el panel o en la raíz): el de Sello y
  `../sign-old` sobre el TSX y el CSS propios del panel
  (`scripts/lint-design.ts`), sin lo que viene del registro `@sello`
  (`src/components/ui/`, `src/lib/utils.ts`, `src/lib/button-variants.ts`,
  `src/styles/sello/`) ni los correos. No mide el contraste: la paleta es la
  de Sello, que la mide allí.
- **Probado de punta a punta** contra el API de `apps/api` con la base de
  `compose.yml`: crear la cuenta e iniciar sesión, el token verificado con
  `jose` contra el JWKS, crear dos espacios, una organización de Costa Rica con
  su `.p12` de desarrollo, firmar de prueba desde el panel y con una clave,
  el registro, el uso, delegar a otro espacio (y que borrar la organización
  con la delegación dé `409`), quitar la delegación, crear y revocar la clave,
  y los miembros; en un navegador, las páginas de día, de noche y en el móvil.
  Las invitaciones, con un API y un panel propios contra otra base: invitar
  (el correo por la consola), abrir el enlace sin sesión, iniciarla, volver y
  aceptar; el enlace de otro correo, el revocado, la invitación repetida, el
  cambio de rol y el último propietario, de día, de noche y en el móvil.
  Lo del 2026-11-20, igual, con el panel construido (`bun run build`) en el
  `:4443` y un API en el `:4444` contra su propia base: quitar a un miembro,
  salir del espacio (y volver al selector), reenviar una invitación,
  verificar en Sello Sign un JWS de El Salvador firmado desde el panel con un
  certificado de `devcert` (válido, sin pegar el certificado) y en el
  navegador, exportar los datos (la descarga y su nombre), y borrar la
  cuenta: bloqueada por ser la única propietaria de un espacio con otros, y
  la de otra persona, que borró su espacio solitario, salió del otro y ya no
  pudo iniciar sesión; de día, de noche y en el móvil.
  El MCP y los límites (2026-11-20), igual, contra un API construido del
  `HEAD` en el `:4444` con su propia base y los límites de la consola
  (`api workspace limits`): el resumen con la cuota, el tope y «Límites del
  plan»; poner, quitar y volver a poner el límite del espacio, uno por
  encima del techo (en el navegador) y el `400` del API tras bajar el techo
  con la consola; firmar de prueba hasta el `429 rate-limited` del espacio
  y hasta el `429 monthly-limit-reached` de prueba; crear una clave con su
  límite, verla en la lista y en el diálogo con su comando, y usar ese
  comando tal cual contra `/mcp` (`initialize`, `tools/list` con las
  herramientas de sus permisos, `sign_document` hasta el `429` de la
  clave); y, con el panel en `CLOUD=true` y la organización de Sello
  puesta a mano en la base, «Cambiar el plan» en el error y en el resumen;
  de día, de noche y en el móvil.
  «Su cuenta» fuera de un espacio, cambiar el correo y borrar un espacio
  (2026-11-20), con el panel en `vite dev` en el `:4446` y un API en el
  `:4447` contra su propia base, en un navegador: llegar a «Su cuenta» sin
  ningún espacio (desde «Crear un espacio»), el `308` de
  `/w/<id>/account`, el menú de la cuenta y «Volver»; pedir el cambio con
  otra contraseña y con la buena (los dos correos por la consola), que
  `/api/auth/change-email` responda `404`, abrir el enlace (el correo
  cambia en better-auth, verificado, y en `users` del API al volver) y el
  enlace vencido; borrar un espacio con una organización delegada a otro
  (el `409` con su enlace), sin ella (la clave deja de valer y la página
  abre el otro espacio), exportar desde el diálogo, borrar el último
  (vuelve a «Crear un espacio») y borrar la cuenta desde `/account` sin
  espacios; de día, de noche, a 390 y a 360 px.
  La bitácora y el límite de intentos (2026-11-20), con el panel en
  `vite dev` en el `:4456` y un API en el `:4454` contra su propia base:
  veinte cambios hechos por el proxy del panel (crear y cambiar el espacio,
  una organización, certificados de El Salvador de `devcert`, claves, una
  revocada por el servidor MCP, invitaciones, un rol, una delegación a otro
  espacio y su renuncia, exportar), y «Actividad» con sus frases, el detalle,
  los filtros de qué y quién, de día, de noche y a 360 px (sin
  desplazamiento lateral; cada fila una tarjeta). En «Su cuenta», cambiar el
  correo con otra contraseña seis veces: cinco «La contraseña no es
  correcta.» y la sexta el límite, que también detiene la contraseña buena;
  la fila de `auth.password_attempt` con sus cinco fallos.
- **Sin probar:** El Salvador (su firmador todavía no estaba en el API:
  `country-not-implemented`), el OIDC contra un proveedor real, la nube
  (`/me/sello-organizations`, abrir el espacio de una organización de Sello y
  traer sus organizaciones), una delegación `locked`, el correo por Resend
  (solo la consola), la cuenta con `OIDC_ONLY` en un navegador (solo el
  typecheck) y el verificador con El Salvador, Guatemala, Panamá y República
  Dominicana desde el panel (se probó Costa Rica, válida y alterada; los
  demás se prueban en `../sign-old`). De los límites, sin probar en un
  navegador: los de un plan de la nube que empuja billing (el nombre del
  plan, el tope del excedente), `organization-limit-reached` (solo su
  prueba) y «Cambiar el plan» contra el centro de cuenta de verdad; del
  MCP, `claude mcp add` con el cliente de Claude Code (se probó el mismo
  pedido con `curl`). Del correo y del espacio, sin probar en un navegador:
  «Su cuenta» con `CLOUD` u `OIDC_ONLY` y una cuenta enlazada al proveedor
  (solo el typecheck y las pruebas de `account-email-change.ts`), el correo
  de verificación por Resend y el bloque de un espacio de una organización
  de Sello. De la bitácora, en un navegador: la vista de un `member` (solo
  el código y el `403` del API), la de la nube (el plan, una organización
  traída o devuelta; solo sus pruebas del API) y «Cargar más» con más de 50
  cambios; del límite, borrar la cuenta (solo sus pruebas: es la misma
  comprobación).
- **Falta:** lo que queda en «Fase 2».

Cómo se levanta y sus variables, en `apps/panel/README.md` y
`apps/panel/.env.example`: `DATABASE_URL`, `BETTER_AUTH_SECRET`,
`BETTER_AUTH_URL` (= `PANEL_URL` del API), `API_URL`, `CLOUD`, `OIDC_ISSUER`,
`OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `OIDC_ONLY` y, opcionales,
`OIDC_PROVIDER_ID`, `OIDC_PROVIDER_NAME`, `RESEND_API_KEY` y `EMAIL_FROM`.
Pruebas, desde `apps/panel`: `bun run typecheck`, `bun run test`,
`bun run lint:design` y `bun run build`.

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
  de uso cada hora, con un _advisory lock_ para que lo haga una instancia),
  `internal/ratelimit` (las firmas por minuto, en Valkey o en memoria),
  `internal/retention` (borrar cada hora lo vencido del registro, con otro
  _advisory lock_), `internal/notices` (los avisos por correo de cada hora,
  con otro), `internal/mail` (el correo: Resend o el log), `internal/audit`
  (el vocabulario de la bitácora de auditoría, sin E/S) e `internal/httpapi` (las rutas, quién puede cada una, los
  errores, los límites al firmar y el `Signer`, que abre el material y llama
  al país).
- **El modelo:** tablas `workspaces`, `workspace_members` (por el `sub` del
  panel), `billing_members` (en la nube, los asientos que empuja billing, por
  el `sub` de Sello: son miembros los primeros `max_members` en el orden de
  billing, sin copiarlos a `workspace_members`), `users` (quién es cada `sub`,
  según su último token, para nombrar a los miembros y escribirles los
  avisos), `organizations`,
  `certificates`, `delegations`, `api_keys`, `signing_log` (sin el documento,
  y sin llave foránea a la organización: sobrevive a su borrado),
  `invitations` (migración `0002`; abajo),
  `usage_counters` (por espacio que firma, organización, ambiente y mes) y
  `workspace_usage` (su suma por espacio, ambiente y mes, en una fila que se
  lee en cada firma; la lleva un _trigger_ de `usage_counters`, así que no se
  aparta de lo que se reporta a billing), `certificate_notices` y
  `usage_notices` (migración `0004`: los avisos que ya salieron; abajo) y
  `audit_events` (migración `0005`: la bitácora de auditoría; abajo).
- **Los límites** (desde el 2026-11-20, migración `0003`; la semántica, en
  `../sello/apps/api/internal/billing/domain/sign.go`): se aplican al firmar,
  en el espacio que firma, también con una organización delegada
  (`httpapi.admit`): primero las firmas por minuto de la clave
  (`api_keys.requests_per_minute`) y del espacio (el techo de su plan, o uno
  menor que se pone él, `workspaces.own_requests_per_minute`), en ventanas
  fijas de un minuto en Valkey (`429 rate-limited`, con `Retry-After` y
  `X-RateLimit-*`; si Valkey falla, se firma igual y se avisa en el log);
  después, las del mes en el ambiente: hasta el tope que empuja billing
  (`prod_hard_cap`, `test_hard_cap`: en un plan de pago, el doble de la cuota
  en producción, cuyo excedente cobra billing, y un 10 % más en prueba) o, sin
  tope, la cuota (`429 monthly-limit-reached`, hasta el primer día del mes
  siguiente). Se lee la cuenta antes de firmar y se suma al registrar: las
  firmas en curso a la vez pueden pasar el tope por unas pocas, y billing no
  cobra más allá de él. Crear o traer una organización más allá de
  `max_organizations` es `409 organization-limit-reached`. Una instalación
  propia no tiene límites salvo los que se configuren: la consola
  (`api workspace limits`, solo para un espacio sin plan) y
  `SIGN_RATE_LIMIT_PER_MINUTE` y `SIGN_LOG_RETENTION_DAYS`, que valen donde la
  consola no puso otro.
- **Las invitaciones y los roles** (desde el 2026-11-20, migración `0002`):
  `owner` y `admin` invitan un correo (normalizado en minúsculas) con un
  rol, y solo un `owner` invita o hace a otro `owner`. El token
  (`sign_inv_…`, 256 bits en base62, `auth.NewInvitationToken`) se responde
  una sola vez y se guarda su SHA-256, como el de una clave; vale 7 días
  (`store.InvitationLifetime`) y lo acepta, una vez, la cuenta del panel cuyo
  `email` es el de la invitación. Una abierta por correo y espacio (índice
  único parcial `invitations_open`); a una vencida la revoca y reemplaza la
  siguiente. Aceptar la misma con la misma cuenta responde lo mismo; quien ya
  estaba en el espacio conserva su rol. Invitar, aceptar y cambiar un rol
  bloquean la fila del espacio (`SELECT … FOR UPDATE`), así que el límite de
  miembros (`max_members`, contra `workspace_members`) y «el espacio nunca se
  queda sin propietario» (`409 last-owner`) valen con peticiones a la vez.
  Solo se cambia el rol de quien se agregó en Sign (`workspace_members`). **En
  la nube,** un espacio de una organización del IdP (`sello_workspace_id`)
  tiene por miembros los asientos que empuja billing (`billing_members`), que
  se invitan y cambian en el centro de cuenta de Sello, como en
  `../sign-old`: invitar, revocar, aceptar o cambiar un rol ahí es
  `409 members-managed-by-sello`. Las invitaciones son de los demás
  espacios: los de una instalación propia y el de la plataforma.
- **Quitar a un miembro y renovar una invitación** (2026-11-20, sin
  migración): `DELETE /workspaces/{w}/members/{user_id}` (`store.RemoveMember`,
  `model.CanRemove`): `owner` y `admin` quitan a otros (solo un `owner` a un
  `owner`) y cualquiera sale por sí mismo; bloquea la fila del espacio, como
  cambiar un rol, así que el último propietario no se va
  (`409 last-owner`). Lo del espacio se queda (las claves que creó la
  persona, sus invitaciones, sus firmas en el registro): es del espacio, no
  de ella. Reenviar es **renovar**
  (`POST /workspaces/{w}/invitations/{i}/renewals`, `store.RenewInvitation`):
  el API guarda solo el SHA-256 del token, así que no puede volver a dar el
  mismo; la misma fila recibe otro hash y 7 días desde ahora, en una
  transacción, y responde el token nuevo una vez. Se eligió eso y no
  revocar y crear otra (dos llamadas que pueden quedar a medias, otro id)
  ni un reenvío del mismo enlace (que pediría guardar el token): la
  invitación sigue siendo una, y el enlace viejo deja de valer. Valen las
  comprobaciones de crearla (`CanInvite` con su rol, `already-member`,
  `workspace-full`).
- **Verificar con el token** (2026-11-20): `POST /verifications` acepta
  también el token del panel. `X-Organization-Id` (id o NIT) se resuelve
  entre las organizaciones de todos los espacios de la persona
  (`store.ResolveOrganizationForPerson`: la del dueño primero, después una
  delegación que firma); cualquier rol vale, en un espacio dueño o en uno
  delegado con `sign` o `certificates:read`, y el ambiente va en
  `environment` del cuerpo, obligatorio con el token (con una clave, el de
  la clave). Verificar no se registra ni cuenta en el uso. Un certificado
  vencido ya no impide verificar (con una clave tampoco): lo que firmó
  mientras valía se puede comprobar; firmar con él sigue siendo
  `409 certificate-expired`.
- **Exportar** (2026-11-20): `GET /workspaces/{w}/export` (token, `owner` o
  `admin`; `export.go`) arma al momento un JSON (`sello-sign-export/1`) y no
  guarda nada: el espacio, sus organizaciones (propias y delegadas) con todos
  sus certificados (los datos públicos; `null` si la delegación no deja
  leerlos) y las delegaciones que dio, sus claves también revocadas (sin
  hash), sus miembros, sus invitaciones abiertas (sin token), su uso y su
  registro de firmas, que va al final y se escribe a medida que se lee, de a
  1000 (`store.EachSignature`), sin tener una conexión mientras escribe. Lo
  anterior al registro se lee antes de responder (un error es un `500`
  normal); si el registro falla a medias, la conexión se corta
  (`http.ErrAbortHandler`) para que no quede un JSON que parezca entero. No
  es una foto de un instante: cada parte se lee por separado.
- **Borrar la cuenta** (2026-11-20, instalación propia; en la nube,
  `404 not-self-hosted`): `GET /me/account-deletion` dice, por espacio,
  `leave`, `delete` (es su único miembro), `transfer` (su único propietario,
  con otros miembros) o `delegated` (su único miembro, con organizaciones que
  otros espacios usan); los dos últimos impiden borrarla
  (`store.AccountSpaces`). `DELETE /me` (`store.DeleteAccount`) lo vuelve a
  calcular bajo el bloqueo de esos espacios, en orden de id, y en la misma
  transacción borra los de `delete` (sus organizaciones primero, que no
  caen en cascada desde el espacio; con ellas, sus certificados, sus
  delegaciones y sus claves; con el espacio, lo demás), las membresías y la
  fila de `users`, o responde `409 account-has-workspaces` sin cambiar nada.
  Después saca de la caché de Valkey las claves borradas y olvida el
  material de los certificados. Lo que la persona hizo en los espacios que
  quedan (sus claves, sus invitaciones, su `user_id` en el registro) se
  queda hasta que el espacio lo borre o venza. Se decidió borrar los
  espacios de los que es el único miembro, y no dejarlos, porque nadie
  podría administrarlos y sus claves seguirían firmando, y no borrar nunca
  uno con otros miembros. Lo que se borra con cada espacio es lo mismo que
  con `DELETE /workspaces/{w}` (`store.deleteSpaces`, abajo).
- **Borrar un espacio** (2026-11-20, sin migración):
  `DELETE /workspaces/{w}` (`deleteWorkspace` de `workspaces.go`,
  `store.DeleteWorkspace` de `store/deletion.go`). Decidido al hacerlo:
  - **Quién:** solo un `owner`, con el token (una clave, aunque tenga todos
    los scopes, `403`; el MCP no lo tiene). Un `admin` administra el
    espacio, pero borrarlo es del dueño.
  - **La confirmación:** el cuerpo lleva `confirmation`, el nombre del
    espacio tal como está (sin los blancos de los lados), como GitHub; se
    compara con el nombre bajo el bloqueo de la fila, así que un cambio de
    nombre a la vez también lo para (`400 confirmation-mismatch`). Se
    eligió el nombre y no el id porque es lo que la persona reconoce y
    escribe, y no un encabezado aparte porque el proxy del panel solo pasa
    los del contrato.
  - **Nunca:** el espacio de la plataforma (`SIGN_PLATFORM_WORKSPACE_ID`,
    `409 platform-workspace`) ni, en la nube, el de una organización del
    IdP (`sello_workspace_id`, `409 workspace-managed-by-sello`): su plan lo
    cobra billing y sus miembros los empuja, y se cierra cuando Sello
    elimina la organización (`../sello/docs/sello-sign.md` y
    `../sello/apps/api/DEPLOY.md` §10.13 no prevén que Sign lo borre por su
    cuenta: billing seguiría cobrando el plan, y abrirlo otra vez lo
    crearía vacío). Así que en la nube solo se borra uno sin organización
    del IdP que haya creado la consola, y en una instalación propia,
    cualquiera.
  - **El uso:** billing solo recibe el de los espacios de una organización
    del IdP (`store.UsageReport`), que no se borran, así que borrar nunca
    pierde un uso que no se le haya reportado. El del espacio borrado se va
    con él (`usage_counters` y `workspace_usage` caen en cascada), y no
    hace falta guardarlo: nadie lo cobra.
  - **Las delegaciones:** mientras otro espacio use una de sus
    organizaciones, `409 workspace-has-delegations` y no cambia nada, como
    `organization-has-delegations` al borrar una organización y `delegated`
    al borrar la cuenta. Antes de comprobarlo, las organizaciones de las que
    la plataforma tiene la delegación `locked` vuelven a su espacio
    (`returnToPlatform`, lo mismo que al cerrar un espacio), como dice
    `../sello/docs/sello-sign.md` («Al cerrar el espacio…»), con sus
    certificados y su historial; las demás se borran. Las delegaciones que
    otros espacios le dieron se van con él.
  - **Lo que se borra** (`store.deleteSpaces`, que comparte con
    `DELETE /me`), en una transacción que bloquea el espacio y sus
    organizaciones (una delegación, una clave o un certificado nuevos
    esperan): sus organizaciones con sus certificados (el material cifrado
    y sus datos) y las claves de una sola de ellas, también las de otro
    espacio; con el espacio, sus miembros, sus asientos de billing, las
    delegaciones que recibió, sus claves, sus invitaciones (también las
    aceptadas y revocadas), su registro de firmas y su uso. Las claves que
    tenían valor salen de la caché de Valkey al momento (también las de
    otro espacio de una de sus organizaciones, que antes no se sacaban al
    borrar la cuenta) y el material descifrado de sus certificados se
    olvida en este proceso. En las demás instancias, el material que
    tuvieran en memoria dura hasta 10 minutos y, sin Valkey, una clave en
    su caché hasta 5, pero ya no hay espacio, organización ni certificado
    con que firmar. Lo que otros espacios firmaron con sus organizaciones se queda
    en su registro (con `organization_name` `null`) y en su uso.
- **La retención del registro:** cada entrada de `signing_log` se guarda los
  días de `retention_days` del espacio que firmó (en la nube, los del plan:
  7, 30, 90, 180 y 365 días de Free a Business, sin plazo en Enterprise; en
  una instalación propia, sin plazo salvo los que se configuren), también la
  que firmó con una organización de otro espacio: es su registro, y ahí se
  contó. Una tarea la borra cada hora, en lotes de 5000 y bajo un _advisory
  lock_, por el id (UUIDv7, ordenado por tiempo), con el índice que ya existía.
  Lo contado en `usage_counters` se queda: se cobra.
- **Los avisos** (desde el 2026-11-20, migración `0004`;
  `internal/notices`, con el correo en `internal/mail`): el API escribe por
  su cuenta, igual en la nube que en una instalación propia, lo que un
  espacio tiene que atender antes de que deje de firmar. Una tarea, un minuto
  después de arrancar y luego cada hora, bajo su propio _advisory lock_
  (`0x5e110_516e`), como la retención. Decidido al hacerlo:
  - **Los certificados:** cada certificado activo de una organización, 30, 7
    y 1 días antes de su `not_after`, y una vez si venció y sigue activo
    (`notices.CertificateThreshold`). Solo el umbral más urgente que toque, y
    nunca uno menos urgente después (uno subido 5 días antes de vencer recibe
    solo el de 7), como el bucle de vencimientos de la plataforma de Sello;
    los de un espacio que tocan en la misma vuelta van en un solo correo, con
    un enlace a la página de cada organización
    (`<PANEL_URL>/w/{w}/organizations/{o}`). Un certificado inactivo no
    avisa.
  - **El uso del plan:** lo firmado en el mes (UTC) en un ambiente, de
    `workspace_usage`, contra la cuota (`max_prod_signatures`,
    `max_test_signatures`) y el tope (`prod_hard_cap`, `test_hard_cap`; sin
    tope, la cuota): al 80 % de la cuota; al llegar a la cuota, solo cuando
    se sigue firmando después (el excedente que cobra billing en producción y
    el margen de prueba de un plan de pago); y al llegar al tope, «Alcanzó el
    límite de…». Solo el más alto alcanzado (`notices.UsageThreshold`), una
    vez por espacio, ambiente, mes, umbral y límite: un plan que cambia en el
    mes se mide de nuevo. Un espacio sin cuota ni tope (Enterprise, una
    instalación sin límites) no recibe ninguno, y uno con los límites de la
    consola, sí, sin hablar de un plan. Con un enlace al resumen del espacio
    (`<PANEL_URL>/w/{w}`).
  - **A quién:** los `owner` y `admin` del espacio (`store.NoticeRecipients`:
    los de `workspace_members` y, en la nube, los asientos de billing que el
    plan admite, por su cuenta de Sello), al correo que dio el último token
    del panel de cada uno (`users.email`), sin repetir uno. Quien nunca entró
    al panel no tiene correo aquí y no recibe nada; un `member`, tampoco.
  - **Las delegaciones:** el aviso de un certificado va solo al espacio dueño
    de la organización, el que sube sus certificados. Un espacio delegado no
    lo recibe: no siempre puede leerlos (`certificates:read`) ni cambiarlos,
    y así cada certificado avisa una vez. El uso sí es del espacio que firma,
    también con una organización delegada: ahí se cuenta.
  - **Quién no:** el espacio de la plataforma (`SIGN_PLATFORM_WORKSPACE_ID`):
    los certificados de sus organizaciones los avisa la plataforma de Sello a
    cada organización (su bucle de vencimientos), y no tiene plan; tampoco un
    espacio cuya organización Sello cerró, que ya no firma.
  - **El envío:** con `RESEND_API_KEY`, por el API HTTP de Resend
    (`POST /emails`, desde `EMAIL_FROM`, las mismas variables que el panel),
    un correo por destinatario, en texto y en HTML simple, sin imágenes ni
    seguimiento, y uno cada 600 ms (el límite de Resend es de la cuenta, que
    comparte el panel); sin ella, al log, como el panel. Un aviso se apunta
    (`certificate_notices`, `usage_notices`) solo después de que Resend lo
    aceptó para cada destinatario, o lo rechazó para siempre (un 400 o un 422:
    una dirección inválida) y lo aceptó para otro; si no, se intenta otra vez
    la hora siguiente, con la misma `Idempotency-Key` por aviso y
    destinatario (Resend la recuerda 24 horas), así que quien ya lo recibió no
    lo recibe otra vez. Por eso el texto no depende de cuándo sale (dice la
    fecha y la hora UTC del vencimiento, no los días que faltan). Un 401, un
    403, un 429 o un 5xx es temporal. Ni el log ni los errores llevan la
    dirección, salvo el log de una instalación sin Resend, que es el correo.
  - **Lo que se guarda:** de cada aviso, el certificado o el espacio, el
    ambiente, el mes, el umbral, el límite y cuándo salió; nunca los
    destinatarios ni el texto. Las filas de un certificado se van con él; las
    del uso, al empezar el segundo mes después del suyo (la misma tarea).
  - **Probado:** los umbrales (sus bordes, lo que ya salió, cada plan), los
    textos exactos de cada aviso y su HTML escapado, la vuelta con una base y
    un correo falsos (un correo por espacio, cada destinatario con su clave,
    lo que no se repite, los reintentos con el mismo mensaje y la misma
    clave, una dirección rechazada, nadie a quien escribir, otra instancia
    con el bloqueo), el envío contra un Resend falso (la petición, cada
    respuesta, el intervalo) y, con `POSTGRES_TEST_URL`
    (`internal/notices/integration_test.go`), los destinatarios de cada
    espacio (los roles, sin correo, los asientos que el plan admite, la misma
    dirección dos veces), el espacio delegado, el de la plataforma y uno
    cerrado que no reciben nada, el uso sin límites, lo que se apunta y no se
    repite, el tope, un fallo de Resend que espera a la vuelta siguiente, el
    bloqueo, la purga del uso viejo y lo que se va con su certificado.
- **La bitácora de auditoría** (desde el 2026-11-20, migración `0005`;
  `internal/audit`, `store/audit.go` y `httpapi/audit.go`; el contrato, en
  «Registro de actividad» de `contracts/README.md`): quién cambió qué en un
  espacio, para quien lo administra (`GET /workspaces/{w}/audit-events`).
  Decidido al hacerlo:
  - **Qué se registra:** cada cambio de una persona (el token) o de una
    clave (también por el servidor MCP, que llama en proceso a la misma
    ruta) al espacio y a lo que guarda: sus datos y sus firmas por minuto,
    sus miembros (rol, quitar, salir, también al borrar la cuenta), sus
    invitaciones (enviar, reenviar, revocar, aceptar), sus organizaciones
    (crear, cambiar, borrar, traerlas de la plataforma y devolverlas), sus
    certificados (subir, activar, desactivar, borrar), sus delegaciones
    (dar, quitar, renunciar) y sus claves (crear, revocar, también las que
    caen con su organización); lo que empuja billing en la nube (el plan y
    sus límites, el nombre, cerrar y reabrir; no los asientos, que son los
    miembros de la organización en Sello), lo que hace la consola
    (`workspace create`, `member add`, `workspace limits`, `key create`) y
    cada exportación. No las firmas ni las verificaciones: el registro de
    firmas ya tiene cada firma. Un cambio que no cambia nada (el mismo rol,
    un `PATCH` con lo mismo, una organización que ya existía) no se
    registra. La lista, con lo que dice cada una, en `audit.Actions` y en
    `AuditAction` del contrato.
  - **En la misma transacción que el cambio:** cada función del _store_ que
    cambia algo escribe su evento con el mismo `tx` (`store.record`), así
    que hay un evento exactamente cuando hay un cambio: ni un cambio sin
    evento si el proceso cae a la mitad, ni un evento de un cambio que se
    deshizo; y lo de antes (el rol anterior, el nombre anterior, el
    certificado que dejó de estar activo) se lee bajo el mismo bloqueo. Si
    escribir el evento falla, el cambio tampoco se hace. Las funciones que
    eran una sola sentencia pasaron a una transacción. La exportación no
    cambia nada: su evento se escribe solo, antes de enviarla, y si falla no
    se envía.
  - **El actor, en el contexto de la petición:** `authed` lo pone una vez
    por petición (`audit.WithActor`): la persona con el `sub`, el correo y
    el nombre de su token, o la clave por su id (y `via: mcp` si la llamó
    una herramienta, que marca `mcpDispatch`); la consola pone el suyo, y
    billing es el actor de todo lo que aplica `ApplyState`. Así no cambió la
    firma de ninguna función del _store_ salvo las de los certificados, que
    reciben el espacio desde el que se actúa. Lo que cambia sin actor (una
    prueba que llama al _store_) queda como `system`. El nombre de una clave
    o de un espacio se lee al escribir el evento, en la misma transacción: es
    el de ese momento.
  - **Entre espacios:** un cambio de otro espacio a lo que este guarda (una
    delegación que le dan o le quitan, un certificado que sube a una
    organización suya quien la tiene delegada con `certificates:write`, como
    la plataforma, una organización que la empresa se lleva o que vuelve a la
    plataforma) queda en los dos, y en el que no actuó el actor es el otro
    espacio (`actor.type: workspace`, con su nombre), nunca su persona ni su
    clave, que no son de su incumbencia. Al borrar un espacio, la
    plataforma ve que ese espacio le devolvió sus organizaciones, no quién
    lo borró.
  - **Qué guarda cada evento:** la hora; el actor (su tipo, el `sub`, el id
    de la clave o del espacio, el correo y el nombre de la persona según su
    token, el nombre de la clave o del espacio, y `via`); la acción
    (`certificate.uploaded`…); el objeto (su tipo, su id y su nombre en ese
    momento: el de la organización, también para sus certificados y
    delegaciones, el de la clave, el correo del miembro o de la invitación);
    y un JSON pequeño con lo que cambió (antes y después, el rol, el
    ambiente, la serie y el vencimiento de un certificado, los permisos de
    una delegación o de una clave). Nunca un secreto: ni claves, ni sus
    hashes, ni tokens de invitación, ni el material de un certificado, ni su
    PIN.
  - **Sin IP ni navegador:** por minimizar los datos, como el resto de
    Sello, y porque no servirían: las personas llegan al API por el proxy del
    panel (`/api/sign/*`), que no pasa la dirección ni el navegador, así que
    el API vería siempre la del panel; guardarlas pediría confiar en una
    cabecera reenviada, y la política de privacidad tendría que decirlo. El
    log de las peticiones tampoco las guarda.
  - **Quién lo lee:** con el token, `owner` y `admin` (un `member`, `403`);
    una clave, `403`, y por eso no está en el servidor MCP: la bitácora nombra
    a personas (los correos de los miembros y de los invitados), que hoy
    ninguna clave lee (miembros e invitaciones son solo del token), y ningún
    scope de hoy encaja: `logs:read` es el registro de firmas, y darle esto
    cambiaría lo que ve una clave que ya existe. Si hace falta, sería un
    scope nuevo (`audit:read`).
  - **Cuánto dura:** 365 días, sea cual sea el plan (`audit.RetentionDays`,
    una sola constante); la tarea de la retención los borra cada hora con el
    registro de firmas, bajo su mismo _advisory lock_, en lotes, por el id
    (UUIDv7). Se van con su espacio (`ON DELETE CASCADE`). Los de las
    personas que borran su cuenta se quedan en los espacios de los que
    salieron, con su correo, hasta que venzan: son lo que pasó en ese
    espacio.
  - **La exportación** lleva la bitácora (`audit_events`), de la más nueva a
    la más antigua, antes del registro de firmas y escrita igual, a medida
    que se lee; la primera es esa exportación.
  - **Probado,** con `POSTGRES_TEST_URL` (`httpapi/audit_test.go`): cada
    clase de cambio de una instalación propia con su actor y sus detalles
    (también los que no cambian nada, una clave que crea otra, una revocada
    por el servidor MCP, la clave que cae con su organización, la consola),
    el espacio delegado que sube un certificado y renuncia (en los dos
    espacios), quién lo lee, los filtros y la paginación, que ningún secreto
    aparezca, la exportación; en la nube, el plan, cerrar y reabrir, traer y
    devolver organizaciones y la delegación bloqueada de la plataforma, en los
    dos espacios; la retención (lo de hace más de 365 días, con el registro de
    firmas) y borrar la cuenta (su espacio se va con su bitácora; el otro
    conserva lo que hizo y que salió). `deletion_test.go` cuenta también
    `audit_events` al borrar un espacio, y que la plataforma ve al espacio
    borrado, no a su persona. Sin base, `internal/audit` (el actor, los
    cambios, las acciones).
- **El servidor MCP** (desde el 2026-11-20, `internal/httpapi/mcp.go`; el
  contrato, en «Servidor MCP» de `contracts/README.md`): `POST /mcp`, con el
  SDK oficial de Go (`github.com/modelcontextprotocol/go-sdk` v1.8.0, el de
  Sello Chat; la única dependencia más, y no es de HTTP: el ruteo sigue
  siendo el `ServeMux`), Streamable HTTP sin estado y con respuestas JSON;
  `GET` y `DELETE /mcp` son `405` (registrados a mano: la ruta comodín
  respondería `404`). Se entra con una clave (`X-API-Key` o
  `Authorization: Bearer sign_…`), que se resuelve como en cada ruta; cada
  `POST` arma un servidor con las herramientas que sus scopes permiten, y
  cada herramienta es una llamada en proceso a su ruta, por el mismo
  `ServeMux` y con la misma clave: no hay un segundo camino para los
  permisos, los límites, los errores ni el registro de firmas (y el log dice
  `mcp tool` con la herramienta, la ruta, el estado y la duración, nunca los
  argumentos). Las herramientas son las de `../sign-old` con el modelo nuevo
  (organizaciones en vez de emisores; además, `get_workspace`, `get_usage` y
  las delegaciones), sin subir certificados ni crear claves, como allí.
  Decidido al hacerlo: los argumentos se leen tal como llegan, sin las
  herramientas tipadas del SDK, que los decodifican a un mapa y los vuelven a
  escribir (el documento perdería el orden de sus claves, y el JWS de El
  Salvador lo firma); `deactivate_certificate` lleva `destructiveHint`
  porque deja de firmar; un error de la ruta es el resultado de la
  herramienta, con su problema y `retry_after`. **Probado:** que cada
  herramienta llama a una ruta que existe, la lista según los scopes y sus
  anotaciones, los argumentos (el documento byte a byte) y, con
  `POSTGRES_TEST_URL`, una conversación por HTTP (`initialize`,
  `tools/list` según los scopes, una firma que llega igual al firmador y
  queda en el registro con su clave, sus errores, el límite por minuto como
  error de la herramienta, sin clave, con el token del panel, otro origen y
  `GET`) y la gestión con el cliente del SDK (organizaciones, certificados,
  delegaciones, una clave de una sola organización, revocar una clave, que
  deja de entrar). A mano, firmar y verificar un DTE con el firmador de El
  Salvador por `sign_document`, con las claves en su orden.
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
  organizaciones a la plataforma, también con el estado repetido), los
  límites (el del mes, también el de un plan de pago con su excedente y el de
  lo firmado por delegación; el de cada minuto, del espacio, del que se pone
  y de una clave, con sus cabeceras; el de organizaciones, también al
  traerlas; los de la instalación) y la retención (lo vencido de cada espacio,
  lo de otro espacio con otra retención, el uso que se queda y la tarea que
  no corre si otra instancia la tiene), las invitaciones (crear, ver, aceptar
  con su correo y no con otro, la repetida, la revocada, la vencida y su
  reemplazo, la aceptada por otra cuenta, el límite de miembros, un espacio
  de una organización de Sello) y los roles (lo que puede un administrador,
  el último propietario, también con dos propietarios que se quitan el rol a
  la vez), y desde el 2026-11-20 quitar a un miembro y salir (también en un
  espacio de una organización de Sello), renovar una invitación (vencida, la
  aceptada, la de un propietario, un espacio lleno), verificar con el token
  (por id y NIT, el ambiente, de otro espacio, una delegación sin `sign`
  ni `certificates:read` y con `certificates:read`, un certificado vencido,
  una clave con otro ambiente, que no se registra), la exportación (sin
  hash, token ni clave; con más de un lote del registro; una organización
  delegada) y borrar la cuenta (cada desenlace, lo que bloquea y lo que se
  borra, la clave que deja de valer al momento, y la nube), y borrar un
  espacio (`deletion_test.go`: quién puede, la confirmación, el `409` con
  una delegación y que no cambie nada, lo que queda en cada tabla, la
  clave propia y la de otro espacio que dejan de valer al momento, el
  registro y el uso del otro espacio que se quedan, el espacio de la
  plataforma, el de una organización de Sello y, en la nube, la
  organización que vuelve a la plataforma con su certificado). Con `VALKEY_TEST_URL`, el límite por
  minuto contra Valkey. Además, a mano, firmar y verificar un DTE
  con el firmador de El Salvador y un certificado de
  `../sello/apps/api/tools/devcert`.
- **Falta** (fase 2): de los avisos, que una persona elija no recibirlos,
  avisos en el panel y otros avisos (una suscripción que vence, una firma
  que falla); de la bitácora, leerla con una clave (un scope nuevo) y los
  asientos que empuja billing.

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
go run ./cmd/api workspace limits -workspace $WS -prod 500 -rpm 60   # opcional: límites de un espacio sin plan

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
`CORS_ORIGINS`, `SIGN_RATE_LIMIT_PER_MINUTE` y `SIGN_LOG_RETENTION_DAYS` (los
límites de los espacios sin plan; vacías, ninguno), `RESEND_API_KEY` y
`EMAIL_FROM` (los avisos; sin la clave, van al log), `LOG_FORMAT` y
`LOG_LEVEL`. `PANEL_URL` y `API_URL` valen por
defecto `http://127.0.0.1:4440` y `http://127.0.0.1:4441`, los del panel. Los
logs nunca llevan una clave, un token, un documento ni material de un
certificado: de cada petición, el método, el patrón de la ruta (no el camino,
que puede llevar un NIT), el estado y la duración.

Pruebas, desde `apps/api`: `go vet ./...` y `go test ./...`; con Postgres,
`bun run test:pg` (`POSTGRES_TEST_URL` y `VALKEY_TEST_URL`, por defecto los de
`compose.yml`).
