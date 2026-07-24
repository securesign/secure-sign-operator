//go:build !race

/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"testing"

	rhtasv1 "github.com/securesign/operator/api/v1"
	utilconversion "github.com/securesign/operator/internal/conversion"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/randfill"
)

func rhtasScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = rhtasv1.AddToScheme(s)
	_ = AddToScheme(s)
	return s
}

// enabledFieldsFuzzerFuncs ensures *bool Enabled fields are never nil in fuzzed v1 hub objects.
// In production, nil is unreachable because the CRD schema defaulter always sets these fields.
// The fuzzer bypasses the API server, so we replicate that invariant here.
func enabledFieldsFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.Ingress, c randfill.Continue) {
			c.FillNoCustom(s)
			if s.Enabled == nil {
				s.Enabled = ptr.To(c.Bool())
			}
		},
		func(s *rhtasv1.MonitoringConfig, c randfill.Continue) {
			c.FillNoCustom(s)
			if s.Metrics.Enabled == nil {
				s.Metrics.Enabled = ptr.To(c.Bool())
			}
			if s.ServiceMonitor.Enabled == nil {
				s.ServiceMonitor.Enabled = ptr.To(c.Bool())
			}
		},
		func(s *rhtasv1.TlogMonitoring, c randfill.Continue) {
			c.FillNoCustom(s)
			if s.Enabled == nil {
				s.Enabled = ptr.To(c.Bool())
			}
		},
		func(s *rhtasv1.NTPMonitoring, c randfill.Continue) {
			c.FillNoCustom(s)
			if s.Enabled == nil {
				s.Enabled = ptr.To(c.Bool())
			}
		},
	}
}

func TestSecuresignConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: rhtasScheme(),
		Hub:    &rhtasv1.Securesign{},
		Spoke:  &Securesign{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{
			tsaSignerFuzzerFuncs,
			tsaStatusFuzzerFuncs,
			securesignTSAStatusFuzzerFuncs,
			tsaCertAuthorityFuzzerFuncs,
			fulcioPKCS11FuzzerFuncs,
			ctlogPKCS11FuzzerFuncs,
			enabledFieldsFuzzerFuncs,
		},
	}))
}

// securesignTSAStatusFuzzerFuncs constrains SecuresignTSAStatus.Url to a well-formed URL
// because conversion adds/removes the API suffix path.
func securesignTSAStatusFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.SecuresignTSAStatus, _ randfill.Continue) {
			// v1 Url includes the API suffix path
			s.Url = "http://tsa-server.ns.svc:3000" + rhtasv1.TimestampPath
		},
		func(s *SecuresignTSAStatus, _ randfill.Continue) {
			// v1alpha1 Url is the base host without the API suffix path
			s.Url = "http://tsa-server.ns.svc:3000"
		},
	}
}

// ctlogStatusFuzzerFuncs constrains the CTlog spec and status for proper roundtrip.
// The Url field must be a well-formed URL because conversion adds/removes the log prefix path.
// The Prefix must be fixed so the URL path matches across the roundtrip.
func ctlogStatusFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.CTlog, c randfill.Continue) {
			c.FillNoCustom(s)
			// Fix prefix to a known value so URL roundtrips correctly
			s.Spec.Prefix = "trusted-artifact-signer"
			// v1 Url includes the log prefix path
			s.Status.Url = "http://ctlog.ns.svc/" + s.Spec.Prefix
		},
		func(s *CTlog, c randfill.Continue) {
			c.FillNoCustom(s)
			// v1alpha1 Url is the base host without the log prefix path
			s.Status.Url = "http://ctlog.ns.svc"
		},
	}
}

func TestCTlogConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: rhtasScheme(),
		Hub:    &rhtasv1.CTlog{},
		Spoke:  &CTlog{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{
			ctlogStatusFuzzerFuncs,
			ctlogPKCS11FuzzerFuncs,
			enabledFieldsFuzzerFuncs,
		},
	}))
}

func TestRekorConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: rhtasScheme(),
		Hub:    &rhtasv1.Rekor{},
		Spoke:  &Rekor{},
		// Only fill fields that survive roundtrip — v1 status type RekorSignerStatus omits KMS.
		FuzzerFuncs: []fuzzer.FuzzerFuncs{
			func(_ runtimeserializer.CodecFactory) []interface{} {
				return []interface{}{
					func(s *RekorStatus, c randfill.Continue) {
						c.FillNoCustom(&s.PublicKeyRef)
						c.FillNoCustom(&s.ServerConfigRef)
						c.FillNoCustom(&s.Signer.PasswordRef)
						c.FillNoCustom(&s.Signer.KeyRef)
						c.FillNoCustom(&s.SearchIndex)
						c.FillNoCustom(&s.PvcName)
						c.FillNoCustom(&s.MonitorPvcName)
						c.FillNoCustom(&s.Url)
						c.FillNoCustom(&s.RekorSearchUIUrl)
						c.FillNoCustom(&s.TreeID)
						c.FillNoCustom(&s.Conditions)
					},
				}
			},
			enabledFieldsFuzzerFuncs,
		},
	}))
}

func TestFulcioConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: rhtasScheme(),
		Hub:    &rhtasv1.Fulcio{},
		Spoke:  &Fulcio{},
		// Only fill fields that survive roundtrip — v1 status types omit spec-only fields.
		FuzzerFuncs: []fuzzer.FuzzerFuncs{
			func(_ runtimeserializer.CodecFactory) []interface{} {
				return []interface{}{
					func(s *FulcioStatus, c randfill.Continue) {
						c.FillNoCustom(&s.Conditions)
						c.FillNoCustom(&s.Url)
						c.FillNoCustom(&s.ServerConfigRef)

						if c.Bool() {
							s.Certificate = &FulcioCert{}
							c.FillNoCustom(&s.Certificate.PrivateKeyRef)
							c.FillNoCustom(&s.Certificate.PrivateKeyPasswordRef)
							c.FillNoCustom(&s.Certificate.CARef)
						}
					},
				}
			},
			fulcioPKCS11FuzzerFuncs,
			enabledFieldsFuzzerFuncs,
		},
	}))
}

// ctlogPKCS11FuzzerFuncs constrains PKCS#11 fields for the CTLog roundtrip test.
// CTlogPKCS11Config contains deeply nested corev1 types (Volume, EnvVar, VolumeMount, etc.)
// that do not survive the JSON annotation round-trip cleanly (nil vs empty slice).
// We constrain these to use only simple scalar fields that round-trip reliably.
func ctlogPKCS11FuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.CTlogPKCS11Config, c randfill.Continue) {
			// Only fuzz the scalar/ref fields that survive JSON round-trip.
			// Omit InitContainers, Volumes, ServerEnv, ServerVolumeMounts
			// because deeply nested corev1 types have nil-vs-empty issues.
			c.FillNoCustom(&s.PinSecretRef)
			c.FillNoCustom(&s.PublicKeyRef)
			c.FillNoCustom(&s.TokenLabel)
			c.FillNoCustom(&s.PKCS11ModulePath)
			c.FillNoCustom(&s.Persistence)
		},
		func(s *rhtasv1.CTlogPKCS11Status, c randfill.Continue) {
			c.FillNoCustom(&s.PinSecretRef)
			c.FillNoCustom(&s.PublicKeyRef)
			c.FillNoCustom(&s.TokenLabel)
			c.FillNoCustom(&s.PKCS11ModulePath)
		},
	}
}

// fulcioPKCS11FuzzerFuncs constrains PKCS#11 fields for the roundtrip test.
// FulcioPKCS11Config contains deeply nested corev1 types (Volume, EnvVar, etc.)
// that do not survive the JSON annotation round-trip cleanly (nil vs empty slice).
// We constrain these to use only simple scalar fields that round-trip reliably.
func fulcioPKCS11FuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.FulcioPKCS11Config, c randfill.Continue) {
			// Only fuzz the scalar/ref fields that survive JSON round-trip.
			// Omit InitContainers, Volumes, ServerEnv, ServerVolumeMounts
			// because deeply nested corev1 types have nil-vs-empty issues.
			c.FillNoCustom(&s.CredentialsRef)
			c.FillNoCustom(&s.PKCS11ConfigRef)
			c.FillNoCustom(&s.KeyConfig)
		},
		func(s *rhtasv1.FulcioPKCS11Status, c randfill.Continue) {
			c.FillNoCustom(&s.CredentialsRef)
			c.FillNoCustom(&s.PKCS11ConfigRef)
		},
	}
}

func TestTrillianConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: rhtasScheme(),
		Hub:    &rhtasv1.Trillian{},
		Spoke:  &Trillian{},
		// Only fill fields that survive roundtrip — v1 status types omit spec-only fields.
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
			enabledFieldsFuzzerFuncs,
		},
	}))
}

func TestTufConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: rhtasScheme(),
		Hub:    &rhtasv1.Tuf{},
		Spoke:  &Tuf{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{
			enabledFieldsFuzzerFuncs,
		},
	}))
}

func TestTimestampAuthorityConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: rhtasScheme(),
		Hub:    &rhtasv1.TimestampAuthority{},
		Spoke:  &TimestampAuthority{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{
			tsaStatusFuzzerFuncs,
			tsaSignerFuzzerFuncs,
			tsaCertAuthorityFuzzerFuncs,
			enabledFieldsFuzzerFuncs,
		},
	}))
}

// tsaStatusFuzzerFuncs constrains the TSA status to only fill fields that survive roundtrip.
// The Url field must be a well-formed URL because conversion adds/removes the API suffix path.
func tsaStatusFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.TimestampAuthorityStatus, c randfill.Continue) {
			c.FillNoCustom(&s.Conditions)
			c.FillNoCustom(&s.CertificateChain)
			// v1 Url includes the API suffix path; must be a valid URL so url.Parse roundtrips correctly
			s.Url = "http://tsa-server.ns.svc:3000" + rhtasv1.TimestampPath

			if c.Bool() {
				s.NtpConfigRef = &rhtasv1.LocalObjectReference{}
				c.FillNoCustom(s.NtpConfigRef)
			}

			if c.Bool() {
				s.Signer = &rhtasv1.TimestampAuthoritySignerStatus{}
				c.FillNoCustom(&s.Signer.CertificateChainRef)
				if c.Bool() {
					s.Signer.FileSigner = &rhtasv1.FileSignerStatus{}
					c.FillNoCustom(&s.Signer.FileSigner.PrivateKeyRef)
					c.FillNoCustom(&s.Signer.FileSigner.PasswordRef)
				}
			}
		},
		func(s *TimestampAuthorityStatus, c randfill.Continue) {
			c.FillNoCustom(&s.Conditions)
			// v1alpha1 Url is the base host without the API suffix path
			s.Url = "http://tsa-server.ns.svc:3000"

			if c.Bool() {
				ref := &LocalObjectReference{}
				c.FillNoCustom(ref)
				s.NTPMonitoring = &NTPMonitoring{
					Config: &NtpMonitoringConfig{
						NtpConfigRef: ref,
					},
				}
			}

			if c.Bool() {
				s.Signer = &TimestampAuthoritySigner{}
				c.FillNoCustom(&s.Signer.CertificateChain.CertificateChainRef)
				if c.Bool() {
					s.Signer.File = &File{}
					c.FillNoCustom(&s.Signer.File.PrivateKeyRef)
					c.FillNoCustom(&s.Signer.File.PasswordRef)
				}
			}
		},
	}
}

// tsaSignerFuzzerFuncs ensures only one signer (File/Kms/Tink) is set at a time.
func tsaSignerFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *TimestampAuthoritySigner, c randfill.Continue) {
			c.FillNoCustom(&s.CertificateChain)
			switch c.Intn(3) {
			case 0:
				s.File = &File{}
				c.FillNoCustom(s.File)
			case 1:
				s.Kms = &KMS{}
				c.FillNoCustom(s.Kms)
			case 2:
				s.Tink = &Tink{}
				c.FillNoCustom(s.Tink)
			}
		},
		func(s *rhtasv1.TimestampAuthoritySigner, c randfill.Continue) {
			c.FillNoCustom(&s.CertificateChain)
			switch c.Intn(3) {
			case 0:
				s.File = &rhtasv1.File{}
				c.FillNoCustom(s.File)
			case 1:
				s.Kms = &rhtasv1.KMS{}
				c.FillNoCustom(s.Kms)
			case 2:
				s.Tink = &rhtasv1.Tink{}
				c.FillNoCustom(s.Tink)
			}
			if c.Bool() {
				s.Auth = &rhtasv1.Auth{}
				c.FillNoCustom(s.Auth)
			}
		},
	}
}

func tsaCertAuthorityFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(ca *TsaCertificateAuthority, c randfill.Continue) {
			c.FillNoCustom(&ca.CommonName)
			c.FillNoCustom(&ca.OrganizationName)
			c.FillNoCustom(&ca.OrganizationEmail)
		},
	}
}
