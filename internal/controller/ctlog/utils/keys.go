package utils

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

const (
	curveType = "p256"
)

var supportedCurves = map[string]elliptic.Curve{
	"p256": elliptic.P256(),
	"p384": elliptic.P384(),
	"p521": elliptic.P521(),
}

type KeyConfig struct {
	PrivateKey     []byte
	PrivateKeyPass []byte
	PublicKey      []byte
}

func CreatePrivateKey() (*KeyConfig, error) {

	key, err := ecdsa.GenerateKey(supportedCurves[curveType], rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	mKey, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}

	var pemKey bytes.Buffer
	err = pem.Encode(&pemKey, &pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: mKey,
	})
	if err != nil {
		return nil, err
	}

	mPubKey, err := x509.MarshalPKIXPublicKey(key.Public())
	if err != nil {
		return nil, err
	}

	var pemPubKey bytes.Buffer
	err = pem.Encode(&pemPubKey, &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: mPubKey,
	})
	if err != nil {
		return nil, err
	}

	return &KeyConfig{
		PrivateKey: pemKey.Bytes(),
		PublicKey:  pemPubKey.Bytes(),
	}, nil
}
