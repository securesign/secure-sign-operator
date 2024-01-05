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
	RekorKey string
}

func CreateRekorKey() (*RekorCertConfig, error) {
	rekorCertConfig := &RekorCertConfig{}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	mKey, err := x509.MarshalECPrivateKey(key)
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

	rekorCertConfig.RekorKey = pemRekorKey.String()

	return rekorCertConfig, nil
}
