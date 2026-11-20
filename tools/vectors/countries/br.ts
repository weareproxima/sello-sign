import {
	CR_TEXT,
	EDGE,
	XSI,
	insertBefore,
	type CountryConfig,
} from '../lib/types'

export default {
	country: 'BR',
	spec: ['sampleNfe', 'sampleNfce', 'sampleDps', 'sampleEvento'],
	cases: (s) => [
		{ name: 'nfe', body: s.sampleNfe },
		{ name: 'nfce', body: s.sampleNfce },
		{ name: 'dps', body: s.sampleDps },
		{ name: 'evento', body: s.sampleEvento },
		{ name: 'edge', body: insertBefore(s.sampleNfe, '</infNFe>', EDGE) },
		{ name: 'root-xsi', body: s.sampleNfe.replace('<NFe', `<NFe ${XSI}`) },
		{
			name: 'cr-in-text',
			body: insertBefore(s.sampleNfe, '</infNFe>', CR_TEXT),
			note: 'known TS bug: xmldom writes the carriage return raw, a verifier reads a line feed',
		},
	],
} satisfies CountryConfig
