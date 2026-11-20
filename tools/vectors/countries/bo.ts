import { EDGE, XSI, insertBefore, type CountryConfig } from '../lib/types'

export default {
	country: 'BO',
	spec: ['sampleFacturaXml'],
	cases: (s) => [
		{ name: 'factura', body: s.sampleFacturaXml },
		{
			name: 'edge',
			body: insertBefore(
				s.sampleFacturaXml,
				'</facturaElectronicaCompraVenta>',
				EDGE
			),
			note: 'the comment is part of the digest (C14N WithComments)',
		},
		{
			name: 'root-xsi',
			body: s.sampleFacturaXml.replace(
				'<facturaElectronicaCompraVenta',
				`<facturaElectronicaCompraVenta ${XSI}`
			),
			note: 'known TS bug: SignedInfo is canonicalized without the xmlns:xsi it inherits',
		},
	],
} satisfies CountryConfig
