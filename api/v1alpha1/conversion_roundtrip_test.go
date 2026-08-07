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
	"github.com/securesign/operator/internal/migration"
	urlfuzz "github.com/securesign/operator/internal/testing/fuzzer"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
	"sigs.k8s.io/randfill"
)

func rhtasScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = rhtasv1.AddToScheme(s)
	_ = AddToScheme(s)
	return s
}

// enabledFieldsFuzzerFuncs ensures *bool Enabled fields are never nil, matching the
// CRD defaulter's guarantee (the fuzzer bypasses the API server, so we replicate it).
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

// httpURLWithPath adapts urlfuzz.HTTPURL to randServiceReference's two-arg shape.
func httpURLWithPath(c randfill.Continue, withPort bool) string {
	return urlfuzz.HTTPURL(c, withPort, c.Bool())
}

// randServiceReference generates a v1 ServiceReference with mutually exclusive URL/Ref fields.
// URL and Ref cannot both be set; conversion picks URL when present and falls back to restoring Ref from annotation.
func randServiceReference(c randfill.Continue, urlFunc func(c randfill.Continue, withPort bool) string) rhtasv1.ServiceReference {
	switch c.Intn(3) {
	case 0:
		return rhtasv1.ServiceReference{
			URL: urlFunc(c, c.Bool()),
		}
	case 1:
		ref := &rhtasv1.ServiceReferenceRef{}
		c.FillNoCustom(ref)
		return rhtasv1.ServiceReference{Ref: ref}
	default:
		return rhtasv1.ServiceReference{}
	}
}

// randServiceReferenceWithOIDC wraps randServiceReference, adding a random OIDCIssuers entry.
func randServiceReferenceWithOIDC(c randfill.Continue, urlFunc func(c randfill.Continue, withPort bool) string) rhtasv1.ServiceRefWithOIDC {
	return rhtasv1.ServiceRefWithOIDC{
		ServiceReference: randServiceReference(c, urlFunc),
		OIDCIssuers:      []string{urlfuzz.HTTPURL(c, c.Bool(), c.Bool())},
	}
}

// grpcServiceReferenceFuzzerFuncs fuzzes v1 ServiceReference with gRPC (dns:///) URLs.
func grpcServiceReferenceFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.ServiceReference, c randfill.Continue) {
			*s = randServiceReference(c, urlfuzz.GRPCURL)
		},
	}
}

// httpServiceReferenceFuzzerFuncs fuzzes v1 ServiceReference with HTTP URLs.
func httpServiceReferenceFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.ServiceReference, c randfill.Continue) {
			*s = randServiceReference(c, httpURLWithPath)
		},
	}
}

// trillianServiceFuzzerFuncs fuzzes v1alpha1 TrillianService.Address as a gRPC target.
func trillianServiceFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *TrillianService, c randfill.Continue) {
			c.FillNoCustom(s)
			s.Address = urlfuzz.GRPCURL(c, false)
			s.Port = urlfuzz.Port(c)
		},
	}
}

func ctlogServiceFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *CtlogService, c randfill.Continue) {
			c.FillNoCustom(s)
			// Prefix already carries the path; a path in Address too doesn't
			// round-trip (the split point isn't recoverable), so keep Address bare.
			s.Address = urlfuzz.HTTPURL(c, false, false)
			s.Port = urlfuzz.Port(c)
			s.Prefix = urlfuzz.URLPath(c)
		},
	}
}

func rekorServiceFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *RekorService, c randfill.Continue) {
			c.FillNoCustom(s)
			s.Address = urlfuzz.HTTPURL(c, false, c.Bool())
			s.Port = urlfuzz.Port(c)
		},
	}
}

func tsaServiceFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *TsaService, c randfill.Continue) {
			c.FillNoCustom(s)
			s.Address = urlfuzz.HTTPURL(c, false, c.Bool())
			s.Port = urlfuzz.Port(c)
		},
	}
}

func fulcioServiceFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.ServiceRefWithOIDC, c randfill.Continue) {
			*s = randServiceReferenceWithOIDC(c, httpURLWithPath)
		},
		func(s *FulcioService, c randfill.Continue) {
			c.FillNoCustom(s)
			s.Address = urlfuzz.HTTPURL(c, false, c.Bool())
			s.Port = urlfuzz.Port(c)
		},
	}
}

// tsaSignerFuzzerFuncs constrains to one signer at a time: with multiple signers
// (Kms/Tink) their per-signer Auth merges into one v1 Auth field and can't split back.
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

// tsaCertAuthorityFuzzerFuncs clears v1alpha1 TsaCertificateAuthority.PasswordRef and PrivateKeyRef
// which have no v1 equivalent — v1 TsaCertificateAuthority only carries the three string fields.
func tsaCertAuthorityFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(ca *TsaCertificateAuthority, c randfill.Continue) {
			c.FillNoCustom(ca)
			// no v1 equivalent in TsaCertificateAuthority
			ca.PasswordRef = nil
			ca.PrivateKeyRef = nil
		},
	}
}

func securesignFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		// v1 ServiceReference fields need per-field URL schemes (gRPC for Trillian, HTTP
		// for the rest); no single type-keyed fuzzer func can tell them apart, so set them explicitly.
		func(s *rhtasv1.Securesign, c randfill.Continue) {
			c.FillNoCustom(s)
			s.Spec.Ctlog.Trillian = randServiceReference(c, urlfuzz.GRPCURL)
			s.Spec.Rekor.Trillian = randServiceReference(c, urlfuzz.GRPCURL)

			s.Spec.Tuf.Ctlog = randServiceReference(c, httpURLWithPath)
			s.Spec.Tuf.Rekor = randServiceReference(c, httpURLWithPath)
			s.Spec.Tuf.Fulcio = randServiceReferenceWithOIDC(c, httpURLWithPath)
			s.Spec.Tuf.Tsa = randServiceReference(c, httpURLWithPath)
		},
	}
}

// ctlogFuzzerFuncs constrains CTlog spec/status so Status.Url stays consistent with
// the Prefix suffix it's built from.
func ctlogFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.CTlog, c randfill.Continue) {
			c.FillNoCustom(s)
			s.Spec.Prefix = urlfuzz.URLPath(c)
			s.Status.Url = urlfuzz.HTTPURL(c, c.Bool(), false)
			if s.Status.Url != "" {
				s.Status.Url += "/" + s.Spec.Prefix
			}

		},
		func(s *CTlog, c randfill.Continue) {
			c.FillNoCustom(s)
			s.Status.Url = urlfuzz.HTTPURL(c, c.Bool(), false)
		},
	}
}

// tsaStatusFuzzerFuncs constrains the TSA status fields for proper roundtrip.
func tsaStatusFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.TimestampAuthorityStatus, c randfill.Continue) {
			c.FillNoCustom(s)
			s.Url = urlfuzz.HTTPURL(c, c.Bool(), false)
			if s.Url != "" {
				s.Url += rhtasv1.TimestampPath
			}
		},
		func(s *TimestampAuthorityStatus, c randfill.Continue) {
			c.FillNoCustom(s)
			s.Url = urlfuzz.HTTPURL(c, c.Bool(), false)
			// NTPMonitoring: only Config.NtpConfigRef survives roundtrip via v1 NtpConfigRef
			if s.NTPMonitoring != nil {
				var ref *LocalObjectReference
				if s.NTPMonitoring.Config != nil {
					ref = s.NTPMonitoring.Config.NtpConfigRef
				}
				if ref != nil {
					s.NTPMonitoring = &NTPMonitoring{Config: &NtpMonitoringConfig{NtpConfigRef: ref}}
				} else {
					s.NTPMonitoring = nil
				}
			}
			// Signer: only CertificateChain.CertificateChainRef and File survive v1 status roundtrip
			if s.Signer != nil {
				s.Signer.CertificateChain.RootCA = nil
				s.Signer.CertificateChain.IntermediateCA = nil
				s.Signer.CertificateChain.LeafCA = nil
				s.Signer.Kms = nil
				s.Signer.Tink = nil
			}
		},
	}
}

