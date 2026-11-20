/**
 * Client-side verification for supported Sello Sign formats.
 *
 * Supports:
 * - SV: JWT RS512
 * - DO: XMLDSig RSA-SHA256, inclusive C14N
 * - CR: XAdES-EPES RSA-SHA256, exclusive C14N
 * - PA: XMLDSig RSA-SHA256, exclusive C14N (rFE, `gNoFirm` excluded)
 * - GT: XAdES-BES RSA-SHA256, inclusive C14N (FEL, `FirmaEmisor`)
 *
 * All processing happens in the browser.
 *
 * The page that runs it is on Sello's site (`sello.tax/sign/verify`, in
 * Sello's monorepo), which carries a byte-for-byte copy of this file and
 * checks it against this one: a change here is copied there. Its tests stay
 * here (`verify-client.spec.ts`), against the real signers.
 *
 * ## Canonicalization
 *
 * The four XML countries do not agree on how a signed node is turned into
 * bytes, so this module carries a small C14N implementation instead of
 * serializing the DOM and patching namespace declarations back in with string
 * surgery. It mirrors the semantics of `xml-crypto` — the library every server
 * signer canonicalizes with — for both algorithms:
 *
 * - **Exclusive** (`…/2001/10/xml-exc-c14n#`, CR and PA): a namespace
 *   declaration is emitted at the node that visibly uses the prefix, and
 *   declarations inherited from ancestors are dropped.
 * - **Inclusive** (`…/TR/2001/REC-xml-c14n-20010315`, DO and GT): declarations
 *   in scope at the apex of the signed subtree are re-emitted on it, sorted by
 *   prefix, so a node can only be digested in its document position.
 *
 * Which one applies is read from the document itself —
 * `SignedInfo/CanonicalizationMethod/@Algorithm` for `SignedInfo`, and the last
 * C14N `Transform` of each `Reference` (falling back to the former) for the
 * referenced nodes — rather than from the country, so a document only has to be
 * self-consistent to verify.
 */

const DSIG_NS = 'http://www.w3.org/2000/09/xmldsig#';

const INCLUSIVE_C14N_ALGO = 'http://www.w3.org/TR/2001/REC-xml-c14n-20010315';
const EXCLUSIVE_C14N_ALGO = 'http://www.w3.org/2001/10/xml-exc-c14n#';
const ENVELOPED_TRANSFORM_ALGO = 'http://www.w3.org/2000/09/xmldsig#enveloped-signature';

/** `Id` of the issuer signature in a Guatemalan FEL document (`gt.ts`). */
const GT_ISSUER_SIGNATURE_ID = 'FirmaEmisor';
/** Group G10 of a Panamanian rFE — excluded from the digest by the ficha. */
const PA_UNSIGNED_GROUP = 'gNoFirm';

const NODE_TYPE_ELEMENT = 1;
const NODE_TYPE_TEXT = 3;
const NODE_TYPE_CDATA = 4;

/** Attributes a same-document `Reference URI="#id"` may resolve through. */
const ID_ATTRIBUTES = ['ID', 'Id', 'id'];

export type ClientVerifyCountry = 'SV' | 'DO' | 'CR' | 'PA' | 'GT';

export interface ClientVerifyResult {
	valid: boolean;
	payload: unknown;
	error?: string;
	kind?: 'jwt' | 'xml';
	country?: ClientVerifyCountry;
	documentId?: string | null;
	documentType?: string | null;
}

type XmlCountry = Exclude<ClientVerifyCountry, 'SV'>;

type C14nMode = 'inclusive' | 'exclusive';

interface NamespaceDeclaration {
	prefix: string;
	namespaceURI: string;
}

interface XmlMetadata {
	country: XmlCountry;
	documentId: string | null;
	documentType: string | null;
}

interface Asn1Node {
	tag: number;
	data: Uint8Array;
	children?: Asn1Node[];
}

function base64urlToString(str: string): string {
	const base64 = str.replace(/-/g, '+').replace(/_/g, '/');
	const padded = base64 + '='.repeat((4 - (base64.length % 4)) % 4);
	return atob(padded);
}

function base64urlToBuffer(str: string): ArrayBuffer {
	const binary = base64urlToString(str);
	const bytes = new Uint8Array(binary.length);
	for (let i = 0; i < binary.length; i++) {
		bytes[i] = binary.charCodeAt(i);
	}
	return bytes.buffer;
}

