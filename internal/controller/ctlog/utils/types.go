package utils

type RootCertificate []byte

type ShardConfig struct {
	TreeID        int64
	PublicKey     []byte
	Prefix        string // URL path prefix for the shard
	NotAfterStart int64  // Unix timestamp: when shard certificates become valid
	NotAfterLimit int64  // Unix timestamp: when shard certificates expire
}
