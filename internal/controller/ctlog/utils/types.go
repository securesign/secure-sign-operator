package utils

import rhtasv1 "github.com/securesign/operator/api/v1"

type RootCertificate []byte

type ShardConfig struct {
	TreeID         int64
	TreeLength     int64
	PublicKey      []byte
	PrivateKey     []byte
	PrivateKeyPass []byte
	Type           string // "file"
}
