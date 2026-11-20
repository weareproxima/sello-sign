import { DS, EDGE, insertBefore, type CountryConfig } from '../lib/types'

export default {
	country: 'UY',
	spec: ['bareCfeXml', 'cfeAdendaXml', 'envioCfeXml'],
	cases: (s) => [
		{ name: 'cfe', body: s.bareCfeXml },
		{ name: 'cfe-adenda', body: s.cfeAdendaXml },
		{
			name: 'envio',
			body: s.envioCfeXml,
			note: 'one signature per CFE, each with URI="" over its own CFE',
		},
		{ name: 'edge', body: insertBefore(s.bareCfeXml, '</CFE>', EDGE) },
		{
			name: 'envio-prefixed',
			body: s.envioCfeXml
				.replaceAll('<CFE xmlns="http://cfe.dgi.gub.uy"', '<DGICFE:CFE')
				.replaceAll('</CFE>', '</DGICFE:CFE>'),
			note: 'prefixed CFEs that inherit their namespace from the envelope',
		},
		{
			name: 'root-ds',
			body: s.bareCfeXml.replace('<CFE', `<CFE ${DS}`),
			note: 'known TS bug: xmldom serializes the Signature as ds:Signature, which its own verify rejects',
		},
	],
} satisfies CountryConfig
