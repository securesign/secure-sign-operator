package utils

type RootCertificate []byte

type ShardConfig struct {
	TreeID     int64
	TreeLength int64
	PublicKey  []byte
	PrivateKey []byte
	Password   []byte // Password for encrypted private key (deprecated, for legacy compatibility)
	Type       string // "file" or "pkcs11"
}
