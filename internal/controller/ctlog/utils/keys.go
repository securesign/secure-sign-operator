package utils

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

const (
	curveType = "p256"
)

type KeyConfig struct {
	PrivateKey     []byte
	PrivateKeyPass []byte
	PublicKey      []byte
}

func (k KeyConfig) ToMap() map[string][]byte {
	return map[string][]byte{
		"private":  k.PrivateKey,
		"public":   k.PublicKey,
		"password": k.PrivateKeyPass,
	}
}

func CreatePrivateKey(password []byte) (*KeyConfig, error) {

	key, err := ecdsa.GenerateKey(supportedCurves[curveType], rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	mKey, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}

	var block *pem.Block
	if len(password) > 0 {
		block, err = x509.EncryptPEMBlock(rand.Reader, "EC PRIVATE KEY", mKey, password, x509.PEMCipherAES256) //nolint:staticcheck
	} else {
		block = &pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: mKey,
		}
	}
	if err != nil {
		return nil, err
	}
	var pemKey bytes.Buffer
	err = pem.Encode(&pemKey, block)
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
		PrivateKey:     pemKey.Bytes(),
		PublicKey:      pemPubKey.Bytes(),
		PrivateKeyPass: password,
	}, nil
}

func GeneratePublicKey(certConfig *KeyConfig) (*KeyConfig, error) {
	var signer crypto.Signer
	var priv crypto.PrivateKey
	var err error
	var ok bool

	privatePEMBlock, _ := pem.Decode(certConfig.PrivateKey)
	if privatePEMBlock == nil {
		return nil, fmt.Errorf("failed to decode private key")
	}

	if x509.IsEncryptedPEMBlock(privatePEMBlock) { //nolint:staticcheck
		if certConfig.PrivateKeyPass == nil {
			return nil, fmt.Errorf("can't find private key password")
		}
		privatePEMBlock.Bytes, err = x509.DecryptPEMBlock(privatePEMBlock, certConfig.PrivateKeyPass) //nolint:staticcheck
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt private key: %w", err)
		}
	}

	if priv, err = x509.ParsePKCS8PrivateKey(privatePEMBlock.Bytes); err != nil {
		// Try it as RSA
		if priv, err = x509.ParsePKCS1PrivateKey(privatePEMBlock.Bytes); err != nil {
			if priv, err = x509.ParseECPrivateKey(privatePEMBlock.Bytes); err != nil {
				return nil, fmt.Errorf("failed to parse private key PEM: %w", err)
			}
		}
	}

	if signer, ok = priv.(crypto.Signer); !ok {
		return nil, fmt.Errorf("failed to convert to crypto.Signer")
	}

	mPubKey, err := x509.MarshalPKIXPublicKey(signer.Public())
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
	certConfig.PublicKey = pemPubKey.Bytes()
	return certConfig, nil
}
