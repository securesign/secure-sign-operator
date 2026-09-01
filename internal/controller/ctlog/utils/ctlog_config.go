package utils

import (
	"bytes"
	"crypto/elliptic"
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

var supportedCurves = map[string]elliptic.Curve{
	"p256": elliptic.P256(),
	"p384": elliptic.P384(),
	"p521": elliptic.P521(),
}

// Config abstracts the proto munging to/from bytes suitable for working
// with secrets / configmaps. Note that we keep fulcioCerts here though
// technically they are not part of the config, however because we create a
// secret/CM that we then mount, they need to be synced.
type Config struct {
	PrivKey         []byte
	PrivKeyPassword []byte
	PubKey          []byte
	LogID           int64
	LogPrefix       string

	// Address of the gRPC Trillian Admin Server (host:port)
	TrillianServerAddr string

	// RootCerts contains one or more Root certificates that are acceptable to the log.
	// It may contain more than one if Fulcio key is rotated for example, so
	// there will be a period of time when we allow both. It might also contain
	// multiple Root Certificates, if we choose to support admitting certificates from fulcio instances run by others
	RootCerts []RootCertificate

	Shards []ShardConfig

	// NotAfterStart is the Unix timestamp when the active log's certificates become valid
	NotAfterStart int64
	// NotAfterLimit is the Unix timestamp when the active log's certificates expire
	NotAfterLimit int64
}

// AddRootCertificate will add the specified root certificate to truststore.
// If it already exists, it's a nop. The fulcio root cert should come from
// the call to fetch a PublicFulcio root and is the ChainPEM from the
// fulcioclient RootResponse.
func (c *Config) AddRootCertificate(root RootCertificate) error {
	for _, fc := range c.RootCerts {
		if bytes.Equal(fc, root) {
			return nil
		}
	}
	c.RootCerts = append(c.RootCerts, root)
	return nil
}

// MarshalConfig marshals the CTLogConfig into a format that can be handed
// to the CTLog in form of a secret or configmap. Returns a map with the
// following keys:
// config - CTLog configuration
// private - CTLog private key, PEM encoded and encrypted with the password
// public - CTLog public key, PEM encoded
// fulcio-%d - For each fulcioCerts, contains one entry so we can support
// multiple.
func (c *Config) MarshalConfig() ([]byte, error) {
	// Since we can have multiple Fulcio secrets, we need to construct a set
	// of files containing them for the RootsPemFile. Names don't matter
	// so we just call them fulcio-%
	// What matters however is to ensure that the filenames match the keys
	// in the configmap / secret that we construct so they get properly mounted.
	rootPems := make([]string, 0, len(c.RootCerts))
	for i := range c.RootCerts {
		rootPems = append(rootPems, fmt.Sprintf("%sfulcio-%d", rootsPemFileDir, i))
	}

	block, _ := pem.Decode(c.PubKey)
	if block == nil {
		return nil, fmt.Errorf("failed to decode public key PEM")
	}

	configs := make([]*configpb.LogConfig, 0, 1+len(c.Shards))

	for _, shard := range c.Shards {
		shardCfg, err := c.marshalShardLogConfig(shard, rootPems)
		if err != nil {
			return nil, fmt.Errorf("failed to create shard config for treeID %d: %w", shard.TreeID, err)
		}
		configs = append(configs, shardCfg)
	}

	activeLog := configpb.LogConfig{
		LogId:        c.LogID,
		Prefix:       c.LogPrefix,
		RootsPemFile: rootPems,
		PrivateKey: mustMarshalAny(&keyspb.PEMKeyFile{
			Path:     privateKeyFile,
			Password: string(c.PrivKeyPassword)}),
		PublicKey:      &keyspb.PublicKey{Der: block.Bytes},
		LogBackendName: "trillian",
		ExtKeyUsages:   []string{"CodeSigning"},
	}

	// Set certificate validity timestamps if provided (as Unix seconds)
	if c.NotAfterStart > 0 {
		activeLog.NotAfterStart = &timestamppb.Timestamp{Seconds: c.NotAfterStart}
	}
	if c.NotAfterLimit > 0 {
		activeLog.NotAfterLimit = &timestamppb.Timestamp{Seconds: c.NotAfterLimit}
	}

	configs = append(configs, &activeLog)

	multiConfig := configpb.LogMultiConfig{
		LogConfigs: &configpb.LogConfigSet{
			Config: configs,
		},
		Backends: &configpb.LogBackendSet{
			Backend: []*configpb.LogBackend{{
				Name:        "trillian",
				BackendSpec: c.TrillianServerAddr,
			}},
		},
	}
	marshalledConfig, err := prototext.Marshal(&multiConfig)
	if err != nil {
		return nil, err
	}
	return marshalledConfig, nil
}

func (c *Config) marshalShardLogConfig(shard ShardConfig, defaultRootPems []string) (*configpb.LogConfig, error) {
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

	// Set certificate validity timestamps if provided (as Unix seconds)
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

func createConfigWithKeys(certConfig *KeyConfig) *Config {
	return &Config{
		PubKey:          certConfig.PublicKey,
		PrivKey:         certConfig.PrivateKey,
		PrivKeyPassword: certConfig.PrivateKeyPass,
	}
}

func CreateCtlogConfig(trillianUrl string, treeID int64, rootCerts []RootCertificate, keyConfig *KeyConfig, logPrefix string, shards []ShardConfig, notAfterStart, notAfterLimit int64) (map[string][]byte, error) {
	ctlogConfig := createConfigWithKeys(keyConfig)
	ctlogConfig.LogID = treeID
	ctlogConfig.LogPrefix = logPrefix
	ctlogConfig.TrillianServerAddr = trillianUrl
	ctlogConfig.Shards = shards
	ctlogConfig.NotAfterStart = notAfterStart
	ctlogConfig.NotAfterLimit = notAfterLimit

	for _, cert := range rootCerts {
		if err := ctlogConfig.AddRootCertificate(cert); err != nil {
			return nil, fmt.Errorf("failed to add fulcio root: %v", err)
		}
	}

	config, err := ctlogConfig.MarshalConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ctlog config: %v", err)
	}

	data := map[string][]byte{
		ConfigKey:  config,
		PrivateKey: ctlogConfig.PrivKey,
		PublicKey:  ctlogConfig.PubKey,
	}
	if len(ctlogConfig.PrivKeyPassword) > 0 {
		data[Password] = ctlogConfig.PrivKeyPassword
	}
	for i, cert := range ctlogConfig.RootCerts {
		fulcioKey := fmt.Sprintf("fulcio-%d", i)
		data[fulcioKey] = cert
	}
	for _, shard := range ctlogConfig.Shards {
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
) (map[string][]byte, error) {
	rootPems := make([]string, 0, len(rootCerts))
	for i := range rootCerts {
		rootPems = append(rootPems, fmt.Sprintf("%sfulcio-%d", rootsPemFileDir, i))
	}

	block, _ := pem.Decode(publicKeyPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode public key PEM")
	}

	logConfig := configpb.LogConfig{
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

	multiConfig := configpb.LogMultiConfig{
		LogConfigs: &configpb.LogConfigSet{
			Config: []*configpb.LogConfig{&logConfig},
		},
		Backends: &configpb.LogBackendSet{
			Backend: []*configpb.LogBackend{{
				Name:        "trillian",
				BackendSpec: trillianUrl,
			}},
		},
	}
	marshalledConfig, err := prototext.Marshal(&multiConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal PKCS#11 ctlog config: %w", err)
	}

	data := map[string][]byte{
		ConfigKey: marshalledConfig,
	}
	for i, cert := range rootCerts {
		data[fmt.Sprintf("fulcio-%d", i)] = cert
	}
	return data, nil
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
		// Failed to parse - invalid configuration
		return false
	}

	// Validate that at least one backend exists
	if multiConfig.Backends == nil || multiConfig.Backends.Backend == nil || len(multiConfig.Backends.Backend) == 0 {
		return false
	}

	// Check if any backend matches the expected Trillian address
	for _, backend := range multiConfig.Backends.Backend {
		if backend.BackendSpec == expectedTrillianAddr {
			return true
		}
	}

	return false
}
