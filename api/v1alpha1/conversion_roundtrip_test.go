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
	"fmt"
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

// randUrlPath generates a URL-safe path
func randUrlPath(c randfill.Continue) string {
	if c.Bool() {
		return fmt.Sprintf("path-%d/sub-%d", c.Intn(100), c.Intn(100))
	}
	return fmt.Sprintf("path-%d", c.Intn(100))
}

// randHttpUrl generates a valid HTTP URL for status fields that go through url.Parse
// in urlWithPath/urlWithoutPath during conversion.
func randHttpUrl(c randfill.Continue, withPort bool) string {
	if c.Bool() {
		return ""
	}
	u := fmt.Sprintf("http://svc-%d.ns.svc", c.Intn(1000))
	if withPort {
		u += fmt.Sprintf(":%d", c.Intn(65534)+1)
	}
	return u
}

// randGrpcUrl generates a valid gRPC target URI (dns:/// scheme) for Trillian service references.
// Trillian uses gRPC, not HTTP — the address format must survive TrillianService ↔ ServiceReference conversion.
func randGrpcUrl(c randfill.Continue, withPort bool) string {
	if c.Bool() {
		return ""
	}
	u := fmt.Sprintf("dns:///svc-%d.ns.svc", c.Intn(1000))
	if withPort {
		u += fmt.Sprintf(":%d", c.Intn(65534)+1)
	}
	return u
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

// randServiceReference generates a v1 ServiceReference with mutually exclusive URL/Ref fields.
// URL and Ref cannot both be set; conversion picks URL when present and falls back to restoring Ref from annotation.
func randServiceReferenceWithOIDC(c randfill.Continue, urlFunc func(c randfill.Continue, withPort bool) string) rhtasv1.ServiceRefWithOIDC {
	return rhtasv1.ServiceRefWithOIDC{
		ServiceReference: randServiceReference(c, urlFunc),
		OIDCIssuers:      []string{randHttpUrl(c, c.Bool())},
	}
}

// trillianServiceFuzzerFuncs constrains both sides of the TrillianService ↔ ServiceReference
// conversion to values that survive the roundtrip:
//   - v1 ServiceReference: URL and Ref are mutually exclusive; URL must be valid "host:port"
//   - v1alpha1 TrillianService.Address must not contain colons (ambiguous with port separator)
func trillianServiceFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.ServiceReference, c randfill.Continue) {
			*s = randServiceReference(c, randGrpcUrl)
		},
		func(s *TrillianService, c randfill.Continue) {
			c.FillNoCustom(s)
			s.Address = randGrpcUrl(c, false)
			// empty address means use autoconfiguration (resolve from ref or autodiscover) - port is irrelevant and it will be dropped during conversion
			if s.Address == "" {
				s.Port = nil
			} else {
				s.Port = ptr.To(int32(c.Intn(65534) + 1))
			}
		},
	}
}

func ctlogServiceFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.ServiceReference, c randfill.Continue) {
			*s = randServiceReference(c, randHttpUrl)
		},
		func(s *CtlogService, c randfill.Continue) {
			c.FillNoCustom(s)
			s.Address = randHttpUrl(c, false)
			// empty address means use autoconfiguration (resolve from ref or autodiscover) - port and prefix are irrelevant and they will be dropped during conversion
			if s.Address == "" {
				s.Port = nil
				s.Prefix = ""
			} else {
				s.Port = ptr.To(int32(c.Intn(65534) + 1))
				s.Prefix = randUrlPath(c)
			}
		},
	}
}

func rekorServiceFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.ServiceReference, c randfill.Continue) {
			*s = randServiceReference(c, randHttpUrl)
		},
		func(s *RekorService, c randfill.Continue) {
			c.FillNoCustom(s)
			s.Address = randHttpUrl(c, false)
			// empty address means use autoconfiguration (resolve from ref or autodiscover) - port is irrelevant and it will be dropped during conversion
			if s.Address == "" {
				s.Port = nil
			} else {
				s.Port = ptr.To(int32(c.Intn(65534) + 1))
			}
		},
	}
}

func tsaServiceFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.ServiceReference, c randfill.Continue) {
			*s = randServiceReference(c, randHttpUrl)
		},
		func(s *TsaService, c randfill.Continue) {
			c.FillNoCustom(s)
			s.Address = randHttpUrl(c, false)
			// empty address means use autoconfiguration (resolve from ref or autodiscover) - port is irrelevant and it will be dropped during conversion
			if s.Address == "" {
				s.Port = nil
			} else {
				s.Port = ptr.To(int32(c.Intn(65534) + 1))
			}
		},
	}
}

func fulcioServiceFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.ServiceReference, c randfill.Continue) {
			*s = randServiceReference(c, randHttpUrl)
		},
		func(s *rhtasv1.ServiceRefWithOIDC, c randfill.Continue) {
			*s = randServiceReferenceWithOIDC(c, randHttpUrl)
		},
		func(s *FulcioService, c randfill.Continue) {
			c.FillNoCustom(s)
			s.Address = randHttpUrl(c, false)
			// empty address means use autoconfiguration (resolve from ref or autodiscover) - port is irrelevant and it will be dropped during conversion
			if s.Address == "" {
				s.Port = nil
			} else {
				s.Port = ptr.To(int32(c.Intn(65534) + 1))
			}
		},
	}
}

