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
		return nil, fmt.Errorf("failed to decode private key")
	}

	proto := configpb.LogConfig{
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

	multiConfig := configpb.LogMultiConfig{
		LogConfigs: &configpb.LogConfigSet{
			Config: []*configpb.LogConfig{&proto},
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

func CreateCtlogConfig(trillianUrl string, treeID int64, rootCerts []RootCertificate, keyConfig *KeyConfig) (map[string][]byte, error) {
	ctlogConfig := createConfigWithKeys(keyConfig)
	ctlogConfig.LogID = treeID
	ctlogConfig.LogPrefix = "trusted-artifact-signer"
	ctlogConfig.TrillianServerAddr = trillianUrl

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
