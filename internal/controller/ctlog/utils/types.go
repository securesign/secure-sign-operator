package utils

type RootCertificate []byte

type ShardConfig struct {
	TreeID        int64
	PublicKey     []byte
	PrivateKey    []byte
	PKCS11        *PKCS11ShardConfig
	Prefix        string
	NotAfterStart int64
	NotAfterLimit int64
	FrozenSTH     *FrozenSTH
	Readonly      bool
	RootCerts     []RootCertificate
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
