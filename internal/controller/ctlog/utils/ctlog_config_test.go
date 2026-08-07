package utils

import (
	"testing"

	"github.com/google/certificate-transparency-go/trillian/ctfe/configpb"
	"github.com/google/trillian/crypto/keyspb"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

const testPublicKeyPEM = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEbWTmxpRLKTMSTCfkVFHzzVqXWeFo
Ry5wMo4cMbp3C9GBeaqHSKbA2g9VNfhYS2Fja7P1vWpzwzCzYXGKiBAcJQ==
-----END PUBLIC KEY-----
`

func TestCreateCtlogPKCS11Config_Valid(t *testing.T) {
	g := NewWithT(t)

	rootCert := []byte("-----BEGIN CERTIFICATE-----\nMIIBfake\n-----END CERTIFICATE-----\n")
	data, err := CreateCtlogPKCS11Config(
		"trillian-logserver.default.svc:8091",
		123456,
		[]RootCertificate{rootCert},
		"PKCS11CA",
		"testpin",
		[]byte(testPublicKeyPEM),
		"trusted-artifact-signer",
	)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(data).To(HaveKey(ConfigKey))
	g.Expect(data).To(HaveKey("fulcio-0"))
	g.Expect(data["fulcio-0"]).To(Equal(rootCert))

	// Parse the protobuf config
	var multiConfig configpb.LogMultiConfig
	g.Expect(prototext.Unmarshal(data[ConfigKey], &multiConfig)).To(Succeed())
	g.Expect(multiConfig.LogConfigs.Config).To(HaveLen(1))

	logCfg := multiConfig.LogConfigs.Config[0]
	g.Expect(logCfg.LogId).To(Equal(int64(123456)))
	g.Expect(logCfg.Prefix).To(Equal("trusted-artifact-signer"))
	g.Expect(logCfg.LogBackendName).To(Equal("trillian"))
	g.Expect(logCfg.RootsPemFile).To(ConsistOf("/ctfe-keys/fulcio-0"))
	g.Expect(logCfg.PublicKey).ToNot(BeNil())
	g.Expect(logCfg.PublicKey.Der).ToNot(BeEmpty())

	// Verify PKCS#11 private key config
	g.Expect(logCfg.PrivateKey).ToNot(BeNil())
	var pkcs11Cfg keyspb.PKCS11Config
	g.Expect(proto.Unmarshal(logCfg.PrivateKey.Value, &pkcs11Cfg)).To(Succeed())
	g.Expect(pkcs11Cfg.TokenLabel).To(Equal("PKCS11CA"))
	g.Expect(pkcs11Cfg.Pin).To(Equal("testpin"))
	g.Expect(pkcs11Cfg.PublicKey).To(Equal(testPublicKeyPEM))

	// Backend
	g.Expect(multiConfig.Backends.Backend).To(HaveLen(1))
	g.Expect(multiConfig.Backends.Backend[0].BackendSpec).To(Equal("trillian-logserver.default.svc:8091"))
}

func TestCreateCtlogPKCS11Config_InvalidPEM(t *testing.T) {
	g := NewWithT(t)

	_, err := CreateCtlogPKCS11Config(
		"trillian:8091",
		1,
		nil,
		"token",
		"pin",
		[]byte("not a PEM block"),
		"prefix",
	)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("decode public key PEM"))
}

func TestCreateCtlogPKCS11Config_MultipleRootCerts(t *testing.T) {
	g := NewWithT(t)

	certs := []RootCertificate{
		[]byte("cert-0"),
		[]byte("cert-1"),
		[]byte("cert-2"),
	}
	data, err := CreateCtlogPKCS11Config(
		"trillian:8091",
		1,
		certs,
		"token",
		"pin",
		[]byte(testPublicKeyPEM),
		"prefix",
	)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(data).To(HaveKey("fulcio-0"))
	g.Expect(data).To(HaveKey("fulcio-1"))
	g.Expect(data).To(HaveKey("fulcio-2"))
	g.Expect(data["fulcio-0"]).To(Equal([]byte("cert-0")))
	g.Expect(data["fulcio-1"]).To(Equal([]byte("cert-1")))
	g.Expect(data["fulcio-2"]).To(Equal([]byte("cert-2")))

	var multiConfig configpb.LogMultiConfig
	g.Expect(prototext.Unmarshal(data[ConfigKey], &multiConfig)).To(Succeed())
	g.Expect(multiConfig.LogConfigs.Config[0].RootsPemFile).To(HaveLen(3))
}
