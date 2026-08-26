package utils

import rhtasv1 "github.com/securesign/operator/api/v1"

type RootCertificate []byte

type ShardConfig struct {
	TreeID     int64
	TreeLength int64
	PublicKey  []byte
	PrivateKey []byte
	Type       string                     // "file" or "pkcs11"
	PKCS11     *rhtasv1.CTlogPKCS11Config // PKCS#11 config for HSM-based shards
}
