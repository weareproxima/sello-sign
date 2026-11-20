export type ContentType = 'application/json' | 'application/xml'

export interface Case {
	/** File stem, unique within the country: `[a-z0-9-]+`. */
	name: string
	body: string
	/** Defaults to the country's. */
	contentType?: ContentType
	/** Why the case exists, for whoever reads the manifest. */
	note?: string
}

export interface CountryConfig {
	/** ISO code, upper case, as getCountrySigner takes it. */
	country: string
	contentType?: ContentType
	/**
	 * Names the country's spec (`signers/<cc>.spec.ts`) declares before its
	 * first `describe(`; gen.ts extracts that preamble and hands them to
	 * cases() as they are (a function stays a function).
	 */
	spec?: string[]
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	cases(spec: Record<string, any>): Case[]
}

export const XSI = 'xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"'
export const DS = 'xmlns:ds="http://www.w3.org/2000/09/xmldsig#"'

/**
 * Markup that exercises parsing, escaping and canonicalization: entities and
 * character references in text and attributes, literal whitespace in an
 * attribute (normalized to spaces by the parser), CDATA, a comment, empty and
 * self-closing elements, non-ASCII text and a prefixed child with its own
 * namespace and attributes in two namespaces.
 */
export const EDGE = [
	'<Edge a="x&amp;y" b="l1&#10;l2" c="t&#9;t" d=\'sq "dq" &lt;\' e="lit\ttab\nline" f="&gt;">',
	'texto &amp; &lt;b&gt; "q" \'a\' ñ € 😀 &#x41;&#66;',
	'<![CDATA[<raw> & ]]>',
	'<!-- comentario -->',
	'<Vacio></Vacio><Otro/>',
	'<p:Extra xmlns:p="urn:extra" p:z="1" z="2" a="3"><p:Inner>x</p:Inner></p:Extra>',
	'</Edge>',
].join('')

/** Inserts fragment right before the first occurrence of `before`. */
export function insertBefore(
	xml: string,
	before: string,
	fragment: string
): string {
	const at = xml.indexOf(before)
	if (at < 0) throw new Error(`vectors: ${before} not found`)
	return xml.slice(0, at) + fragment + xml.slice(at)
}

/**
 * A carriage return in text, which only a character reference can carry:
 * xmldom serializes it raw, so the document a verifier reads (where it is a
 * line feed) is not the one that was digested.
 */
export const CR_TEXT = '<Nota>linea 1&#13;linea 2</Nota>'
