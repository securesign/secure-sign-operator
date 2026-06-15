//go:build !race

package v1alpha1

import (
	"testing"

	v1 "github.com/securesign/operator/api/v1"
	utilconversion "github.com/securesign/operator/internal/testing/conversion"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/randfill"
)

func TestSecuresignConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &v1.Securesign{},
		Spoke: &Securesign{},
	}))
}

func TestCTlogConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &v1.CTlog{},
		Spoke: &CTlog{},
	}))
}

func TestRekorConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &v1.Rekor{},
		Spoke: &Rekor{},
	}))
}

func TestFulcioConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &v1.Fulcio{},
		Spoke: &Fulcio{},
		// v1alpha1 status reuses FulcioCert (spec type) which carries CommonName,
		// OrganizationName, OrganizationEmail. v1 status doesn't store these —
		// they're computed inline by the controller. The fuzzer only fills fields
		// that have a v1 counterpart.
		FuzzerFuncs: []fuzzer.FuzzerFuncs{
			func(_ runtimeserializer.CodecFactory) []interface{} {
				return []interface{}{
					func(s *FulcioStatus, c randfill.Continue) {
						c.FillNoCustom(&s.Conditions)
						c.FillNoCustom(&s.ServerConfigRef)
						c.FillNoCustom(&s.Url)
						if c.Bool() {
							s.Certificate = &FulcioCert{}
							c.FillNoCustom(&s.Certificate.PrivateKeyRef)
							c.FillNoCustom(&s.Certificate.PrivateKeyPasswordRef)
							c.FillNoCustom(&s.Certificate.CARef)
						}
					},
				}
			},
		},
	}))
}

func TestTrillianConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &v1.Trillian{},
		Spoke: &Trillian{},
		// v1alpha1 status reuses spec types (TrillianDB, TrillianLogServer) which carry
		// spec-only fields (Create, Provider, PodRequirements, etc.). The v1 status uses
		// dedicated types without these fields, so they are intentionally lost during
		// round-trip. The custom fuzzer only fills fields that have a v1 counterpart.
		FuzzerFuncs: []fuzzer.FuzzerFuncs{
			func(_ runtimeserializer.CodecFactory) []interface{} {
				return []interface{}{
					func(s *TrillianStatus, c randfill.Continue) {
						c.FillNoCustom(&s.Conditions)
						c.FillNoCustom(&s.Db.Pvc.Name)
						c.FillNoCustom(&s.Db.DatabaseSecretRef)
						c.FillNoCustom(&s.Db.TLS)
						c.FillNoCustom(&s.LogServer.TLS)
						c.FillNoCustom(&s.LogSigner.TLS)
					},
				}
			},
		},
	}))
}

func TestTufConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &v1.Tuf{},
		Spoke: &Tuf{},
	}))
}

func TestTimestampAuthorityConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &v1.TimestampAuthority{},
		Spoke: &TimestampAuthority{},
		// v1alpha1 status reuses spec types (TimestampAuthoritySigner, NTPMonitoring)
		// which carry spec-only fields (RootCA, IntermediateCA, Kms, Tink, inline NTP
		// config, etc.). The v1 status uses dedicated types without these fields, so
		// they are intentionally lost during round-trip. The custom fuzzer only fills
		// fields that have a v1 counterpart.
		FuzzerFuncs: []fuzzer.FuzzerFuncs{
			func(_ runtimeserializer.CodecFactory) []interface{} {
				return []interface{}{
					func(s *TimestampAuthorityStatus, c randfill.Continue) {
						c.FillNoCustom(&s.Conditions)
						c.FillNoCustom(&s.Url)
						if c.Bool() {
							s.Signer = &TimestampAuthoritySigner{}
							c.FillNoCustom(&s.Signer.CertificateChain.CertificateChainRef)
							if c.Bool() {
								s.Signer.File = &File{}
								c.FillNoCustom(&s.Signer.File.PasswordRef)
								c.FillNoCustom(&s.Signer.File.PrivateKeyRef)
							}
						}
						if c.Bool() {
							s.NTPMonitoring = &NTPMonitoring{}
							if c.Bool() {
								s.NTPMonitoring.Config = &NtpMonitoringConfig{
									NtpConfigRef: &LocalObjectReference{},
								}
								c.FillNoCustom(s.NTPMonitoring.Config.NtpConfigRef)
							}
						}
					},
				}
			},
		},
	}))
}
