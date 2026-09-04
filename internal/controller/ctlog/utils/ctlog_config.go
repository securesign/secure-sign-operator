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

	// This is hardcoded since this is where we mount the certs in the
	// container.
	rootsPemFileDir = "/ctfe-keys/"
)

// CreateConfig builds a CTLog protobuf MultiConfig and the accompanying secret
// data map from the resolved status log entries. Every entry is serialized
// uniformly — the Active flag plays no role here.
func CreateConfig(trillianUrl string, logs []ShardConfig) (map[string][]byte, error) {
	if len(logs) == 0 {
		return nil, fmt.Errorf("no log entries to serialize")
	}

	data := make(map[string][]byte)

	// Collect per-log root cert files and private key files into the data map.
	// Track the first non-empty set of root cert paths as the default for logs
	// that don't carry their own (inherited from the upstream resolution stage).
	var defaultRootPemPaths []string
	for _, log := range logs {
		for j, cert := range log.RootCerts {
			key := fmt.Sprintf("log-%d-root-%d", log.TreeID, j)
			data[key] = cert
		}
		if defaultRootPemPaths == nil && len(log.RootCerts) > 0 {
			defaultRootPemPaths = rootPemPaths(log.TreeID, len(log.RootCerts))
		}
		if len(log.PrivateKey) > 0 {
			data[fmt.Sprintf("log-%d-private", log.TreeID)] = log.PrivateKey
		}
	}

	configs := make([]*configpb.LogConfig, 0, len(logs))
	for _, log := range logs {
		cfg, err := marshalLogConfig(log, defaultRootPemPaths)
		if err != nil {
			return nil, fmt.Errorf("failed to create config for log %d (%s): %w", log.TreeID, log.Prefix, err)
		}
		configs = append(configs, cfg)
	}

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

	data[ConfigKey] = marshalledConfig
	return data, nil
}

func marshalLogConfig(log ShardConfig, defaultRootPems []string) (*configpb.LogConfig, error) {
	block, _ := pem.Decode(log.PublicKey)
	if block == nil {
		return nil, fmt.Errorf("failed to decode public key for log %d", log.TreeID)
	}

	rootPems := defaultRootPems
	if len(log.RootCerts) > 0 {
		rootPems = rootPemPaths(log.TreeID, len(log.RootCerts))
	}

	var privateKey *anypb.Any
	if log.PKCS11 != nil {
		privateKey = mustMarshalAny(&keyspb.PKCS11Config{
			TokenLabel: log.PKCS11.TokenLabel,
			Pin:        log.PKCS11.Pin,
			PublicKey:  string(log.PublicKey),
		})
	} else {
		privateKey = mustMarshalAny(&keyspb.PEMKeyFile{
			Path: fmt.Sprintf("%slog-%d-private", rootsPemFileDir, log.TreeID),
		})
	}

	cfg := &configpb.LogConfig{
		LogId:          log.TreeID,
		Prefix:         log.Prefix,
		RootsPemFile:   rootPems,
		PrivateKey:     privateKey,
		PublicKey:      &keyspb.PublicKey{Der: block.Bytes},
		LogBackendName: "trillian",
		ExtKeyUsages:   []string{"CodeSigning"},
		IsReadonly:     log.Readonly,
	}

	if log.FrozenSTH != nil {
		cfg.FrozenSth = &configpb.SignedTreeHead{
			TreeSize:          log.FrozenSTH.TreeSize,
			Timestamp:         log.FrozenSTH.Timestamp,
			Sha256RootHash:    log.FrozenSTH.Sha256RootHash,
			TreeHeadSignature: log.FrozenSTH.TreeHeadSignature,
		}
	}

	if log.NotAfterStart > 0 {
		cfg.NotAfterStart = &timestamppb.Timestamp{Seconds: log.NotAfterStart}
	}
	if log.NotAfterLimit > 0 {
		cfg.NotAfterLimit = &timestamppb.Timestamp{Seconds: log.NotAfterLimit}
	}

	return cfg, nil
}

func rootPemPaths(treeID int64, count int) []string {
	paths := make([]string, count)
	for i := range count {
		paths[i] = fmt.Sprintf("%slog-%d-root-%d", rootsPemFileDir, treeID, i)
	}
	return paths
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
