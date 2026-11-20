import { EDGE, insertBefore, type CountryConfig } from '../lib/types'

const EXT_NS =
	'xmlns:ext="urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2"'

export default {
	country: 'CO',
	spec: ['sampleInvoice', 'sampleCreditNote', 'buildInvoice'],
	cases: (s) => {
		const first = (s.sampleInvoice as string).match(
			/\t\t<ext:UBLExtension>[\s\S]*?<\/ext:UBLExtension>/
		)![0]
		return [
			{ name: 'invoice', body: s.sampleInvoice },
			{ name: 'credit-note', body: s.sampleCreditNote },
			{ name: 'edge', body: insertBefore(s.sampleInvoice, '</Invoice>', EDGE) },
			{
				name: 'one-extension',
				body: s.buildInvoice({
					extensions: `\t<ext:UBLExtensions>\n${first}\n\t</ext:UBLExtensions>`,
				}),
				note: 'only the DianExtensions: the signer appends the second UBLExtension',
			},
			{
				name: 'no-extensions',
				body: s.buildInvoice({ extensions: '' }),
				note: 'no ext:UBLExtensions at all: the signer creates it',
			},
			{
				name: 'no-ext-ns',
				body: s.buildInvoice({ extensions: '' }).replace(` ${EXT_NS}`, ''),
				note: 'no xmlns:ext in scope: the serializer declares it on the new ext:UBLExtensions',
			},
		]
	},
} satisfies CountryConfig
