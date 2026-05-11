package v1alpha1

import (
	"encoding/json"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/conversion"

	v1beta1 "github.com/securesign/operator/api/v1beta1"
)

const pkcs11ConfigAnnotation = "rhtas.redhat.com/pkcs11-config"

// ConvertTo converts v1alpha1 Fulcio (spoke) to v1beta1 Fulcio (hub).
func (src *Fulcio) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.Fulcio)

	dst.ObjectMeta = src.ObjectMeta

	// Spec.Certificate: v1alpha1 has no CAType/PKCS11 fields, default to file
	dst.Spec.Certificate.CAType = v1beta1.CATypeFile
	dst.Spec.Certificate.PrivateKeyRef = convertSecretKeySelectorTo(src.Spec.Certificate.PrivateKeyRef)
	dst.Spec.Certificate.PrivateKeyPasswordRef = convertSecretKeySelectorTo(src.Spec.Certificate.PrivateKeyPasswordRef)
	dst.Spec.Certificate.CARef = convertSecretKeySelectorTo(src.Spec.Certificate.CARef)
	dst.Spec.Certificate.CommonName = src.Spec.Certificate.CommonName
	dst.Spec.Certificate.OrganizationName = src.Spec.Certificate.OrganizationName
	dst.Spec.Certificate.OrganizationEmail = src.Spec.Certificate.OrganizationEmail

	// Round-trip: restore PKCS#11 config from annotation if present
	if data, ok := src.Annotations[pkcs11ConfigAnnotation]; ok {
		var pkcs11 v1beta1.PKCS11Config
		if err := json.Unmarshal([]byte(data), &pkcs11); err != nil {
			return fmt.Errorf("unmarshalling pkcs11 config annotation: %w", err)
		}
		dst.Spec.Certificate.CAType = v1beta1.CATypePKCS11
		dst.Spec.Certificate.PKCS11 = &pkcs11
		delete(dst.Annotations, pkcs11ConfigAnnotation)
	}

	// Spec fields
	dst.Spec.ExternalAccess = convertExternalAccessTo(src.Spec.ExternalAccess)
	dst.Spec.Ctlog = convertCtlogServiceTo(src.Spec.Ctlog)
	dst.Spec.Config = convertFulcioConfigTo(src.Spec.Config)
	dst.Spec.Monitoring = convertMonitoringConfigTo(src.Spec.Monitoring)
	dst.Spec.TrustedCA = convertLocalObjectRefTo(src.Spec.TrustedCA)
	dst.Spec.Replicas = src.Spec.Replicas
	dst.Spec.Affinity = src.Spec.Affinity
	dst.Spec.Resources = src.Spec.Resources
	dst.Spec.Tolerations = src.Spec.Tolerations

	// Status
	dst.Status.ServerConfigRef = convertLocalObjectRefTo(src.Status.ServerConfigRef)
	dst.Status.Url = src.Status.Url
	if src.Status.Certificate != nil {
		dstCert := v1beta1.FulcioCert{
			PrivateKeyRef:         convertSecretKeySelectorTo(src.Status.Certificate.PrivateKeyRef),
			PrivateKeyPasswordRef: convertSecretKeySelectorTo(src.Status.Certificate.PrivateKeyPasswordRef),
			CARef:                 convertSecretKeySelectorTo(src.Status.Certificate.CARef),
			CommonName:            src.Status.Certificate.CommonName,
			OrganizationName:      src.Status.Certificate.OrganizationName,
			OrganizationEmail:     src.Status.Certificate.OrganizationEmail,
		}
		dst.Status.Certificate = &dstCert
	}
	dst.Status.Conditions = src.Status.Conditions

	return nil
}

