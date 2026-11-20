import { DS, EDGE, insertBefore, type CountryConfig } from '../lib/types'

export default {
	country: 'PE',
	spec: [
		'buildInvoice',
		'FILLED_EXTENSIONS',
		'SLOT_FIRST_EXTENSIONS',
		'CREDIT_NOTE',
		'SUMMARY_DOCUMENTS',
		'VOIDED_DOCUMENTS',
		'DESPATCH_ADVICE',
	],
	cases: (s) => [
		{ name: 'invoice', body: s.buildInvoice() },
		{
			name: 'boleta',
			body: s.buildInvoice({ typeCode: '03', signatureUri: '#signatureKG' }),
		},
		{
			name: 'filled-extensions',
			body: s.buildInvoice({ extensions: s.FILLED_EXTENSIONS }),
			note: 'a new UBLExtension is appended',
		},
		{
			name: 'slot-first',
			body: s.buildInvoice({ extensions: s.SLOT_FIRST_EXTENSIONS }),
		},
		{
			name: 'no-extensions',
			body: s.buildInvoice({ extensions: '' }),
			note: 'ext:UBLExtensions is created',
		},
		{
			name: 'no-ext-prefix',
			body: s
				.buildInvoice({ extensions: '' })
				.replace(/ xmlns:ext="[^"]*"/, ''),
			note: 'the root does not declare ext: xmldom declares it on the new UBLExtensions',
		},
		{
			name: 'root-ds',
			body: s.buildInvoice().replace('<Invoice', `<Invoice ${DS}`),
			note: 'ds already declared on the root',
		},
		{ name: 'credit-note', body: s.CREDIT_NOTE },
		{ name: 'summary', body: s.SUMMARY_DOCUMENTS },
		{ name: 'voided', body: s.VOIDED_DOCUMENTS },
		{ name: 'despatch', body: s.DESPATCH_ADVICE },
		{ name: 'edge', body: insertBefore(s.buildInvoice(), '</Invoice>', EDGE) },
	],
} satisfies CountryConfig
