// Package xmldsig builds and checks enveloped XML signatures (XMLDSig) the
// way Sello Sign's countries make them, over etree documents read and written
// as the TypeScript signers did with @xmldom/xmldom (Parse, Serialize), and
// canonicalized with the c14n package. Its DOM (Parse, Serialize,
// SerializeAround, the escaping, StripWhitespace and the lookups) is also the
// one the XAdES countries use.
package xmldsig
