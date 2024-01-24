package utils

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

const (
	bitSize = 4096

	curveType = "p256"
)

type PrivateKeyConfig struct {
	PrivateKey     []byte
	PrivateKeyPass []byte
	PublicKey      []byte
}

func CreatePrivateKey() (*PrivateKeyConfig, error) {
	key, err := ecdsa.GenerateKey(supportedCurves[curveType], rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	mKey, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}

	mPubKey, err := x509.MarshalPKIXPublicKey(key.Public())
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

	var pemPubKey bytes.Buffer
	err = pem.Encode(&pemPubKey, &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: mPubKey,
	})
	if err != nil {
		return nil, err
	}

	return &PrivateKeyConfig{
		PrivateKey: pemKey.Bytes(),
		PublicKey:  pemPubKey.Bytes(),
	}, nil
}