// ConvertFrom converts v1beta1 Fulcio (hub) to v1alpha1 Fulcio (spoke).
func (dst *Fulcio) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.Fulcio)

	dst.ObjectMeta = src.ObjectMeta

	// Certificate fields
	dst.Spec.Certificate.PrivateKeyRef = convertSecretKeySelectorFrom(src.Spec.Certificate.PrivateKeyRef)
	dst.Spec.Certificate.PrivateKeyPasswordRef = convertSecretKeySelectorFrom(src.Spec.Certificate.PrivateKeyPasswordRef)
	dst.Spec.Certificate.CARef = convertSecretKeySelectorFrom(src.Spec.Certificate.CARef)
	dst.Spec.Certificate.CommonName = src.Spec.Certificate.CommonName
	dst.Spec.Certificate.OrganizationName = src.Spec.Certificate.OrganizationName
	dst.Spec.Certificate.OrganizationEmail = src.Spec.Certificate.OrganizationEmail

	// Preserve PKCS#11 config in annotation for round-trip fidelity
	if src.Spec.Certificate.CAType == v1beta1.CATypePKCS11 && src.Spec.Certificate.PKCS11 != nil {
		data, err := json.Marshal(src.Spec.Certificate.PKCS11)
		if err != nil {
			return fmt.Errorf("marshalling pkcs11 config to annotation: %w", err)
		}
		if dst.Annotations == nil {
			dst.Annotations = make(map[string]string)
		}
		dst.Annotations[pkcs11ConfigAnnotation] = string(data)
	}

	// Spec fields
	dst.Spec.ExternalAccess = convertExternalAccessFrom(src.Spec.ExternalAccess)
	dst.Spec.Ctlog = convertCtlogServiceFrom(src.Spec.Ctlog)
	dst.Spec.Config = convertFulcioConfigFrom(src.Spec.Config)
	dst.Spec.Monitoring = convertMonitoringConfigFrom(src.Spec.Monitoring)
	dst.Spec.TrustedCA = convertLocalObjectRefFrom(src.Spec.TrustedCA)
	dst.Spec.Replicas = src.Spec.Replicas
	dst.Spec.Affinity = src.Spec.Affinity
	dst.Spec.Resources = src.Spec.Resources
	dst.Spec.Tolerations = src.Spec.Tolerations

	// Status
	dst.Status.ServerConfigRef = convertLocalObjectRefFrom(src.Status.ServerConfigRef)
	dst.Status.Url = src.Status.Url
	if src.Status.Certificate != nil {
		dstCert := FulcioCert{
			PrivateKeyRef:         convertSecretKeySelectorFrom(src.Status.Certificate.PrivateKeyRef),
			PrivateKeyPasswordRef: convertSecretKeySelectorFrom(src.Status.Certificate.PrivateKeyPasswordRef),
			CARef:                 convertSecretKeySelectorFrom(src.Status.Certificate.CARef),
			CommonName:            src.Status.Certificate.CommonName,
			OrganizationName:      src.Status.Certificate.OrganizationName,
			OrganizationEmail:     src.Status.Certificate.OrganizationEmail,
		}
		dst.Status.Certificate = &dstCert
	}
	dst.Status.Conditions = src.Status.Conditions

	return nil
}

// Helpers for converting shared types between v1alpha1 and v1beta1.
// These are straightforward copies since the types are structurally identical.

func convertSecretKeySelectorTo(s *SecretKeySelector) *v1beta1.SecretKeySelector {
	if s == nil {
		return nil
	}
	return &v1beta1.SecretKeySelector{
		LocalObjectReference: v1beta1.LocalObjectReference{Name: s.Name},
		Key:                  s.Key,
	}
}

func convertSecretKeySelectorFrom(s *v1beta1.SecretKeySelector) *SecretKeySelector {
	if s == nil {
		return nil
	}
	return &SecretKeySelector{
		LocalObjectReference: LocalObjectReference{Name: s.Name},
		Key:                  s.Key,
	}
}

func convertLocalObjectRefTo(r *LocalObjectReference) *v1beta1.LocalObjectReference {
	if r == nil {
		return nil
	}
	return &v1beta1.LocalObjectReference{Name: r.Name}
}