// tsaSignerFuzzerFuncs constrains to one signer at a time — not a validation rule,
// but a conversion limitation: if multiple signers (Kms/Tink) are configured, their
// per-signer Auth is merged into a single v1 Auth field and we can't split it back.
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
		// http url vs grpc url fuzzers are figting between for ServiceReference fuzzer funcs - do not use them and overwrite
		func(s *rhtasv1.Securesign, c randfill.Continue) {
			c.FillNoCustom(s)
			s.Spec.Ctlog.Trillian = randServiceReference(c, randGrpcUrl)
			s.Spec.Rekor.Trillian = randServiceReference(c, randGrpcUrl)

			s.Spec.Tuf.Ctlog = randServiceReference(c, randHttpUrl)
			s.Spec.Tuf.Rekor = randServiceReference(c, randHttpUrl)
			s.Spec.Tuf.Fulcio = randServiceReferenceWithOIDC(c, randHttpUrl)
			s.Spec.Tuf.Tsa = randServiceReference(c, randHttpUrl)
		},
	}
}

// ctlogFuzzerFuncs constrains the CTlog spec and status for proper roundtrip.
// We need to fuzz the whole CTlog object because the URL - suffix of the URL - is stored in the status.
func ctlogFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.CTlog, c randfill.Continue) {
			c.FillNoCustom(s)
			s.Spec.Prefix = randUrlPath(c)
			s.Status.Url = randHttpUrl(c, c.Bool())
			if s.Status.Url != "" {
				s.Status.Url += "/" + s.Spec.Prefix
			}

		},
		func(s *CTlog, c randfill.Continue) {
			c.FillNoCustom(s)
			s.Status.Url = randHttpUrl(c, c.Bool())
		},
	}
}

// tsaStatusFuzzerFuncs constrains the TSA status fields for proper roundtrip.
func tsaStatusFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.TimestampAuthorityStatus, c randfill.Continue) {
			c.FillNoCustom(s)
			s.Url = randHttpUrl(c, c.Bool())
			if s.Url != "" {
				s.Url += rhtasv1.TimestampPath
			}
		},
		func(s *TimestampAuthorityStatus, c randfill.Continue) {
			c.FillNoCustom(s)
			s.Url = randHttpUrl(c, c.Bool())
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

// securesignStatusFuzzerFuncs constrains SecuresignStatus URL fields to valid HTTP URLs.
// Status URLs go through url.Parse in urlWithPath/urlWithoutPath during conversion;
// v1 TSAStatus.Url includes the TimestampPath suffix that conversion adds/removes.
func securesignStatusFuzzerFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		func(s *rhtasv1.SecuresignStatus, c randfill.Continue) {
			c.FillNoCustom(s)
			s.TSAStatus.Url = randHttpUrl(c, c.Bool())
			if s.TSAStatus.Url != "" {
				s.TSAStatus.Url += rhtasv1.TimestampPath
			}
			s.RekorStatus.Url = randHttpUrl(c, c.Bool())
			s.FulcioStatus.Url = randHttpUrl(c, c.Bool())
			s.TufStatus.Url = randHttpUrl(c, c.Bool())
		},
		func(s *SecuresignStatus, c randfill.Continue) {
			c.FillNoCustom(s)
			s.TSAStatus.Url = randHttpUrl(c, c.Bool())
			s.RekorStatus.Url = randHttpUrl(c, c.Bool())
			s.FulcioStatus.Url = randHttpUrl(c, c.Bool())
			s.TufStatus.Url = randHttpUrl(c, c.Bool())
		},
	}
}

// Tests

func TestSecuresignConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: rhtasScheme(),
		Hub:    &rhtasv1.Securesign{},
		Spoke:  &Securesign{},
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
			enabledFieldsFuzzerFuncs,
		},
	}))
}

func TestCTlogConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: rhtasScheme(),
		Hub:    &rhtasv1.CTlog{},
		Spoke:  &CTlog{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{
			ctlogFuzzerFuncs,
			trillianServiceFuzzerFuncs,
			enabledFieldsFuzzerFuncs,
		},
	}))
}

func TestRekorConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: rhtasScheme(),
		Hub:    &rhtasv1.Rekor{},
		Spoke:  &Rekor{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{
			rekorStatusFuzzerFuncs,
			trillianServiceFuzzerFuncs,
			enabledFieldsFuzzerFuncs,
		},
	}))
}

func TestFulcioConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: rhtasScheme(),
		Hub:    &rhtasv1.Fulcio{},
		Spoke:  &Fulcio{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{
			fulcioStatusFuzzerFuncs,
			enabledFieldsFuzzerFuncs,
		},
	}))
}

func TestTrillianConversion(t *testing.T) {
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