// rekorStatusFuzzerFuncs clears RekorStatus.Signer.KMS which has no v1 equivalent
// in the slim RekorSignerStatus type.
func rekorStatusFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *RekorStatus, c randfill.Continue) {
			c.FillNoCustom(s)
			// no v1 equivalent in RekorSignerStatus
			s.Signer.KMS = ""
			// no v1 equivalent — removed from v1 API
			s.RekorSearchUIUrl = ""
		},
	}
}

// fulcioStatusFuzzerFuncs clears FulcioStatus.Certificate string fields (CommonName,
// OrganizationName, OrganizationEmail) which have no v1 equivalent in FulcioCertStatus.
func fulcioStatusFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *FulcioStatus, c randfill.Continue) {
			c.FillNoCustom(s)
			if s.Certificate != nil {
				s.Certificate.CommonName = ""
				s.Certificate.OrganizationName = ""
				s.Certificate.OrganizationEmail = ""
			}
		},
	}
}

// trillianStatusFuzzerFuncs clears v1alpha1 TrillianStatus fields that only exist in the
// full spec types but not in the slim v1 status types (TrillianDBStatus, TrillianServiceStatus).
func trillianStatusFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *TrillianStatus, c randfill.Continue) {
			c.FillNoCustom(s)
			// no v1 equivalent in TrillianDBStatus
			s.Db.Create = nil
			s.Db.Pvc = Pvc{Name: s.Db.Pvc.Name}
			s.Db.Provider = ""
			s.Db.Uri = ""
			s.LogServer.Replicas = nil
			s.LogServer.Affinity = nil
			s.LogServer.Resources = nil
			s.LogServer.Tolerations = nil
			s.LogSigner.Replicas = nil
			s.LogSigner.Affinity = nil
			s.LogSigner.Resources = nil
			s.LogSigner.Tolerations = nil
		},
	}
}

// securesignStatusFuzzerFuncs constrains SecuresignStatus URL fields to valid HTTP
// URLs; v1 TSAStatus.Url also carries the TimestampPath suffix conversion adds/removes.
func securesignStatusFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.SecuresignStatus, c randfill.Continue) {
			c.FillNoCustom(s)
			s.TSAStatus.Url = urlfuzz.HTTPURL(c, c.Bool(), false)
			if s.TSAStatus.Url != "" {
				s.TSAStatus.Url += rhtasv1.TimestampPath
			}
			s.RekorStatus.Url = urlfuzz.HTTPURL(c, c.Bool(), c.Bool())
			s.FulcioStatus.Url = urlfuzz.HTTPURL(c, c.Bool(), c.Bool())
			s.TufStatus.Url = urlfuzz.HTTPURL(c, c.Bool(), c.Bool())
		},
		func(s *SecuresignStatus, c randfill.Continue) {
			c.FillNoCustom(s)
			s.TSAStatus.Url = urlfuzz.HTTPURL(c, c.Bool(), false)
			s.RekorStatus.Url = urlfuzz.HTTPURL(c, c.Bool(), c.Bool())
			s.FulcioStatus.Url = urlfuzz.HTTPURL(c, c.Bool(), c.Bool())
			s.TufStatus.Url = urlfuzz.HTTPURL(c, c.Bool(), c.Bool())
		},
	}
}

