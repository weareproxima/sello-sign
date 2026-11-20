import type { CountryConfig } from '../lib/types'

const edge = `{
  "identificacion": { "codigoGeneracion": "0F1E2D3C-4B5A-4987-8654-3210FEDCBA98", "tipoDte": "03", "version": 3 },
  "2": "índice dos", "1": "índice uno", "10": "diez", "01": "no es índice",
  "texto": "a\\u00e9 \\"comillas\\" \\\\ \\/ <b>&amp; \\u2028 \\n\\t\\u0001 😀",
  "numeros": [1.0, 1e21, 1e-7, 0.1, -0, 123456789012345678901, 1.5e300, 1E2, 0.000001, 2.5e-7, 1e400],
  "anidado": { "b": true, "a": null, "c": [] },
  "texto": "duplicada: gana la última, en el lugar de la primera"
}`

export default {
	country: 'SV',
	contentType: 'application/json',
	spec: ['sampleDte', 'sampleDteJson', 'sampleDteXml'],
	cases: (s) => [
		{ name: 'dte', body: s.sampleDteJson },
		{
			name: 'dte-pretty',
			body: JSON.stringify(s.sampleDte, null, 2),
			note: 'JSON.stringify(JSON.parse(body)) compacts it',
		},
		{
			name: 'dte-xml',
			body: s.sampleDteXml,
			contentType: 'application/xml',
			note: 'an XML body is signed as is, without typ in the header and with a random document id',
		},
		{
			name: 'json-edge',
			body: edge,
			note: 'index-like keys first, duplicate keys, string escapes and JS number formatting',
		},
		{
			name: 'no-id',
			body: '{"identificacion":{"tipoDte":"01","codigoGeneracion":7}}',
			note: 'codigoGeneracion is not a string: the document id is crypto.randomUUID().toUpperCase()',
		},
	],
} satisfies CountryConfig
