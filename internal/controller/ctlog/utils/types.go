package utils

import rhtasv1 "github.com/securesign/operator/api/v1"

type RootCertificate []byte

type ShardConfig struct {
	TreeID        int64
	PublicKey     []byte
	PrivateKey    []byte
	Password      []byte // Password for encrypted private key (deprecated, for legacy compatibility)
	Type          string // "file" or "pkcs11"
	Prefix        string // URL path prefix for the shard
	NotAfterStart int64  // Unix timestamp: when shard certificates become valid
	NotAfterLimit int64  // Unix timestamp: when shard certificates expire
	// PKCS11-specific fields (only used when Type == "pkcs11")
	ModulePath    string                       // Absolute path to PKCS#11 module (.so)
	TokenLabel    string                       // HSM token label
	Pin           []byte                       // HSM user PIN (from secret)
	PinSecretRef  *rhtasv1.SecretKeySelector   // Reference to HSM PIN secret
}