function base64ToBuffer(base64: string): ArrayBuffer {
	const binary = atob(base64);
	const bytes = new Uint8Array(binary.length);
	for (let i = 0; i < binary.length; i++) {
		bytes[i] = binary.charCodeAt(i);
	}
	return bytes.buffer;
}

function bufferToBase64(buffer: ArrayBuffer): string {
	const bytes = new Uint8Array(buffer);
	let binary = '';
	for (let i = 0; i < bytes.length; i++) {
		binary += String.fromCharCode(bytes[i]);
	}
	return btoa(binary);
}

function integerToBase64url(data: Uint8Array): string {
	let start = 0;
	while (start < data.length - 1 && data[start] === 0) start++;
	const trimmed = data.slice(start);
	let binary = '';
	for (let i = 0; i < trimmed.length; i++) {
		binary += String.fromCharCode(trimmed[i]);
	}
	return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

function parseAsn1(bytes: Uint8Array, offset: number = 0): { node: Asn1Node; end: number } {
	const tag = bytes[offset];
	let pos = offset + 1;
	let length = bytes[pos++];

	if (length & 0x80) {
		const numLenBytes = length & 0x7f;
		length = 0;
		for (let i = 0; i < numLenBytes; i++) {
			length = (length << 8) | bytes[pos++];
		}
	}

	const data = bytes.slice(pos, pos + length);
	const node: Asn1Node = { tag, data };

	if (tag === 0x30) {
		node.children = [];
		let childPos = 0;
		while (childPos < data.length) {
			const { node: child, end } = parseAsn1(data, childPos);
			node.children.push(child);
			childPos = end;
		}
	}

	return { node, end: pos + length };
}

function jwkFromPkcs8(der: ArrayBuffer): JsonWebKey {
	const bytes = new Uint8Array(der);
	const { node: root } = parseAsn1(bytes);
	if (!root.children || root.children.length < 3) {
		throw new Error('Invalid PKCS#8 structure');
	}
	return jwkFromRsaPrivateKey(root.children[2].data);
}

function jwkFromRsaPrivateKey(der: Uint8Array): JsonWebKey {
	const { node: root } = parseAsn1(der);
	if (!root.children || root.children.length < 3) {
		throw new Error('Invalid RSAPrivateKey structure');
	}

	return {
		kty: 'RSA',
		n: integerToBase64url(root.children[1].data),
		e: integerToBase64url(root.children[2].data)
	};
}

function spkiFromX509Der(der: ArrayBuffer): ArrayBuffer {
	const { node: root } = parseAsn1(new Uint8Array(der));
	if (!root.children || root.children.length < 1 || !root.children[0].children) {
		throw new Error('Invalid X.509 certificate');
	}

	const tbs = root.children[0];
	const tbsChildren = tbs.children;
	if (!tbsChildren) {
		throw new Error('Invalid X.509 certificate structure');
	}
	const spki = tbsChildren.find(
		(child) =>
			child.tag === 0x30 &&
			child.children &&
			child.children.length >= 2 &&
			child.children[1].tag === 0x03
	);

	if (!spki) {
		throw new Error('SubjectPublicKeyInfo not found in certificate');
	}

	return derSequence(spki.data);
}

/** DER definite-length octets — short form under 128 bytes, long form above. */
function derLength(length: number): number[] {
	if (length < 0x80) return [length];

	const octets: number[] = [];
	let remaining = length;
	while (remaining > 0) {
		octets.unshift(remaining & 0xff);
		remaining = Math.floor(remaining / 256);
	}
	return [0x80 | octets.length, ...octets];
}

/**
 * Re-wraps parsed content in a DER `SEQUENCE`.
 *
 * A 2048-bit `SubjectPublicKeyInfo` is ~270 bytes, so the length has to be
 * encoded in long form; emitting only the short form silently rejected every
 * real certificate.
 */
function derSequence(content: Uint8Array): ArrayBuffer {
	const header = derLength(content.length);
	const bytes = new Uint8Array(1 + header.length + content.length);
	bytes[0] = 0x30;
	bytes.set(header, 1);
	bytes.set(content, 1 + header.length);
	return bytes.buffer;
}

/**
 * Strips any character outside the standard base64 alphabet.
 *
 * Run AFTER normalizing escape sequences (see `normalizeCertContent`) so that
 * stray letters like the `n` in a literal `\n` are not preserved as legitimate
 * base64 characters (which would silently corrupt the decoded bytes).
 */
function cleanBase64(input: string): string {
	return input.replace(/[^A-Za-z0-9+/=]/g, '');
}

/**
 * Normalizes pasted certificate / key content.
 *
 * Handles the common case where users paste the raw output of `redis-cli` /
 * `keydb-cli` (or other tools), which wraps the value in quotes and represents
 * newlines as the literal two-character escape sequence `\n` (backslash + n).
 *
 * Without this, the `n` in `\n` survives base64 cleanup as a valid base64
 * character and corrupts the decoded bytes — the parser ends up reading
 * garbage instead of the expected `SEQUENCE` tag.
 */
function normalizeCertContent(input: string): string {
	let s = input.trim();
	// Strip a single layer of surrounding quotes (double or single).
	if ((s.startsWith('"') && s.endsWith('"')) || (s.startsWith("'") && s.endsWith("'"))) {
		s = s.slice(1, -1);
	}
	// Convert literal escape sequences to their real character.
	s = s
		.replace(/\\r\\n/g, '\n')
		.replace(/\\n/g, '\n')
		.replace(/\\r/g, '\n')
		.replace(/\\t/g, '\t');
	return s;
}

async function importRsaVerifyKey(
	rawContent: string,
	hash: 'SHA-256' | 'SHA-512'
): Promise<CryptoKey> {
	const algorithm = { name: 'RSASSA-PKCS1-v1_5', hash };
	const content = normalizeCertContent(rawContent);

	if (content.includes('<CertificadoMH>')) {
		const block = content.match(/<publicKey>([\s\S]*?)<\/publicKey>/);
		if (!block) throw new Error('No public key found in certificate');
		const match = block[1].match(/<encodied>([^<]*)<\/encodied>/);
		if (!match) throw new Error('No encoded public key data found');
		return crypto.subtle.importKey(
			'spki',
			base64ToBuffer(cleanBase64(match[1])),
			algorithm,
			false,
			['verify']
		);
	}

	if (content.includes('BEGIN CERTIFICATE')) {
		const base64 = cleanBase64(
			content.replace(/-----BEGIN CERTIFICATE-----/g, '').replace(/-----END CERTIFICATE-----/g, '')
		);
		const spki = spkiFromX509Der(base64ToBuffer(base64));
		return crypto.subtle.importKey('spki', spki, algorithm, false, ['verify']);
	}

	if (content.includes('BEGIN PUBLIC KEY')) {
		const base64 = cleanBase64(
			content.replace(/-----BEGIN PUBLIC KEY-----/g, '').replace(/-----END PUBLIC KEY-----/g, '')
		);
		return crypto.subtle.importKey('spki', base64ToBuffer(base64), algorithm, false, ['verify']);
	}

	if (content.includes('BEGIN RSA PRIVATE KEY')) {
		const base64 = cleanBase64(
			content
				.replace(/-----BEGIN RSA PRIVATE KEY-----/g, '')
				.replace(/-----END RSA PRIVATE KEY-----/g, '')
		);
		const jwk = jwkFromRsaPrivateKey(new Uint8Array(base64ToBuffer(base64)));
		return crypto.subtle.importKey('jwk', jwk, algorithm, false, ['verify']);
	}

	if (content.includes('BEGIN PRIVATE KEY')) {
		const base64 = cleanBase64(
			content.replace(/-----BEGIN PRIVATE KEY-----/g, '').replace(/-----END PRIVATE KEY-----/g, '')
		);
		const jwk = jwkFromPkcs8(base64ToBuffer(base64));
		return crypto.subtle.importKey('jwk', jwk, algorithm, false, ['verify']);
	}

	throw new Error('Unsupported certificate format');
}

function isJwt(input: string): boolean {
	const trimmed = input.trim();
	if (trimmed.includes('<')) return false;
	return trimmed.split('.').length === 3;
}

/**
 * Removes whitespace-only text nodes.
 *
 * Every server signer normalises the document this way before hashing
 * (`preserveWhiteSpace = false` in the PHP references DO and PA follow), so a
 * pretty-printed and a compact document digest alike.
 */
function stripWhitespaceNodes(node: Node): void {
	const toRemove: Node[] = [];
	for (let i = 0; i < node.childNodes.length; i++) {
		const child = node.childNodes[i];
		if (child.nodeType === NODE_TYPE_TEXT && child.nodeValue?.trim() === '') {
			toRemove.push(child);
		} else if (child.nodeType === NODE_TYPE_ELEMENT) {
			stripWhitespaceNodes(child);
		}
	}
	for (const child of toRemove) {
		node.removeChild(child);
	}
}

// ── Canonicalization (XML-C14N 1.0, inclusive and exclusive) ───────────────

const TEXT_ESCAPES: Record<string, string> = {
	'&': '&amp;',
	'<': '&lt;',
	'>': '&gt;',
	'\r': '&#xD;'
};

const ATTRIBUTE_ESCAPES: Record<string, string> = {
	'&': '&amp;',
	'<': '&lt;',
	'"': '&quot;',
	'\r': '&#xD;',
	'\n': '&#xA;',
	'\t': '&#x9;'
};

function encodeText(value: string): string {
	return value.replace(/[&<>\r]/g, (char) => TEXT_ESCAPES[char]);
}

function encodeAttributeValue(value: string): string {
	return value.replace(/[&<"\r\n\t]/g, (char) => ATTRIBUTE_ESCAPES[char]);
}

function qualifiedName(element: Element): string {
	return element.tagName || element.nodeName;
}

/** Ordering of non-namespace attributes: unprefixed first, then by URI + name. */
function compareAttributes(a: Attr, b: Attr): number {
	if (!a.namespaceURI && b.namespaceURI) return -1;
	if (!b.namespaceURI && a.namespaceURI) return 1;
	const left = `${a.namespaceURI ?? ''}${a.localName}`;
	const right = `${b.namespaceURI ?? ''}${b.localName}`;
	return left === right ? 0 : left < right ? -1 : 1;
}

function compareNamespaces(a: NamespaceDeclaration, b: NamespaceDeclaration): number {
	return a.prefix === b.prefix ? 0 : a.prefix.localeCompare(b.prefix);
}

/**
 * Whether a prefix already counts as declared.
 *
 * Exclusive C14N re-declares a prefix that is bound to a different URI deeper
 * in the tree; inclusive C14N (as `xml-crypto` implements it, and therefore as
 * every signature this module has to check was produced) tracks the prefix
 * alone.
 */
function isPrefixInScope(
	scope: NamespaceDeclaration[],
	mode: C14nMode,
	prefix: string,
	namespaceURI: string
): boolean {
	return scope.some(
		(declaration) =>
			declaration.prefix === prefix &&
			(mode === 'inclusive' || declaration.namespaceURI === namespaceURI)
	);
}

/** The namespace declarations this element has to carry in the output. */
function renderNamespaces(
	element: Element,
	mode: C14nMode,
	prefixesInScope: NamespaceDeclaration[],
	defaultNs: string,
	ancestorNamespaces: NamespaceDeclaration[]
): { rendered: string; defaultNs: string } {
	const rendered: string[] = [];
	const toRender: NamespaceDeclaration[] = [];
	const currentNs = element.namespaceURI || '';
	let newDefaultNs = defaultNs;

	if (element.prefix) {
		if (!isPrefixInScope(prefixesInScope, mode, element.prefix, currentNs)) {
			const declaration = { prefix: element.prefix, namespaceURI: currentNs };
			toRender.push(declaration);
			prefixesInScope.push(declaration);
		}
	} else if (defaultNs !== currentNs) {
		newDefaultNs = currentNs;
		rendered.push(` xmlns="${currentNs}"`);
	}

	for (let i = 0; i < element.attributes.length; i++) {
		const attribute = element.attributes[i];

		// Inclusive C14N keeps every declaration written on the element; the
		// exclusive form only keeps the ones a prefix in use needs.
		if (
			mode === 'inclusive' &&
			attribute.prefix === 'xmlns' &&
			!isPrefixInScope(prefixesInScope, mode, attribute.localName, attribute.value)
		) {
			const declaration = { prefix: attribute.localName, namespaceURI: attribute.value };
			toRender.push(declaration);
			prefixesInScope.push(declaration);
		}

		if (
			attribute.prefix &&
			attribute.prefix !== 'xmlns' &&
			attribute.prefix !== 'xml' &&
			!isPrefixInScope(prefixesInScope, mode, attribute.prefix, attribute.namespaceURI ?? '')
		) {
			const declaration = {
				prefix: attribute.prefix,
				namespaceURI: attribute.namespaceURI ?? ''
			};
			toRender.push(declaration);
			prefixesInScope.push(declaration);
		}
	}

	for (const ancestor of ancestorNamespaces) {
		const listed = toRender.some(
			(declaration) =>
				declaration.prefix === ancestor.prefix && declaration.namespaceURI === ancestor.namespaceURI
		);
		if (!listed) toRender.push(ancestor);
	}

	toRender.sort(compareNamespaces);
	for (const declaration of toRender) {
		rendered.push(
			declaration.prefix
				? ` xmlns:${declaration.prefix}="${declaration.namespaceURI}"`
				: ` xmlns="${declaration.namespaceURI}"`
		);
	}

	return { rendered: rendered.join(''), defaultNs: newDefaultNs };
}

function renderAttributes(element: Element): string {
	const attributes: Attr[] = [];
	for (let i = 0; i < element.attributes.length; i++) {
		const attribute = element.attributes[i];
		if (attribute.name.indexOf('xmlns') === 0) continue;
		attributes.push(attribute);
	}
	attributes.sort(compareAttributes);
	return attributes
		.map((attribute) => ` ${attribute.name}="${encodeAttributeValue(attribute.value)}"`)
		.join('');
}

function canonicalizeNode(
	node: Node,
	mode: C14nMode,
	prefixesInScope: NamespaceDeclaration[],
	defaultNs: string,
	ancestorNamespaces: NamespaceDeclaration[]
): string {
	if (node.nodeType === NODE_TYPE_TEXT || node.nodeType === NODE_TYPE_CDATA) {
		return encodeText(node.nodeValue ?? '');
	}
	if (node.nodeType !== NODE_TYPE_ELEMENT) return '';

	const element = node as Element;
	const namespaces = renderNamespaces(
		element,
		mode,
		prefixesInScope,
		defaultNs,
		ancestorNamespaces
	);
	const name = qualifiedName(element);
	const parts = ['<', name, namespaces.rendered, renderAttributes(element), '>'];

	for (let i = 0; i < element.childNodes.length; i++) {
		parts.push(
			canonicalizeNode(
				element.childNodes[i],
				mode,
				prefixesInScope.slice(),
				namespaces.defaultNs,
				[]
			)
		);
	}

	parts.push('</', name, '>');
	return parts.join('');
}

function canonicalize(
	element: Element,
	mode: C14nMode,
	ancestorNamespaces: NamespaceDeclaration[] = []
): string {
	const prefixesInScope = ancestorNamespaces.map((declaration) => ({ ...declaration }));
	return canonicalizeNode(element, mode, prefixesInScope, '', ancestorNamespaces);
}

/** The prefix an element declares first, else its own. */
function ownNamespacePrefix(element: Element): string {
	for (let i = 0; i < element.attributes.length; i++) {
		const name = element.attributes[i].name;
		if (/^xmlns:?/.test(name)) return name.replace(/^xmlns:?/, '');
	}
	return element.prefix || '';
}

/**
 * Namespace declarations inherited from ancestors, nearest first, deduplicated
 * by prefix and excluding the element's own.
 *
 * Inclusive C14N re-emits these on the apex of the signed subtree, which is why
 * a GT node can only be digested where it sits. Mirrors `findAncestorNs` in
 * `xml-crypto`, and therefore `ancestorNamespaces()` in `gt.ts`.
 */
function ancestorNamespaceDeclarations(element: Element): NamespaceDeclaration[] {
	const collected: NamespaceDeclaration[] = [];
	let current: Node | null = element.parentNode;

	while (current && current.nodeType === NODE_TYPE_ELEMENT) {
		const parent = current as Element;
		for (let i = 0; i < parent.attributes.length; i++) {
			const attribute = parent.attributes[i];
			if (/^xmlns:?/.test(attribute.name)) {
				collected.push({
					prefix: attribute.name.replace(/^xmlns:?/, ''),
					namespaceURI: attribute.value || ''
				});
			}
		}
		current = parent.parentNode;
	}

	const ownPrefix = ownNamespacePrefix(element);
	const seen = new Set<string>();
	const inherited: NamespaceDeclaration[] = [];

	for (const declaration of collected) {
		if (seen.has(declaration.prefix)) continue;
		seen.add(declaration.prefix);
		if (declaration.prefix !== ownPrefix) inherited.push(declaration);
	}

	return inherited;
}

// ── DOM lookups ────────────────────────────────────────────────────────────

function getElementsByLocalName(root: Element | Document, localName: string): Element[] {
	const elements = root.getElementsByTagName('*');
	const matches: Element[] = [];
	for (let i = 0; i < elements.length; i++) {
		const element = elements[i];
		if (element.localName === localName || element.tagName === localName) {
			matches.push(element);
		}
	}
	return matches;
}

function getFirstText(root: Element | Document, localName: string): string | null {
	const match = getElementsByLocalName(root, localName)[0];
	return match?.textContent?.trim() || null;
}

/** First direct child element with the given local name, in any namespace. */
function findChildByLocalName(parent: Element, localName: string): Element | null {
	for (let i = 0; i < parent.childNodes.length; i++) {
		const child = parent.childNodes[i];
		if (child.nodeType !== NODE_TYPE_ELEMENT) continue;
		const element = child as Element;
		if (element.localName === localName || element.tagName === localName) return element;
	}
	return null;
}

/** Every `ds:Signature` in the document, preferring the XMLDSig namespace. */
function findSignatures(root: Element): Element[] {
	const all = getElementsByLocalName(root, 'Signature');
	const namespaced = all.filter((element) => element.namespaceURI === DSIG_NS);
	return namespaced.length > 0 ? namespaced : all;
}

/** Resolves a same-document `Reference URI="#id"` by `ID` / `Id` / `id`. */
function findElementById(root: Element, id: string): Element | null {
	for (const attribute of ID_ATTRIBUTES) {
		if (root.getAttribute(attribute) === id) return root;
	}
	const all = root.getElementsByTagName('*');
	for (let i = 0; i < all.length; i++) {
		const element = all[i];
		for (const attribute of ID_ATTRIBUTES) {
			if (element.getAttribute(attribute) === id) return element;
		}
	}
	return null;
}

async function sha256Base64(data: string): Promise<string> {
	const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(data));
	return bufferToBase64(digest);
}

function xmlDocumentMetadata(root: Element): XmlMetadata {
	const localName = root.localName || root.tagName;

	if (localName === 'ECF') {
		return {
			country: 'DO',
			documentId: getFirstText(root, 'eNCF'),
			documentType: getFirstText(root, 'TipoeCF')
		};
	}

	// PA: `dId` (A03) is the CUFE prefixed by "FE"; `gDGen/iDoc` (B06) the type.
	if (localName === 'rFE') {
		return {
			country: 'PA',
			documentId: getFirstText(root, 'dId'),
			documentType: getFirstText(root, 'iDoc')
		};
	}

	// GT: the authorization UUID once certified, else the issuer's access number.
	if (localName === 'GTDocumento') {
		const datosGenerales = getElementsByLocalName(root, 'DatosGenerales')[0];
		const certificacion = getElementsByLocalName(root, 'Certificacion')[0];
		const numeroAutorizacion = certificacion
			? getFirstText(certificacion, 'NumeroAutorizacion')
			: null;
		const numeroAcceso = datosGenerales?.getAttribute('NumeroAcceso')?.trim() || null;

		return {
			country: 'GT',
			documentId: numeroAutorizacion || numeroAcceso || null,
			documentType: datosGenerales?.getAttribute('Tipo')?.trim() || null
		};
	}

	const crDocTypes: Record<string, string> = {
		FacturaElectronica: '01',
		NotaDebitoElectronica: '02',
		NotaCreditoElectronica: '03',
		TiqueteElectronico: '04',
		MensajeReceptor: '05'
	};

	return {
		country: 'CR',
		documentId: getFirstText(root, 'Clave'),
		documentType: crDocTypes[localName] ?? null
	};
}

async function verifyJwt(input: string, certContent?: string): Promise<ClientVerifyResult> {
	if (!certContent) {
		return { valid: false, payload: null, error: 'Certificate is required for JWT verification' };
	}

	const parts = input.trim().split('.');
	if (parts.length !== 3) {
		return { valid: false, payload: null, error: 'Invalid JWT format' };
	}

	const [headerB64, payloadB64, signatureB64] = parts;

	try {
		const header = JSON.parse(base64urlToString(headerB64)) as Record<string, unknown>;
		if (header.alg !== 'RS512') {
			return { valid: false, payload: null, error: `Unsupported algorithm: ${header.alg}` };
		}

		const publicKey = await importRsaVerifyKey(certContent, 'SHA-512');
		const signingInput = new TextEncoder().encode(`${headerB64}.${payloadB64}`);
		const signature = base64urlToBuffer(signatureB64);
		const valid = await crypto.subtle.verify(
			'RSASSA-PKCS1-v1_5',
			publicKey,
			signature,
			signingInput
		);

		let payload: unknown;
		try {
			payload = JSON.parse(base64urlToString(payloadB64));
		} catch {
			payload = base64urlToString(payloadB64);
		}

		const payloadObj =
			typeof payload === 'object' && payload !== null ? (payload as Record<string, unknown>) : null;
		const identificacion = payloadObj?.identificacion as Record<string, unknown> | undefined;

		return {
			valid,
			payload,
			kind: 'jwt',
			country: 'SV',
			documentId:
				typeof identificacion?.codigoGeneracion === 'string'
					? identificacion.codigoGeneracion
					: null,
			documentType: typeof identificacion?.tipoDte === 'string' ? identificacion.tipoDte : null
		};
	} catch (err) {
		return {
			valid: false,
			payload: null,
			error: err instanceof Error ? err.message : 'Verification failed'
		};
	}
}

/**
 * The canonical forms of `SignedInfo` the signature may have been taken over.
 *
 * Under inclusive C14N the namespace declarations in scope belong on the
 * canonicalized `SignedInfo` — that is what `gt.ts` emits and what
 * `xml-crypto`'s own verifier reconstructs. `do.ts`, following the PHP DGII
 * reference, instead canonicalizes `SignedInfo` as if it were detached, so an
 * e-CF whose root declares anything beyond its default namespace (`xmlns:xsi`
 * is common) was signed without those declarations.
 *
 * Both are canonical forms of the same element and the RSA signature still has
 * to match one of them byte for byte, so offering both accepts either
 * convention without weakening the check.
 */
function signedInfoCandidates(signedInfo: Element, mode: C14nMode): string[] {
	if (mode === 'exclusive') return [canonicalize(signedInfo, mode)];

	const withAncestors = canonicalize(signedInfo, mode, ancestorNamespaceDeclarations(signedInfo));
	const detached = canonicalize(signedInfo, mode);
	return withAncestors === detached ? [withAncestors] : [withAncestors, detached];
}

/** The C14N flavour a `CanonicalizationMethod` / `Transform` URI selects. */
function c14nModeOf(algorithm: string, fallback: C14nMode): C14nMode {
	if (algorithm.startsWith(EXCLUSIVE_C14N_ALGO)) return 'exclusive';
	if (algorithm.startsWith(INCLUSIVE_C14N_ALGO)) return 'inclusive';
	return fallback;
}

function transformAlgorithms(reference: Element): string[] {
	return getElementsByLocalName(reference, 'Transform').map(
		(transform) => transform.getAttribute('Algorithm') || ''
	);
}

/**
 * Digest input for one `Reference`.
 *
 * `URI=""` covers the whole document minus every signature (the enveloped
 * transform for DO and PA, the `not(ancestor-or-self::ds:Signature)` XPath for
 * CR) — and, for PA, minus `gNoFirm`, which the Ficha Técnica excludes by rule
 * rather than by transform. `URI="#id"` resolves to the element carrying that
 * `ID`; it is digested where it sits, because under inclusive C14N (GT) the
 * ancestors' namespace declarations are part of its canonical form.
 */
function referenceDigestInput(
	root: Element,
	reference: Element,
	country: XmlCountry,
	mode: C14nMode
): string | null {
	const uri = reference.getAttribute('URI') ?? '';
	const enveloped = transformAlgorithms(reference).includes(ENVELOPED_TRANSFORM_ALGO);

	const target =
		uri === '' ? root : uri.startsWith('#') ? findElementById(root, uri.slice(1)) : null;
	if (!target) return null;

	const ancestorNamespaces = mode === 'inclusive' ? ancestorNamespaceDeclarations(target) : [];

	const dropSignatures = uri === '' || (enveloped && findSignatures(target).length > 0);
	const dropUnsignedGroup = country === 'PA' && target === root;
	if (!dropSignatures && !dropUnsignedGroup) {
		return canonicalize(target, mode, ancestorNamespaces);
	}

	const clone = target.cloneNode(true) as Element;
	if (dropSignatures) {
		for (const signature of findSignatures(clone)) {
			signature.parentNode?.removeChild(signature);
		}
	}
	if (dropUnsignedGroup) {
		const unsignedGroup = findChildByLocalName(clone, PA_UNSIGNED_GROUP);
		if (unsignedGroup) clone.removeChild(unsignedGroup);
	}
	stripWhitespaceNodes(clone);

	return canonicalize(clone, mode, ancestorNamespaces);
}

async function verifyXml(input: string, certContent?: string): Promise<ClientVerifyResult> {
	try {
		const doc = new DOMParser().parseFromString(input.trim(), 'application/xml');
		const root = doc.documentElement;
		if (!root) {
			return { valid: false, payload: null, error: 'Invalid XML document' };
		}

		const meta = xmlDocumentMetadata(root);
		const signatures = findSignatures(root);
		// A FEL document carries the issuer's signature and, once certified, the
		// certifier's; only the issuer's is ours to check.
		const signatureElement =
			meta.country === 'GT'
				? (signatures.find((element) => element.getAttribute('Id') === GT_ISSUER_SIGNATURE_ID) ??
					signatures[0])
				: signatures[0];
		if (!signatureElement) {
			return { valid: false, payload: null, error: 'No XML signature found' };
		}

		const signedInfoElement = getElementsByLocalName(signatureElement, 'SignedInfo')[0];
		const signatureValue = getFirstText(signatureElement, 'SignatureValue');
		if (!signedInfoElement || !signatureValue) {
			return { valid: false, payload: null, error: 'Incomplete XML signature' };
		}

		const embeddedCertB64 = getFirstText(signatureElement, 'X509Certificate');
		let effectiveCert = certContent?.trim() ?? '';
		if (!effectiveCert && embeddedCertB64) {
			effectiveCert = `-----BEGIN CERTIFICATE-----\n${embeddedCertB64}\n-----END CERTIFICATE-----`;
		}
		if (!effectiveCert) {
			return { valid: false, payload: null, error: 'Certificate is required for XML verification' };
		}

		const publicKey = await importRsaVerifyKey(effectiveCert, 'SHA-256');

		const c14nMethod = getElementsByLocalName(signedInfoElement, 'CanonicalizationMethod')[0];
		const signatureMode = c14nModeOf(c14nMethod?.getAttribute('Algorithm') || '', 'inclusive');
		const signatureBytes = base64ToBuffer(signatureValue);

		let signatureOk = false;
		for (const candidate of signedInfoCandidates(signedInfoElement, signatureMode)) {
			signatureOk = await crypto.subtle.verify(
				'RSASSA-PKCS1-v1_5',
				publicKey,
				signatureBytes,
				new TextEncoder().encode(candidate)
			);
			if (signatureOk) break;
		}

		const failure = (error: string): ClientVerifyResult => ({
			valid: false,
			payload: null,
			error,
			kind: 'xml',
			country: meta.country,
			documentId: meta.documentId,
			documentType: meta.documentType
		});

		if (!signatureOk) {
			return failure('Invalid XML signature');
		}

		const references = getElementsByLocalName(signedInfoElement, 'Reference');
		if (references.length === 0) {
			return failure('XML signature has no reference');
		}

		for (const reference of references) {
			const expectedDigest = getFirstText(reference, 'DigestValue');
			if (!expectedDigest) {
				return failure('XML reference has no digest');
			}

			const referenceMode = c14nModeOf(
				transformAlgorithms(reference).findLast(
					(algorithm) =>
						algorithm.startsWith(EXCLUSIVE_C14N_ALGO) || algorithm.startsWith(INCLUSIVE_C14N_ALGO)
				) || '',
				signatureMode
			);

			const digestInput = referenceDigestInput(root, reference, meta.country, referenceMode);
			if (digestInput === null) {
				return failure('Unresolved XML reference');
			}
			if ((await sha256Base64(digestInput)) !== expectedDigest) {
				return failure('XML digest mismatch');
			}
		}

		return {
			valid: true,
			payload: input.trim(),
			kind: 'xml',
			country: meta.country,
			documentId: meta.documentId,
			documentType: meta.documentType
		};
	} catch (err) {
		return {
			valid: false,
			payload: null,
			error: err instanceof Error ? err.message : 'Verification failed'
		};
	}
}

export async function verifySignatureClient(
	input: string,
	certContent?: string
): Promise<ClientVerifyResult> {
	return isJwt(input) ? verifyJwt(input, certContent) : verifyXml(input, certContent);
}

export function decodeJwtPayload(jwt: string): unknown {
	const parts = jwt.trim().split('.');
	if (parts.length !== 3) return null;
	try {
		return JSON.parse(base64urlToString(parts[1]));
	} catch {
		return null;
	}
}
