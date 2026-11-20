# Vectores de referencia

Firma documentos de referencia con los firmadores de TS de `../sign-old` (que
solo se lee) para comprobar que los de Go dan los mismos bytes. Escribe solo en
`apps/api/internal/signing/testdata/vectors/<cc>/`.

```sh
cd tools/vectors && bun run gen.ts          # todos los países de countries/
bun run tools/vectors/gen.ts do pa          # desde la raíz, solo esos
```

`node_modules` es un enlace a `../../../sign-old/node_modules` (no versionado:
`ln -s ../../../sign-old/node_modules tools/vectors/node_modules`). Otra copia de
sign-old: `SIGN_OLD=/ruta/a/sign-old/src/lib`.

## Qué fija

Todo lo que una firma toma del mundo, solo dentro del proceso de bun
(`lib/determinism.ts`):

- **La clave y el certificado**: `apps/api/internal/signing/testdata/key.pem`
  (RSA 2048, PKCS #8) y `cert.pem` (autofirmado, `CN=EMPRESA DE PRUEBA,
O=Pruebas, C=SV`, serie `57E1C2BCB5F78CB05CED90D5BB246B39`, del 2026-01-01 al
  2031-01-01). Son de prueba y solo viven ahí.
- **La hora**: `Date` se reemplaza; `new Date()` y `Date.now()` dan
  `2026-11-19T15:04:05.123Z` (`now` del manifiesto; en Go, `Options.Now`).
- **El azar**: un flujo de bytes por caso (SHA-256 en modo contador con la
  semilla `<cc>/<caso>`), del que salen `crypto.randomUUID()` y
  `crypto.getRandomValues()` de `globalThis`, `Math.random()` y, del módulo
  `crypto` de Node que importan los firmadores, `randomUUID`, `randomInt`,
  `randomBytes` y `getRandomValues` (un plugin de bun reescribe sus `import`
  al cargarlos: sign-old no cambia). Cómo sale cada valor de los bytes, para
  que Go lo derive igual de `Options.Rand`, está en `lib/random.ts`:
  `randomUUID` toma 16 bytes con los bits de la versión 4 (`signing.NewUUID`);
  `randomInt(min, max)`, el algoritmo de Node con lecturas de 6 bytes.

## Lo que escribe

Por país, `manifest.json` y, por caso, `<caso>.in.xml` (o `.in.json`) con el
documento tal cual y `<caso>.out.xml` (o `.out.jws`) con lo que respondió el
firmador de TS. Cada caso del manifiesto lleva `document_id`, `document_type`,
`self_verified` (si el `verify` del propio firmador de TS acepta su firma: los
que no, son sus errores conocidos), `error` si no firmó, `random` (en hex,
todos los bytes del flujo que consumió, en orden: en Go,
`Options.Rand = bytes.NewReader(...)`) y `random_calls` (cada llamada y su
resultado).

Las pruebas de Go los leen con `internal/signing/signingtest` (`Load`, `Run`).

## Un país

Cada país es un archivo, `countries/<cc>.ts`, que exporta su configuración
(`lib/types.ts`):

```ts
export default {
	country: 'DO',
	spec: ['sampleEcfXml'], // lo que su spec declara antes del primer describe(
	cases: (s) => [
		{ name: 'ecf', body: s.sampleEcfXml },
		{ name: 'root-xsi', body: s.sampleEcfXml.replace('<ECF', `<ECF ${XSI}`) },
	],
} satisfies CountryConfig
```

Los ejemplos salen del preámbulo de `signers/<cc>.spec.ts` de sign-old, que
`lib/samples.ts` extrae a `.cache/`, importa y borra; una función se
entrega como función. `lib/types.ts` trae también `XSI`, `DS`, `EDGE` (un
fragmento con referencias, CDATA, un comentario y espacios de nombres) y
`CR_TEXT` (un retorno de carro en el texto). Volver a generar un país borra y
reescribe su carpeta; dos ejecuciones dan los mismos bytes.
