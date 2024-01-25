package utils

import (
	"bytes"
	"context"
	"crypto"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/google/certificate-transparency-go/trillian/ctfe/configpb"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/securesign/operator/controllers/common"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	corev1 "k8s.io/api/core/v1"
	"knative.dev/pkg/logging"
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
	PrivKey         crypto.PrivateKey
	PrivKeyPassword string
	PubKey          crypto.PublicKey
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
func (c *Config) AddRootCertificate(ctx context.Context, root RootCertificate) error {
	for _, fc := range c.RootCerts {
		if bytes.Equal(fc, root) {
			//TODO: improve logging
			logging.FromContext(ctx).Info("Found existing root certificate, not adding")
			return nil
		}
	}
	//TODO: improve logging
	logging.FromContext(ctx).Info("Adding new root certificate")
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
func (c *Config) MarshalConfig(ctx context.Context) (map[string][]byte, error) {
	// Since we can have multiple Fulcio secrets, we need to construct a set
	// of files containing them for the RootsPemFile. Names don't matter
	// so we just call them fulcio-%
	// What matters however is to ensure that the filenames match the keys
	// in the configmap / secret that we construct so they get properly mounted.
	rootPems := make([]string, 0, len(c.RootCerts))
	for i := range c.RootCerts {
		rootPems = append(rootPems, fmt.Sprintf("%sfulcio-%d", rootsPemFileDir, i))
	}

	var pubkey crypto.Signer
	var ok bool
	// Note this goofy cast to crypto.Signer since the any interface has no
	// methods so cast here so that we get the Public method which all core
	// keys support.
	if pubkey, ok = c.PrivKey.(crypto.Signer); !ok {
		logging.FromContext(ctx).Fatalf("Failed to convert private key to crypto.Signer")
	}
	keyDER, err := x509.MarshalPKIXPublicKey(pubkey.Public())
	if err != nil {
		logging.FromContext(ctx).Panicf("Failed to marshal the public key: %v", err)
	}
	proto := configpb.LogConfig{
		LogId:        c.LogID,
		Prefix:       c.LogPrefix,
		RootsPemFile: rootPems,
		PrivateKey: mustMarshalAny(&keyspb.PEMKeyFile{
			Path:     privateKeyFile,
			Password: c.PrivKeyPassword}),
		PublicKey:      &keyspb.PublicKey{Der: keyDER},
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
	secrets, err := c.marshalSecrets()
	if err != nil {
		return nil, err
	}
	secrets[ConfigKey] = marshalledConfig
	return secrets, nil
}

// MarshalSecrets returns a map suitable for creating a secret out of
// containing the following keys:
// private - CTLog private key, PEM encoded and encrypted with the password
// public - CTLog public key, PEM encoded
// fulcio-%d - For each fulcioCerts, contains one entry so we can support
// multiple.
func (c *Config) marshalSecrets() (map[string][]byte, error) {
	// Encode private key to PKCS #8 ASN.1 PEM.
	marshalledPrivKey, err := x509.MarshalPKCS8PrivateKey(c.PrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	block := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: marshalledPrivKey,
	}
	// Encrypt the pem
	encryptedBlock, err := x509.EncryptPEMBlock(rand.Reader, block.Type, block.Bytes, []byte(c.PrivKeyPassword), x509.PEMCipherAES256) // nolint
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt private key: %w", err)
	}

	privPEM := pem.EncodeToMemory(encryptedBlock)
	if privPEM == nil {
		return nil, fmt.Errorf("failed to encode encrypted private key")
	}
	// Encode public key to PKIX ASN.1 PEM.
	var pubkey crypto.Signer
	var ok bool

	// Note this goofy cast to crypto.Signer since the any interface has no
	// methods so cast here so that we get the Public method which all core
	// keys support.
	if pubkey, ok = c.PrivKey.(crypto.Signer); !ok {
		return nil, fmt.Errorf("failed to convert private key to crypto.Signer")
	}

	marshalledPubKey, err := x509.MarshalPKIXPublicKey(pubkey.Public())
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}
	pubPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: marshalledPubKey,
		},
	)
	data := map[string][]byte{
		PrivateKey: privPEM,
		PublicKey:  pubPEM,
	}
	for i, cert := range c.RootCerts {
		fulcioKey := fmt.Sprintf("fulcio-%d", i)
		data[fulcioKey] = cert
	}
	return data, nil
}

func mustMarshalAny(pb proto.Message) *anypb.Any {
	ret, err := anypb.New(pb)
	if err != nil {
		panic(fmt.Sprintf("MarshalAny failed: %v", err))
	}
	return ret
}

func createConfigWithKeys(certConfig *PrivateKeyConfig) (*Config, error) {
	var signer crypto.Signer
	var privKey crypto.PrivateKey
	var err error
	var ok bool

	if certConfig.PrivateKey == nil {
		return nil, fmt.Errorf("failed to get private key")
	}

	block, _ := pem.Decode(certConfig.PrivateKey)
	if block == nil {
		return nil, fmt.Errorf("failed to decode private key")
	}
	if x509.IsEncryptedPEMBlock(block) {
		if certConfig.PrivateKeyPass == nil {
			return nil, fmt.Errorf("failed to get private key password")
		}
		block.Bytes, err = x509.DecryptPEMBlock(block, certConfig.PrivateKeyPass)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt private key: %w", err)
		}
	}

	privKey, err = x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		privKey, err = x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			privKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("failed to parse private key: %w", err)
			}
		}
	}

	if signer, ok = privKey.(crypto.Signer); !ok {
		return nil, fmt.Errorf("failed to convert to crypto.Signer")
	} else {
		return &Config{
			PrivKey: privKey,
			PubKey:  signer.Public(),
		}, nil
	}
}

func CreateCtlogConfig(ctx context.Context, ns string, trillianUrl string, treeID int64, rootCerts []RootCertificate, labels map[string]string, keyConfig *PrivateKeyConfig) (*corev1.Secret, error) {
	ctlogConfig, err := createConfigWithKeys(keyConfig)
	if err != nil {
		return nil, err
	}

	if keyConfig.PrivateKeyPass == nil {
		ctlogConfig.PrivKeyPassword = string(common.GeneratePassword(8))
	} else {
		ctlogConfig.PrivKeyPassword = string(keyConfig.PrivateKeyPass)
	}

	ctlogConfig.LogID = treeID
	ctlogConfig.LogPrefix = "trusted-artifact-signer"
	ctlogConfig.TrillianServerAddr = trillianUrl

	for _, cert := range rootCerts {
		if err = ctlogConfig.AddRootCertificate(ctx, cert); err != nil {
			return nil, fmt.Errorf("Failed to add fulcio root: %v", err)
		}
	}

	configMap, err := ctlogConfig.MarshalConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("Failed to marshal ctlog config: %v", err)
	}

	config := kubernetes.CreateSecret("ctlog-secret", ns, configMap, labels)

	return config, nil
}
