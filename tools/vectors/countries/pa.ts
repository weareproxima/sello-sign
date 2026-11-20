import { EDGE, XSI, insertBefore, type CountryConfig } from '../lib/types'

export default {
	country: 'PA',
	spec: ['sampleRfe', 'sampleRfeWithNoFirm', 'sampleRfeCompact'],
	cases: (s) => [
		{ name: 'rfe', body: s.sampleRfe },
		{
			name: 'rfe-no-firm',
			body: s.sampleRfeWithNoFirm,
			note: 'gNoFirm present: kept last and out of the digest',
		},
		{ name: 'rfe-compact', body: s.sampleRfeCompact },
		{ name: 'edge', body: insertBefore(s.sampleRfe, '</rFE>', EDGE) },
		{ name: 'root-xsi', body: s.sampleRfe.replace('<rFE', `<rFE ${XSI}`) },
	],
} satisfies CountryConfig
