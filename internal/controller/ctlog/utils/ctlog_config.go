package utils

import (
	"encoding/pem"
	"fmt"

	"github.com/google/certificate-transparency-go/trillian/ctfe/configpb"
	"github.com/google/trillian/crypto/keyspb"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// reference code https://github.com/sigstore/scaffolding/blob/main/cmd/ctlog/createctconfig/main.go
const (
	// ConfigKey is the key in the map holding the marshalled CTLog config.
	ConfigKey = "config"
	// PrivateKey is the key in the map holding the encrypted PEM private key
	// for CTLog.
	PrivateKey = "private"
	// PublicKey is the key in the map holding the PEM public key for CTLog.
	PublicKey = "public"
	// Password is private key password
	Password = "password"

	// This is hardcoded since this is where we mount the certs in the
	// container.
	rootsPemFileDir = "/ctfe-keys/"
	// This file contains the private key for the CTLog
	privateKeyFile = "/ctfe-keys/private"
)

func CreateCtlogConfig(trillianUrl string, treeID int64, rootCerts []RootCertificate, keyConfig *KeyConfig, logPrefix string, shards []ShardConfig, notAfterStart, notAfterLimit int64) (map[string][]byte, error) {
	rootPems := make([]string, 0, len(rootCerts))
	for i := range rootCerts {
		rootPems = append(rootPems, fmt.Sprintf("%sfulcio-%d", rootsPemFileDir, i))
	}

	block, _ := pem.Decode(keyConfig.PublicKey)
	if block == nil {
		return nil, fmt.Errorf("failed to decode public key PEM")
	}

	configs := make([]*configpb.LogConfig, 0, 1+len(shards))
	for _, shard := range shards {
		shardCfg, err := marshalShardLogConfig(shard, rootPems)
		if err != nil {
			return nil, fmt.Errorf("failed to create shard config for treeID %d: %w", shard.TreeID, err)
		}
		configs = append(configs, shardCfg)
	}

	activeLog := configpb.LogConfig{
		LogId:        treeID,
		Prefix:       logPrefix,
		RootsPemFile: rootPems,
		PrivateKey: mustMarshalAny(&keyspb.PEMKeyFile{
			Path:     privateKeyFile,
			Password: string(keyConfig.PrivateKeyPass)}),
		PublicKey:      &keyspb.PublicKey{Der: block.Bytes},
		LogBackendName: "trillian",
		ExtKeyUsages:   []string{"CodeSigning"},
	}
	if notAfterStart > 0 {
		activeLog.NotAfterStart = &timestamppb.Timestamp{Seconds: notAfterStart}
	}
	if notAfterLimit > 0 {
		activeLog.NotAfterLimit = &timestamppb.Timestamp{Seconds: notAfterLimit}
	}
	configs = append(configs, &activeLog)

	marshalledConfig, err := prototext.Marshal(&configpb.LogMultiConfig{
		LogConfigs: &configpb.LogConfigSet{Config: configs},
		Backends: &configpb.LogBackendSet{
			Backend: []*configpb.LogBackend{{
				Name:        "trillian",
				BackendSpec: trillianUrl,
			}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ctlog config: %w", err)
	}

	data := map[string][]byte{
		ConfigKey:  marshalledConfig,
		PrivateKey: keyConfig.PrivateKey,
		PublicKey:  keyConfig.PublicKey,
	}
	if len(keyConfig.PrivateKeyPass) > 0 {
		data[Password] = keyConfig.PrivateKeyPass
	}
	for i, cert := range rootCerts {
		data[fmt.Sprintf("fulcio-%d", i)] = cert
	}
	for _, shard := range shards {
		if len(shard.PrivateKey) > 0 {
			data[fmt.Sprintf("shard-%d-private", shard.TreeID)] = shard.PrivateKey
		}
		for i, cert := range shard.RootCerts {
			data[fmt.Sprintf("shard-%d-root-%d", shard.TreeID, i)] = cert
		}
	}
	return data, nil
}

// CreateCtlogPKCS11Config creates a CTLog protobuf configuration for PKCS#11 mode.
// Instead of PEMKeyFile, the PrivateKey field is an anypb.Any wrapping keyspb.PKCS11Config.
// The returned map contains only the protobuf config and root certificates;
// private key material lives on the HSM and is not included.
func CreateCtlogPKCS11Config(
	trillianUrl string,
	treeID int64,
	rootCerts []RootCertificate,
	tokenLabel, pin string,
	publicKeyPEM []byte,
	logPrefix string,
	shards []ShardConfig,
	notAfterStart, notAfterLimit int64,
) (map[string][]byte, error) {
	rootPems := make([]string, 0, len(rootCerts))
	for i := range rootCerts {
		rootPems = append(rootPems, fmt.Sprintf("%sfulcio-%d", rootsPemFileDir, i))
	}

	block, _ := pem.Decode(publicKeyPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode public key PEM")
	}

	configs := make([]*configpb.LogConfig, 0, 1+len(shards))
	for _, shard := range shards {
		shardCfg, err := marshalShardLogConfig(shard, rootPems)
		if err != nil {
			return nil, fmt.Errorf("failed to create shard config for treeID %d: %w", shard.TreeID, err)
		}
		configs = append(configs, shardCfg)
	}

	activeLog := configpb.LogConfig{
		LogId:        treeID,
		Prefix:       logPrefix,
		RootsPemFile: rootPems,
		PrivateKey: mustMarshalAny(&keyspb.PKCS11Config{
			TokenLabel: tokenLabel,
			Pin:        pin,
			PublicKey:  string(publicKeyPEM),
		}),
		PublicKey:      &keyspb.PublicKey{Der: block.Bytes},
		LogBackendName: "trillian",
		ExtKeyUsages:   []string{"CodeSigning"},
	}
	if notAfterStart > 0 {
		activeLog.NotAfterStart = &timestamppb.Timestamp{Seconds: notAfterStart}
	}
	if notAfterLimit > 0 {
		activeLog.NotAfterLimit = &timestamppb.Timestamp{Seconds: notAfterLimit}
	}
	configs = append(configs, &activeLog)

	marshalledConfig, err := prototext.Marshal(&configpb.LogMultiConfig{
		LogConfigs: &configpb.LogConfigSet{Config: configs},
		Backends: &configpb.LogBackendSet{
			Backend: []*configpb.LogBackend{{
				Name:        "trillian",
				BackendSpec: trillianUrl,
			}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal PKCS#11 ctlog config: %w", err)
	}

	data := map[string][]byte{
		ConfigKey: marshalledConfig,
	}
	for i, cert := range rootCerts {
		data[fmt.Sprintf("fulcio-%d", i)] = cert
	}
	for _, shard := range shards {
		if len(shard.PrivateKey) > 0 {
			data[fmt.Sprintf("shard-%d-private", shard.TreeID)] = shard.PrivateKey
		}
		for i, cert := range shard.RootCerts {
			data[fmt.Sprintf("shard-%d-root-%d", shard.TreeID, i)] = cert
		}
	}
	return data, nil
}

func marshalShardLogConfig(shard ShardConfig, defaultRootPems []string) (*configpb.LogConfig, error) {
	block, _ := pem.Decode(shard.PublicKey)
	if block == nil {
		return nil, fmt.Errorf("failed to decode public key for shard %d", shard.TreeID)
	}

	rootPems := defaultRootPems
	if len(shard.RootCerts) > 0 {
		rootPems = make([]string, 0, len(shard.RootCerts))
		for i := range shard.RootCerts {
			rootPems = append(rootPems, fmt.Sprintf("%sshard-%d-root-%d", rootsPemFileDir, shard.TreeID, i))
		}
	}

	var privateKey *anypb.Any
	if shard.PKCS11 != nil {
		privateKey = mustMarshalAny(&keyspb.PKCS11Config{
			TokenLabel: shard.PKCS11.TokenLabel,
			Pin:        shard.PKCS11.Pin,
			PublicKey:  string(shard.PublicKey),
		})
	} else {
		shardPrivateKeyFile := fmt.Sprintf("/ctfe-keys/shard-%d-private", shard.TreeID)
		privateKey = mustMarshalAny(&keyspb.PEMKeyFile{
			Path: shardPrivateKeyFile,
		})
	}

	cfg := &configpb.LogConfig{
		LogId:          shard.TreeID,
		Prefix:         shard.Prefix,
		RootsPemFile:   rootPems,
		PrivateKey:     privateKey,
		PublicKey:      &keyspb.PublicKey{Der: block.Bytes},
		LogBackendName: "trillian",
		ExtKeyUsages:   []string{"CodeSigning"},
		IsReadonly:     shard.Readonly,
	}

	if shard.FrozenSTH != nil {
		cfg.FrozenSth = &configpb.SignedTreeHead{
			TreeSize:          shard.FrozenSTH.TreeSize,
			Timestamp:         shard.FrozenSTH.Timestamp,
			Sha256RootHash:    shard.FrozenSTH.Sha256RootHash,
			TreeHeadSignature: shard.FrozenSTH.TreeHeadSignature,
		}
	}

	if shard.NotAfterStart > 0 {
		cfg.NotAfterStart = &timestamppb.Timestamp{Seconds: shard.NotAfterStart}
	}
	if shard.NotAfterLimit > 0 {
		cfg.NotAfterLimit = &timestamppb.Timestamp{Seconds: shard.NotAfterLimit}
	}

	return cfg, nil
}

func mustMarshalAny(pb proto.Message) *anypb.Any {
	ret, err := anypb.New(pb)
	if err != nil {
		panic(fmt.Sprintf("MarshalAny failed: %v", err))
	}
	return ret
}

func IsSecretDataValid(secretData map[string][]byte, expectedTrillianAddr string) bool {
	if secretData == nil {
		return false
	}

	configData, ok := secretData[ConfigKey]
	if !ok || len(configData) == 0 {
		return false
	}

	// Parse the protobuf text format configuration
	var multiConfig configpb.LogMultiConfig
	if err := prototext.Unmarshal(configData, &multiConfig); err != nil {
		return false
	}

	if multiConfig.Backends == nil || multiConfig.Backends.Backend == nil || len(multiConfig.Backends.Backend) == 0 {
		return false
	}

	for _, backend := range multiConfig.Backends.Backend {
		if backend.BackendSpec == expectedTrillianAddr {
			return true
		}
	}

	return false
}
