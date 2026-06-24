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
	"sigs.k8s.io/randfill"
)

func rhtasScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = rhtasv1.AddToScheme(s)
	_ = AddToScheme(s)
	return s
}

func TestSecuresignConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: rhtasScheme(),
		Hub:    &rhtasv1.Securesign{},
		Spoke:  &Securesign{},
		// Exclude deprecated rekor.pvc from fuzzing
		FuzzerFuncs: []fuzzer.FuzzerFuncs{
			func(_ runtimeserializer.CodecFactory) []interface{} {
				return []interface{}{
					func(obj *SecuresignSpec, c randfill.Continue) {
						c.Fill(obj)
						// Clear the deprecated rekor.pvc field
						obj.Rekor.Pvc = Pvc{}
					},
					func(obj *rhtasv1.SecuresignSpec, c randfill.Continue) {
						c.Fill(obj)
						// Clear the deprecated rekor.pvc field in v1 as well
						obj.Rekor.Pvc = rhtasv1.Pvc{}
					},
				}
			},
		},
	}))
}

func TestCTlogConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: rhtasScheme(),
		Hub:    &rhtasv1.CTlog{},
		Spoke:  &CTlog{},
	}))
}

func TestRekorConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: rhtasScheme(),
		Hub:    &rhtasv1.Rekor{},
		Spoke:  &Rekor{},
		// Exclude deprecated spec.pvc from fuzzing as it doesn't roundtrip
		// (it's deprecated in both v1alpha1 and v1 - users should use spec.attestations.pvc)
		// Only fill status fields that survive roundtrip — v1 status type RekorSignerStatus omits KMS.
		FuzzerFuncs: []fuzzer.FuzzerFuncs{
			func(_ runtimeserializer.CodecFactory) []interface{} {
				return []interface{}{
					func(obj *RekorSpec, c randfill.Continue) {
						c.Fill(obj)
						// Clear the deprecated spec.pvc field to prevent roundtrip failures
						// Users should use spec.attestations.pvc instead
						obj.Pvc = Pvc{}
					},
					func(obj *rhtasv1.RekorSpec, c randfill.Continue) {
						c.Fill(obj)
						// Clear the deprecated spec.pvc field in v1 as well
						obj.Pvc = rhtasv1.Pvc{}
					},
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
							c.FillNoCustom(&s.Certificate.CommonName)
						}
					},
				}
			},
		},
	}))
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
		},
	}))
}

func TestTufConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: rhtasScheme(),
		Hub:    &rhtasv1.Tuf{},
		Spoke:  &Tuf{},
	}))
}

func TestTimestampAuthorityConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: rhtasScheme(),
		Hub:    &rhtasv1.TimestampAuthority{},
		Spoke:  &TimestampAuthority{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{
			func(_ runtimeserializer.CodecFactory) []interface{} {
				return []interface{}{
					func(s *TimestampAuthorityStatus, c randfill.Continue) {
						c.FillNoCustom(&s.Conditions)
						c.FillNoCustom(&s.Url)

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
			},
		},
	}))
}
