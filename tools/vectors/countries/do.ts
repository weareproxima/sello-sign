import {
	CR_TEXT,
	DS,
	EDGE,
	XSI,
	insertBefore,
	type CountryConfig,
} from '../lib/types'

export default {
	country: 'DO',
	spec: ['sampleEcfXml'],
	cases: (s) => [
		{ name: 'ecf', body: s.sampleEcfXml },
		{ name: 'edge', body: insertBefore(s.sampleEcfXml, '</ECF>', EDGE) },
		{
			name: 'root-xsi',
			body: s.sampleEcfXml.replace('<ECF', `<ECF ${XSI}`),
			note: 'xmlns:xsi on the root, as .NET XmlSerializer writes it',
		},
		{
			name: 'root-ds',
			body: s.sampleEcfXml.replace('<ECF', `<ECF ${DS}`),
			note: 'known TS bug: xmldom serializes the Signature as ds:Signature, which its own verify rejects',
		},
		{
			name: 'no-id',
			body: s.sampleEcfXml.replace(/<eNCF>[^<]*<\/eNCF>/, ''),
			note: 'no eNCF: the document id is random',
		},
		{
			name: 'cr-in-text',
			body: insertBefore(s.sampleEcfXml, '</ECF>', CR_TEXT),
			note: 'known TS bug: xmldom writes the carriage return raw, a verifier reads a line feed',
		},
		{
			name: 'doc-pi',
			body: s.sampleEcfXml.replace(
				'?>',
				'?>\n<?xml-stylesheet type="text/xsl" href="ecf.xsl"?>'
			),
			note: 'known TS bug: URI="" covers the processing instructions outside the root, which TS left out of the digest',
		},
	],
} satisfies CountryConfig