func convertLocalObjectRefFrom(r *v1beta1.LocalObjectReference) *LocalObjectReference {
	if r == nil {
		return nil
	}
	return &LocalObjectReference{Name: r.Name}
}

func convertExternalAccessTo(ea ExternalAccess) v1beta1.ExternalAccess {
	return v1beta1.ExternalAccess{
		Enabled:             ea.Enabled,
		Host:                ea.Host,
		RouteSelectorLabels: ea.RouteSelectorLabels,
	}
}

func convertExternalAccessFrom(ea v1beta1.ExternalAccess) ExternalAccess {
	return ExternalAccess{
		Enabled:             ea.Enabled,
		Host:                ea.Host,
		RouteSelectorLabels: ea.RouteSelectorLabels,
	}
}

func convertCtlogServiceTo(c CtlogService) v1beta1.CtlogService {
	return v1beta1.CtlogService{
		Address: c.Address,
		Port:    c.Port,
		Prefix:  c.Prefix,
	}
}

func convertCtlogServiceFrom(c v1beta1.CtlogService) CtlogService {
	return CtlogService{
		Address: c.Address,
		Port:    c.Port,
		Prefix:  c.Prefix,
	}
}

func convertMonitoringConfigTo(m MonitoringConfig) v1beta1.MonitoringConfig {
	return v1beta1.MonitoringConfig{Enabled: m.Enabled}
}

func convertMonitoringConfigFrom(m v1beta1.MonitoringConfig) MonitoringConfig {
	return MonitoringConfig{Enabled: m.Enabled}
}

func convertFulcioConfigTo(c FulcioConfig) v1beta1.FulcioConfig {
	dst := v1beta1.FulcioConfig{}
	for _, i := range c.OIDCIssuers {
		dst.OIDCIssuers = append(dst.OIDCIssuers, convertOIDCIssuerTo(i))
	}
	for _, i := range c.MetaIssuers {
		dst.MetaIssuers = append(dst.MetaIssuers, convertOIDCIssuerTo(i))
	}
	for _, i := range c.CIIssuerMetadata {
		dst.CIIssuerMetadata = append(dst.CIIssuerMetadata, convertCIIssuerMetadataTo(i))
	}
	return dst
}

func convertFulcioConfigFrom(c v1beta1.FulcioConfig) FulcioConfig {
	dst := FulcioConfig{}
	for _, i := range c.OIDCIssuers {
		dst.OIDCIssuers = append(dst.OIDCIssuers, convertOIDCIssuerFrom(i))
	}
	for _, i := range c.MetaIssuers {
		dst.MetaIssuers = append(dst.MetaIssuers, convertOIDCIssuerFrom(i))
	}
	for _, i := range c.CIIssuerMetadata {
		dst.CIIssuerMetadata = append(dst.CIIssuerMetadata, convertCIIssuerMetadataFrom(i))
	}
	return dst
}

func convertOIDCIssuerTo(i OIDCIssuer) v1beta1.OIDCIssuer {
	return v1beta1.OIDCIssuer{
		IssuerURL:         i.IssuerURL,
		Issuer:            i.Issuer,
		ClientID:          i.ClientID,
		Type:              i.Type,
		CIProvider:        i.CIProvider,
		IssuerClaim:       i.IssuerClaim,
		SubjectDomain:     i.SubjectDomain,
		SPIFFETrustDomain: i.SPIFFETrustDomain,
		ChallengeClaim:    i.ChallengeClaim,
	}
}

func convertOIDCIssuerFrom(i v1beta1.OIDCIssuer) OIDCIssuer {
	return OIDCIssuer{
		IssuerURL:         i.IssuerURL,
		Issuer:            i.Issuer,
		ClientID:          i.ClientID,
		Type:              i.Type,
		CIProvider:        i.CIProvider,
		IssuerClaim:       i.IssuerClaim,
		SubjectDomain:     i.SubjectDomain,
		SPIFFETrustDomain: i.SPIFFETrustDomain,
		ChallengeClaim:    i.ChallengeClaim,
	}
}

