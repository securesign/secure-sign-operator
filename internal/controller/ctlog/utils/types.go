package utils

type RootCertificate []byte

type ShardConfig struct {
	TreeID             int64
	PublicKey          []byte
	PrivateKey         []byte
	PrivateKeyPassword []byte
	Prefix             string // URL path prefix for the shard
	NotAfterStart      int64  // Unix timestamp: when shard certificates become valid
	NotAfterLimit      int64  // Unix timestamp: when shard certificates expire
	FrozenSTH          *FrozenSTH
	Readonly           bool
	RootCerts          []RootCertificate
}

type FrozenSTH struct {
	TreeSize          int64
	Timestamp         int64
	Sha256RootHash    []byte
	TreeHeadSignature []byte
}
