package utils

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/url"

	"github.com/google/certificate-transparency-go/trillian/ctfe/configpb"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/sigstore/sigstore/pkg/cryptoutils"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	corev1 "k8s.io/api/core/v1"
	"knative.dev/pkg/logging"

	fulcioclient "github.com/sigstore/fulcio/pkg/api"
)

// reference code https://github.com/sigstore/scaffolding/blob/main/cmd/ctlog/createctconfig/main.go
const (
	bitSize = 4096

	curveType = "p256"
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

	// FulcioCerts contains one or more Root certificates for Fulcio.
	// It may contain more than one if Fulcio key is rotated for example, so
	// there will be a period of time when we allow both. It might also contain
	// multiple Root Certificates, if we choose to support admitting certificates from fulcio instances run by others
	FulcioCerts [][]byte
}

func extractFulcioRoot(fulcioRoot []byte) ([]byte, error) {
	// Fetch only root certificate from the chain
	certs, err := cryptoutils.UnmarshalCertificatesFromPEM(fulcioRoot)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal certficate chain: %w", err)
	}
	return cryptoutils.MarshalCertificateToPEM(certs[len(certs)-1])
}

// AddFulcioRoot will add the specified fulcioRoot to the list of trusted
// Fulcios. If it already exists, it's a nop.
// The fulcioRoot should come from the call to fetch a PublicFulcio root
// and is the ChainPEM from the fulcioclient RootResponse.
func (c *Config) AddFulcioRoot(ctx context.Context, fulcioRoot []byte) error {
	root, err := extractFulcioRoot(fulcioRoot)
	if err != nil {
		return fmt.Errorf("extracting fulcioRoot: %w", err)
	}
	for _, fc := range c.FulcioCerts {
		if bytes.Equal(fc, root) {
			//TODO: improve logging
			logging.FromContext(ctx).Info("Found existing fulcio root, not adding")
			return nil
		}
	}
	//TODO: improve logging
	logging.FromContext(ctx).Info("Adding new FulcioRoot")
	c.FulcioCerts = append(c.FulcioCerts, root)
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
	rootPems := make([]string, 0, len(c.FulcioCerts))
	for i := range c.FulcioCerts {
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
	for i, cert := range c.FulcioCerts {
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

func createConfigWithKeys(ctx context.Context, keytype string, privateKey []byte) (*Config, error) {
	var signer crypto.Signer
	var privKey crypto.PrivateKey
	var ok bool

	if privateKey == nil {
		var err error
		if keytype == "rsa" {
			privKey, err = rsa.GenerateKey(rand.Reader, bitSize)
			if err != nil {
				return nil, fmt.Errorf("failed to generate Private RSA Key: %w", err)
			}
		} else {
			privKey, err = ecdsa.GenerateKey(supportedCurves[curveType], rand.Reader)
			if err != nil {
				return nil, fmt.Errorf("failed to generate Private ECDSA Key: %w", err)
			}
		}
		if signer, ok = privKey.(crypto.Signer); !ok {
			return nil, fmt.Errorf("failed to convert to Signer")
		}
		return &Config{
			PrivKey: privKey,
			PubKey:  signer.Public(),
		}, nil
	}

	block, rest := pem.Decode([]byte(privateKey))
	if block == nil || block.Type != "EC PARAMETERS" {
		return nil, fmt.Errorf("failed to decode EC PARAMETERS")
	}

	block, _ = pem.Decode(rest)
	if block == nil || block.Type != "EC PRIVATE KEY" {
		return nil, fmt.Errorf("failed to decode private key")
	}

	privKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %s", err)
	}
	if signer, ok = privKey.(crypto.Signer); !ok {
		return nil, fmt.Errorf("failed to convert to Signer")
	}
	return &Config{
		PrivKey: privKey,
		PubKey:  signer.Public(),
	}, nil
}

func CreateCtlogConfig(ctx context.Context, ns string, trillianUrl string, treeID int64, fulcioUrl string, labels map[string]string, privateKey []byte) (*corev1.Secret, *corev1.Secret, error) {
	u, err := url.Parse(fulcioUrl)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid fulcioURL %s : %v", fulcioUrl, err)
	}
	client := fulcioclient.NewClient(u)
	root, err := client.RootCert()
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to fetch fulcio Root cert: %w", err)
	}

	ctlogConfig, err := createConfigWithKeys(ctx, "ecdsa", privateKey)
	if err != nil {
		return nil, nil, err
	}
	ctlogConfig.PrivKeyPassword = "test"
	ctlogConfig.LogID = treeID
	ctlogConfig.LogPrefix = "trusted-artifact-signer"
	ctlogConfig.TrillianServerAddr = trillianUrl

	if err = ctlogConfig.AddFulcioRoot(ctx, root.ChainPEM); err != nil {
		return nil, nil, fmt.Errorf("Failed to add fulcio root: %v", err)
	}
	configMap, err := ctlogConfig.MarshalConfig(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to marshal ctlog config: %v", err)
	}

	config := kubernetes.CreateSecret("ctlog-secret", ns, configMap, labels)

	pubData := map[string][]byte{PublicKey: configMap[PublicKey]}
	pubKeySecret := kubernetes.CreateSecret("ctlog-public-key", ns, pubData, labels)

	return config, pubKeySecret, nil
}
