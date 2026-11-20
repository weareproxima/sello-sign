import { DS, EDGE, insertBefore, type CountryConfig } from '../lib/types'

export default {
	country: 'CR',
	spec: ['sampleFe'],
	cases: (s) => [
		{ name: 'fe', body: s.sampleFe },
		{
			name: 'edge',
			body: insertBefore(s.sampleFe, '</FacturaElectronica>', EDGE),
		},
		{
			name: 'root-ds',
			body: s.sampleFe.replace(
				'<FacturaElectronica',
				`<FacturaElectronica ${DS}`
			),
			note: 'xmlns:ds already on the root',
		},
		{
			name: 'nota-credito',
			body: s.sampleFe.replace(/FacturaElectronica/g, 'NotaCreditoElectronica'),
			note: 'document type 03 from the root name',
		},
		{
			name: 'no-clave',
			body: s.sampleFe.replace(/<Clave>[^<]*<\/Clave>/, ''),
			note: 'no Clave: the document id is random',
		},
	],
} satisfies CountryConfig
