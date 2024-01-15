package utils

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
)

type RekorCertConfig struct {
	RekorKey    []byte
	RekorPubKey []byte
}

func CreateRekorKey() (*RekorCertConfig, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	key.Public()

	mKey, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}

	mPubKey, err := x509.MarshalPKIXPublicKey(key.Public())
	if err != nil {
		return nil, err
	}

	var pemRekorKey bytes.Buffer
	err = pem.Encode(&pemRekorKey, &pem.Block{
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

	return &RekorCertConfig{
		RekorKey:    pemRekorKey.Bytes(),
		RekorPubKey: pemPubKey.Bytes(),
	}, nil
}
