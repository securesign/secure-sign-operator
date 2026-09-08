package utils

// Reproduction scenario for the Qodo finding: the sharded config path no
// longer resolves or serializes the password for legacy encrypted PEM signing
// keys. ShardConfig still carries PrivateKeyPassword and keys.go still
// supports encrypted PEM blocks, but marshalLogConfig emits a bare
// keyspb.PEMKeyFile with no Password, so CTFE cannot open the key.

import (
	"testing"

	"github.com/google/certificate-transparency-go/trillian/ctfe/configpb"
	"github.com/google/trillian/crypto/keyspb"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/encoding/prototext"
)

func TestQodo_CreateConfig_DropsEncryptedPEMPassword(t *testing.T) {
	g := NewWithT(t)

	data, _, err := CreateConfig("trillian.default.svc:8091", []ShardConfig{
		{
			TreeID:             111,
			Prefix:             "trusted-artifact-signer",
			PublicKey:          []byte(testPublicKeyPEM),
			PrivateKey:         []byte("-----BEGIN EC PRIVATE KEY-----\nProc-Type: 4,ENCRYPTED\n\nencrypted\n-----END EC PRIVATE KEY-----\n"),
			PrivateKeyPassword: []byte("s3cr3t"),
			RootCerts:          []RootCertificate{[]byte("-----BEGIN CERTIFICATE-----\nMIIBfake\n-----END CERTIFICATE-----\n")},
		},
	})
	g.Expect(err).ToNot(HaveOccurred())

	var multi configpb.LogMultiConfig
	g.Expect(prototext.Unmarshal(data[ConfigKey], &multi)).To(Succeed())
	g.Expect(multi.LogConfigs.Config).To(HaveLen(1))

	pemKeyFile := &keyspb.PEMKeyFile{}
	g.Expect(multi.LogConfigs.Config[0].PrivateKey.UnmarshalTo(pemKeyFile)).To(Succeed())

	g.Expect(pemKeyFile.Password).To(Equal("s3cr3t"),
		"ShardConfig.PrivateKeyPassword is never serialized into keyspb.PEMKeyFile.Password, "+
			"so CTFE cannot decrypt legacy encrypted PEM signing keys")
}