// signerVolumesFuzzerFuncs generates roundtrip-safe values for v1-only
// volume extension fields that are preserved via MarshalData/UnmarshalData annotations:
//   - FulcioSpec.{InitContainers, Volumes, VolumeMounts}
func signerVolumesFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.FulcioSpec, c randfill.Continue) {
			c.FillNoCustom(s)
			if c.Bool() {
				s.InitContainers = []rhtasv1.InitContainerSpec{
					{
						Name:    "init-" + c.String(5),
						Image:   c.String(10) + ":latest",
						Command: []string{"/bin/sh", "-c", "echo test"},
					},
				}
			} else {
				s.InitContainers = nil
			}
			if c.Bool() {
				volName := "vol-" + c.String(5)
				s.Volumes = []core.Volume{
					{
						Name: volName,
						VolumeSource: core.VolumeSource{
							ConfigMap: &core.ConfigMapVolumeSource{
								LocalObjectReference: core.LocalObjectReference{Name: "cm-" + c.String(5)},
							},
						},
					},
				}
				s.VolumeMounts = []core.VolumeMount{
					{Name: volName, MountPath: "/mnt/" + c.String(5)},
				}
			} else {
				s.Volumes = nil
				s.VolumeMounts = nil
			}
		},
	}
}

// ctlogVolumesFuzzerFuncs generates roundtrip-safe values for v1-only
// volume extension fields on CTlogSpec.
func ctlogVolumesFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.CTlogSpec, c randfill.Continue) {
			c.FillNoCustom(s)
			s.Prefix = urlfuzz.URLPath(c)
			if c.Bool() {
				s.InitContainers = []rhtasv1.InitContainerSpec{
					{
						Name:    "init-" + c.String(5),
						Image:   c.String(10) + ":latest",
						Command: []string{"/bin/sh", "-c", "echo test"},
					},
				}
			} else {
				s.InitContainers = nil
			}
			if c.Bool() {
				volName := "vol-" + c.String(5)
				s.Volumes = []core.Volume{
					{
						Name: volName,
						VolumeSource: core.VolumeSource{
							ConfigMap: &core.ConfigMapVolumeSource{
								LocalObjectReference: core.LocalObjectReference{Name: "cm-" + c.String(5)},
							},
						},
					},
				}
				s.VolumeMounts = []core.VolumeMount{
					{Name: volName, MountPath: "/mnt/" + c.String(5)},
				}
			} else {
				s.Volumes = nil
				s.VolumeMounts = nil
			}
		},
	}
}

// signerAuthFuzzerFuncs constrains FulcioSigner to valid states:
// either file (with optional File struct) or KMS (with Kms).
// Both cannot be set simultaneously — the conversion mutual exclusion guard
// clears the opposite branch based on Type. Auth is orthogonal and can be
// set for any signer type.
func signerAuthFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.FulcioSigner, c randfill.Continue) {
			c.FillNoCustom(&s.CertificateChain)
			switch c.Intn(2) {
			case 0:
				s.Type = rhtasv1.FulcioSignerTypeFile
				if c.Bool() {
					s.File = &rhtasv1.FulcioFile{}
					c.FillNoCustom(s.File)
				}
			case 1:
				s.Type = rhtasv1.FulcioSignerTypeKMS
				s.Kms = &rhtasv1.KMS{}
				c.FillNoCustom(s.Kms)
			}
			if c.Bool() {
				s.Auth = &rhtasv1.Auth{}
				c.FillNoCustom(s.Auth)
			}
		},
	}
}

// ctlogAuthFuzzerFuncs generates roundtrip-safe Auth values on CTlogSigner.
func ctlogAuthFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.CTlogSigner, c randfill.Continue) {
			s.Type = "file"
			if c.Bool() {
				s.Auth = &rhtasv1.Auth{
					Env: []core.EnvVar{
						{Name: "TEST_VAR", Value: c.String(10)},
					},
					SecretMount: []rhtasv1.SecretKeySelector{
						{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret-" + c.String(5)}, Key: "key"},
					},
				}
			} else {
				s.Auth = nil
			}
			if c.Bool() {
				s.File = &rhtasv1.CTlogFile{}
				c.FillNoCustom(&s.File.PrivateKeyRef)
				c.FillNoCustom(&s.File.PrivateKeyPasswordRef) //nolint:staticcheck
				c.FillNoCustom(&s.File.PublicKeyRef)
				if s.File.PrivateKeyRef == nil && s.File.PrivateKeyPasswordRef == nil && s.File.PublicKeyRef == nil { //nolint:staticcheck
					s.File.PrivateKeyRef = &rhtasv1.SecretKeySelector{}
					c.FillNoCustom(s.File.PrivateKeyRef)
				}
			}
		},
	}
}

