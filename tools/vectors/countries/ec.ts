import { EDGE, insertBefore, type CountryConfig } from '../lib/types'

export default {
	country: 'EC',
	spec: ['sampleFactura', 'sampleComprobante', 'infoTributaria'],
	cases: (s) => [
		{ name: 'factura', body: s.sampleFactura },
		{ name: 'edge', body: insertBefore(s.sampleFactura, '</factura>', EDGE) },
		{
			name: 'no-id',
			body: s.sampleFactura.replace(
				'<factura id="comprobante" version="1.1.0">',
				'<factura version="1.1.0">'
			),
			note: 'root without id: the signer adds id="comprobante"',
		},
		{ name: 'nota-credito', body: s.sampleComprobante('notaCredito', '04') },
		{
			name: 'no-clave',
			body: `<?xml version="1.0" encoding="UTF-8"?>\n<guiaRemision id="comprobante" version="1.1.0">\n${s.infoTributaria('', '')}\n</guiaRemision>`,
			note: 'no claveAcceso and no codDoc: random id, type from the root name',
		},
	],
} satisfies CountryConfig
