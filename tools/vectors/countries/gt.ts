import { EDGE, insertBefore, type CountryConfig } from '../lib/types'

export default {
	country: 'GT',
	spec: ['sampleDte', 'buildDte'],
	cases: (s) => [
		{ name: 'dte', body: s.sampleDte },
		{
			name: 'edge',
			body: insertBefore(s.sampleDte, '</dte:DatosEmision>', EDGE),
			note: 'EDGE inside the signed DatosEmision',
		},
		{
			name: 'adenda',
			body: s.buildDte({ adenda: 'Pedido 42 &amp; más' }),
			note: 'an Adenda, never signed',
		},
		{
			name: 'certificado',
			body: s.buildDte({ certificacion: true, certifier: true }),
			note: 'already certified: NumeroAutorizacion is the id and the issuer signature goes before FirmaCertificador',
		},
		{
			name: 'no-id',
			body: s.sampleDte.replace(
				'<dte:DatosEmision ID="DatosEmision">',
				'<dte:DatosEmision>'
			),
			note: 'DatosEmision without ID: the signer adds it',
		},
		{
			name: 'no-numero',
			body: s.sampleDte.replace(' NumeroAcceso="987654321"', ''),
			note: 'no NumeroAcceso: the document id is random',
		},
	],
} satisfies CountryConfig