// Tests

func TestSecuresignConversion(t *testing.T) {
	t.Parallel()
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: rhtasScheme(),
		Hub:    &rhtasv1.Securesign{},
		Spoke:  &Securesign{},
		HubAfterMutation: func(hub conversion.Hub) {
			migration.StripAll(hub.(*rhtasv1.Securesign))
		},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{
			securesignStatusFuzzerFuncs,
			tsaSignerFuzzerFuncs,
			tsaStatusFuzzerFuncs,
			tsaCertAuthorityFuzzerFuncs,
			trillianServiceFuzzerFuncs,
			ctlogServiceFuzzerFuncs,
			rekorServiceFuzzerFuncs,
			fulcioServiceFuzzerFuncs,
			tsaServiceFuzzerFuncs,
			securesignFuzzerFuncs,
			signerVolumesFuzzerFuncs,
			ctlogVolumesFuzzerFuncs,
			signerAuthFuzzerFuncs,
			ctlogAuthFuzzerFuncs,
			enabledFieldsFuzzerFuncs,
		},
	}))
}

func TestCTlogConversion(t *testing.T) {
	t.Parallel()
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: rhtasScheme(),
		Hub:    &rhtasv1.CTlog{},
		Spoke:  &CTlog{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{
			ctlogFuzzerFuncs,
			ctlogVolumesFuzzerFuncs,
			ctlogAuthFuzzerFuncs,
			trillianServiceFuzzerFuncs,
			grpcServiceReferenceFuzzerFuncs,
			enabledFieldsFuzzerFuncs,
		},
	}))
}

func TestRekorConversion(t *testing.T) {
	t.Parallel()
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: rhtasScheme(),
		Hub:    &rhtasv1.Rekor{},
		Spoke:  &Rekor{},
		HubAfterMutation: func(hub conversion.Hub) {
			migration.StripAll(hub.(*rhtasv1.Rekor))
		},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{
			rekorStatusFuzzerFuncs,
			trillianServiceFuzzerFuncs,
			grpcServiceReferenceFuzzerFuncs,
			enabledFieldsFuzzerFuncs,
		},
	}))
}

func TestFulcioConversion(t *testing.T) {
	t.Parallel()
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: rhtasScheme(),
		Hub:    &rhtasv1.Fulcio{},
		Spoke:  &Fulcio{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{
			fulcioStatusFuzzerFuncs,
			signerVolumesFuzzerFuncs,
			signerAuthFuzzerFuncs,
			enabledFieldsFuzzerFuncs,
		},
	}))
}

func TestTrillianConversion(t *testing.T) {
	t.Parallel()
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: rhtasScheme(),
		Hub:    &rhtasv1.Trillian{},
		Spoke:  &Trillian{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{
			trillianStatusFuzzerFuncs,
			enabledFieldsFuzzerFuncs,
		},
	}))
}

func TestTufConversion(t *testing.T) {
	t.Parallel()
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: rhtasScheme(),
		Hub:    &rhtasv1.Tuf{},
		Spoke:  &Tuf{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{
			enabledFieldsFuzzerFuncs,
			ctlogServiceFuzzerFuncs,
			rekorServiceFuzzerFuncs,
			fulcioServiceFuzzerFuncs,
			tsaServiceFuzzerFuncs,
			httpServiceReferenceFuzzerFuncs,
		},
	}))
}

func TestTimestampAuthorityConversion(t *testing.T) {
	t.Parallel()
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