func convertCIIssuerMetadataTo(m CIIssuerMetadata) v1beta1.CIIssuerMetadata {
	return v1beta1.CIIssuerMetadata{
		IssuerName:            m.IssuerName,
		DefaultTemplateValues: m.DefaultTemplateValues,
		ExtensionTemplates: v1beta1.Extensions{
			BuildSignerURI:                      m.ExtensionTemplates.BuildSignerURI,
			BuildSignerDigest:                   m.ExtensionTemplates.BuildSignerDigest,
			RunnerEnvironment:                   m.ExtensionTemplates.RunnerEnvironment,
			SourceRepositoryURI:                 m.ExtensionTemplates.SourceRepositoryURI,
			SourceRepositoryDigest:              m.ExtensionTemplates.SourceRepositoryDigest,
			SourceRepositoryRef:                 m.ExtensionTemplates.SourceRepositoryRef,
			SourceRepositoryIdentifier:          m.ExtensionTemplates.SourceRepositoryIdentifier,
			SourceRepositoryOwnerURI:            m.ExtensionTemplates.SourceRepositoryOwnerURI,
			SourceRepositoryOwnerIdentifier:     m.ExtensionTemplates.SourceRepositoryOwnerIdentifier,
			BuildConfigURI:                      m.ExtensionTemplates.BuildConfigURI,
			BuildConfigDigest:                   m.ExtensionTemplates.BuildConfigDigest,
			BuildTrigger:                        m.ExtensionTemplates.BuildTrigger,
			RunInvocationURI:                    m.ExtensionTemplates.RunInvocationURI,
			SourceRepositoryVisibilityAtSigning: m.ExtensionTemplates.SourceRepositoryVisibilityAtSigning,
		},
		SubjectAlternativeNameTemplate: m.SubjectAlternativeNameTemplate,
	}
}

func convertCIIssuerMetadataFrom(m v1beta1.CIIssuerMetadata) CIIssuerMetadata {
	return CIIssuerMetadata{
		IssuerName:            m.IssuerName,
		DefaultTemplateValues: m.DefaultTemplateValues,
		ExtensionTemplates: Extensions{
			BuildSignerURI:                      m.ExtensionTemplates.BuildSignerURI,
			BuildSignerDigest:                   m.ExtensionTemplates.BuildSignerDigest,
			RunnerEnvironment:                   m.ExtensionTemplates.RunnerEnvironment,
			SourceRepositoryURI:                 m.ExtensionTemplates.SourceRepositoryURI,
			SourceRepositoryDigest:              m.ExtensionTemplates.SourceRepositoryDigest,
			SourceRepositoryRef:                 m.ExtensionTemplates.SourceRepositoryRef,
			SourceRepositoryIdentifier:          m.ExtensionTemplates.SourceRepositoryIdentifier,
			SourceRepositoryOwnerURI:            m.ExtensionTemplates.SourceRepositoryOwnerURI,
			SourceRepositoryOwnerIdentifier:     m.ExtensionTemplates.SourceRepositoryOwnerIdentifier,
			BuildConfigURI:                      m.ExtensionTemplates.BuildConfigURI,
			BuildConfigDigest:                   m.ExtensionTemplates.BuildConfigDigest,
			BuildTrigger:                        m.ExtensionTemplates.BuildTrigger,
			RunInvocationURI:                    m.ExtensionTemplates.RunInvocationURI,
			SourceRepositoryVisibilityAtSigning: m.ExtensionTemplates.SourceRepositoryVisibilityAtSigning,
		},
		SubjectAlternativeNameTemplate: m.SubjectAlternativeNameTemplate,
	}
}
