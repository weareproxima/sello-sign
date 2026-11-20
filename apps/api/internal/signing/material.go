package signing

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
)

// EncodeMaterial writes material as a PEM bundle: the key as PKCS#8, then
// the certificate and its chain. It is what the API encrypts and stores.
func EncodeMaterial(m Material) ([]byte, error) {
	if m.Key == nil {
		return nil, errors.New("signing: material without a key")
	}
	der, err := x509.MarshalPKCS8PrivateKey(m.Key)
	if err != nil {
		return nil, fmt.Errorf("signing: marshal key: %w", err)
	}
	var b bytes.Buffer
	if err := pem.Encode(&b, &pem.Block{Type: "PRIVATE KEY", Bytes: der}); err != nil {
		return nil, err
	}
	certs := m.Chain
	if m.Certificate != nil {
		certs = append([]*x509.Certificate{m.Certificate}, m.Chain...)
	}
	for _, c := range certs {
		if err := pem.Encode(&b, &pem.Block{Type: "CERTIFICATE", Bytes: c.Raw}); err != nil {
			return nil, err
		}
	}
	return b.Bytes(), nil
}

// DecodeMaterial reads what EncodeMaterial wrote. The first certificate is
// the signer's; the rest, its chain. Material with a chain but no signer
// certificate isn't written by EncodeMaterial.
func DecodeMaterial(data []byte) (Material, error) {
	var m Material
	for {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			break
		}
		switch block.Type {
		case "PRIVATE KEY":
			key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err != nil {
				return Material{}, fmt.Errorf("signing: parse key: %w", err)
			}
			signer, ok := key.(crypto.Signer)
			if !ok {
				return Material{}, errors.New("signing: the key cannot sign")
			}
			m.Key = signer
		case "CERTIFICATE":
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return Material{}, fmt.Errorf("signing: parse certificate: %w", err)
			}
			if m.Certificate == nil {
				m.Certificate = cert
			} else {
				m.Chain = append(m.Chain, cert)
			}
		}
	}
	if m.Key == nil {
		return Material{}, errors.New("signing: material without a key")
	}
	return m, nil
}
