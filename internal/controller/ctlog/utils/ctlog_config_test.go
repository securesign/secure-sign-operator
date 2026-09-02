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

func TestCreateConfig_PKCS11(t *testing.T) {
	g := NewWithT(t)

	rootCert := []byte("-----BEGIN CERTIFICATE-----\nMIIBfake\n-----END CERTIFICATE-----\n")
	data, err := CreateConfig(
		"trillian-logserver.default.svc:8091",
		[]ShardConfig{
			{
				TreeID:    123456,
				Prefix:    "trusted-artifact-signer",
				PublicKey: []byte(testPublicKeyPEM),
				RootCerts: []RootCertificate{rootCert},
				PKCS11: &PKCS11ShardConfig{
					TokenLabel: "PKCS11CA",
					Pin:        "testpin",
				},
			},
		},
	)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(data).To(HaveKey(ConfigKey))
	g.Expect(data).To(HaveKey("log-123456-root-0"))
	g.Expect(data["log-123456-root-0"]).To(Equal(rootCert))

	var multiConfig configpb.LogMultiConfig
	g.Expect(prototext.Unmarshal(data[ConfigKey], &multiConfig)).To(Succeed())
	g.Expect(multiConfig.LogConfigs.Config).To(HaveLen(1))

	logCfg := multiConfig.LogConfigs.Config[0]
	g.Expect(logCfg.LogId).To(Equal(int64(123456)))
	g.Expect(logCfg.Prefix).To(Equal("trusted-artifact-signer"))
	g.Expect(logCfg.LogBackendName).To(Equal("trillian"))
	g.Expect(logCfg.RootsPemFile).To(ConsistOf("/ctfe-keys/log-123456-root-0"))
	g.Expect(logCfg.PublicKey).ToNot(BeNil())
	g.Expect(logCfg.PublicKey.Der).ToNot(BeEmpty())

	var pkcs11Cfg keyspb.PKCS11Config
	g.Expect(proto.Unmarshal(logCfg.PrivateKey.Value, &pkcs11Cfg)).To(Succeed())
	g.Expect(pkcs11Cfg.TokenLabel).To(Equal("PKCS11CA"))
	g.Expect(pkcs11Cfg.Pin).To(Equal("testpin"))
	g.Expect(pkcs11Cfg.PublicKey).To(Equal(testPublicKeyPEM))

	g.Expect(multiConfig.Backends.Backend).To(HaveLen(1))
	g.Expect(multiConfig.Backends.Backend[0].BackendSpec).To(Equal("trillian-logserver.default.svc:8091"))
}

func TestCreateConfig_InvalidPEM(t *testing.T) {
	g := NewWithT(t)

	_, err := CreateConfig(
		"trillian:8091",
		[]ShardConfig{
			{
				TreeID:    1,
				Prefix:    "prefix",
				PublicKey: []byte("not a PEM block"),
				PKCS11: &PKCS11ShardConfig{
					TokenLabel: "token",
					Pin:        "pin",
				},
			},
		},
	)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("decode public key"))
}

func TestCreateConfig_MultipleRootCerts(t *testing.T) {
	g := NewWithT(t)

	certs := []RootCertificate{
		[]byte("cert-0"),
		[]byte("cert-1"),
		[]byte("cert-2"),
	}
	data, err := CreateConfig(
		"trillian:8091",
		[]ShardConfig{
			{
				TreeID:    1,
				Prefix:    "prefix",
				PublicKey: []byte(testPublicKeyPEM),
				RootCerts: certs,
				PKCS11: &PKCS11ShardConfig{
					TokenLabel: "token",
					Pin:        "pin",
				},
			},
		},
	)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(data).To(HaveKey("log-1-root-0"))
	g.Expect(data).To(HaveKey("log-1-root-1"))
	g.Expect(data).To(HaveKey("log-1-root-2"))
	g.Expect(data["log-1-root-0"]).To(Equal([]byte("cert-0")))
	g.Expect(data["log-1-root-1"]).To(Equal([]byte("cert-1")))
	g.Expect(data["log-1-root-2"]).To(Equal([]byte("cert-2")))

	var multiConfig configpb.LogMultiConfig
	g.Expect(prototext.Unmarshal(data[ConfigKey], &multiConfig)).To(Succeed())
	g.Expect(multiConfig.LogConfigs.Config[0].RootsPemFile).To(HaveLen(3))
}

func TestCreateConfig_MultipleLogs(t *testing.T) {
	g := NewWithT(t)

	rootCert := []byte("-----BEGIN CERTIFICATE-----\nMIIBfake\n-----END CERTIFICATE-----\n")
	data, err := CreateConfig(
		"trillian:8091",
		[]ShardConfig{
			{
				TreeID:    111,
				Prefix:    "active",
				PublicKey: []byte(testPublicKeyPEM),
				RootCerts: []RootCertificate{rootCert},
				PKCS11: &PKCS11ShardConfig{
					TokenLabel: "token",
					Pin:        "pin",
				},
			},
			{
				TreeID:    222,
				Prefix:    "frozen",
				PublicKey: []byte(testPublicKeyPEM),
				Readonly:  true,
				PKCS11: &PKCS11ShardConfig{
					TokenLabel: "token2",
					Pin:        "pin2",
				},
			},
		},
	)
	g.Expect(err).ToNot(HaveOccurred())

	var multiConfig configpb.LogMultiConfig
	g.Expect(prototext.Unmarshal(data[ConfigKey], &multiConfig)).To(Succeed())
	g.Expect(multiConfig.LogConfigs.Config).To(HaveLen(2))

	g.Expect(multiConfig.LogConfigs.Config[0].LogId).To(Equal(int64(111)))
	g.Expect(multiConfig.LogConfigs.Config[0].IsReadonly).To(BeFalse())
	g.Expect(multiConfig.LogConfigs.Config[1].LogId).To(Equal(int64(222)))
	g.Expect(multiConfig.LogConfigs.Config[1].IsReadonly).To(BeTrue())

	// Frozen log without root certs inherits the active log's root cert paths
	g.Expect(multiConfig.LogConfigs.Config[1].RootsPemFile).To(ConsistOf("/ctfe-keys/log-111-root-0"))
}

func TestCreateConfig_NoLogs(t *testing.T) {
	g := NewWithT(t)
	_, err := CreateConfig("trillian:8091", nil)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("no log entries"))
}
