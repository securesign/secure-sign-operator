package utils

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
)

type RootCertificate []byte

type ShardConfig struct {
	TreeID                int64
	PublicKey             []byte
	PrivateKey            []byte
	PrivateKeyPassword    []byte
	PrivateKeyPasswordRef *rhtasv1.SecretKeySelector
	PKCS11                *PKCS11ShardConfig
	Prefix                string
	NotAfterStart         int64
	NotAfterLimit         int64
	FrozenSTH             *FrozenSTH
	Readonly              bool
	Mirror                bool
	RootCerts             []RootCertificate
}

type PKCS11ShardConfig struct {
	TokenLabel string
	Pin        string
}

type FrozenSTH struct {
	TreeSize          int64
	Timestamp         int64
	Sha256RootHash    []byte
	TreeHeadSignature []byte
}
