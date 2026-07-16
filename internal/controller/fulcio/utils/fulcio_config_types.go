package utils

import rhtasv1 "github.com/securesign/operator/api/v1"

type FulcioServerConfig struct {
	OIDCIssuers      map[string]OIDCIssuer       `yaml:"oidc-issuers"`
	MetaIssuers      map[string]OIDCIssuer       `yaml:"meta-issuers"`
	CIIssuerMetadata map[string]CIIssuerMetadata `yaml:"ci-issuer-metadata"`
}

type OIDCIssuer struct {
	IssuerURL         string `yaml:"issuer-url,omitempty"`
	Issuer            string `yaml:"issuer"`
	ClientID          string `yaml:"client-id"`
	Type              string `yaml:"type"`
	CIProvider        string `yaml:"ci-provider,omitempty"`
	IssuerClaim       string `yaml:"issuer-claim,omitempty"`
	SubjectDomain     string `yaml:"subject-domain,omitempty"`
	SPIFFETrustDomain string `yaml:"spiffe-trust-domain,omitempty"`
	ChallengeClaim    string `yaml:"challenge-claim,omitempty"`
}

type CIIssuerMetadata struct {
	IssuerName                     string            `yaml:"issuer-name"`
	DefaultTemplateValues          map[string]string `yaml:"default-template-values,omitempty"`
	ExtensionTemplates             Extensions        `yaml:"extension-templates,omitempty"`
	SubjectAlternativeNameTemplate string            `yaml:"subject-alternative-name-template,omitempty"`
}

type Extensions struct {
	BuildSignerURI                      string `yaml:"build-signer-uri,omitempty"`
	BuildSignerDigest                   string `yaml:"build-signer-digest,omitempty"`
	RunnerEnvironment                   string `yaml:"runner-environment,omitempty"`
	SourceRepositoryURI                 string `yaml:"source-repository-uri,omitempty"`
	SourceRepositoryDigest              string `yaml:"source-repository-digest,omitempty"`
	SourceRepositoryRef                 string `yaml:"source-repository-ref,omitempty"`
	SourceRepositoryIdentifier          string `yaml:"source-repository-identifier,omitempty"`
	SourceRepositoryOwnerURI            string `yaml:"source-repository-owner-uri,omitempty"`
	SourceRepositoryOwnerIdentifier     string `yaml:"source-repository-owner-identifier,omitempty"`
	BuildConfigURI                      string `yaml:"build-config-uri,omitempty"`
	BuildConfigDigest                   string `yaml:"build-config-digest,omitempty"`
	BuildTrigger                        string `yaml:"build-trigger,omitempty"`
	RunInvocationURI                    string `yaml:"run-invocation-uri,omitempty"`
	SourceRepositoryVisibilityAtSigning string `yaml:"source-repository-visibility-at-signing,omitempty"`
}

func NewFulcioServerConfig(fulcioConfig rhtasv1.FulcioConfig) *FulcioServerConfig {
	oidcIssuers := make(map[string]OIDCIssuer)
	metaIssuers := make(map[string]OIDCIssuer)
	ciIssuerMetadata := make(map[string]CIIssuerMetadata)

	for _, issuer := range fulcioConfig.OIDCIssuers {
		oidcIssuers[issuer.Issuer] = convertOIDCIssuer(issuer)
	}

	for _, issuer := range fulcioConfig.MetaIssuers {
		metaIssuers[issuer.Issuer] = convertOIDCIssuer(issuer)
	}

	for _, metadata := range fulcioConfig.CIIssuerMetadata {
		ciIssuerMetadata[metadata.IssuerName] = convertCIIssuerMetadata(metadata)
	}

	return &FulcioServerConfig{
		OIDCIssuers:      oidcIssuers,
		MetaIssuers:      metaIssuers,
		CIIssuerMetadata: ciIssuerMetadata,
	}
}

func convertOIDCIssuer(in rhtasv1.OIDCIssuer) OIDCIssuer {
	return OIDCIssuer{
		IssuerURL:         in.IssuerURL,
		Issuer:            in.Issuer,
		ClientID:          in.ClientID,
		Type:              in.Type,
		CIProvider:        in.CIProvider,
		IssuerClaim:       in.IssuerClaim,
		SubjectDomain:     in.SubjectDomain,
		SPIFFETrustDomain: in.SPIFFETrustDomain,
		ChallengeClaim:    in.ChallengeClaim,
	}
}

func convertCIIssuerMetadata(in rhtasv1.CIIssuerMetadata) CIIssuerMetadata {
	return CIIssuerMetadata{
		IssuerName:                     in.IssuerName,
		DefaultTemplateValues:          in.DefaultTemplateValues,
		ExtensionTemplates:             convertExtensions(in.ExtensionTemplates),
		SubjectAlternativeNameTemplate: in.SubjectAlternativeNameTemplate,
	}
}

func convertExtensions(in rhtasv1.Extensions) Extensions {
	return Extensions{
		BuildSignerURI:                      in.BuildSignerURI,
		BuildSignerDigest:                   in.BuildSignerDigest,
		RunnerEnvironment:                   in.RunnerEnvironment,
		SourceRepositoryURI:                 in.SourceRepositoryURI,
		SourceRepositoryDigest:              in.SourceRepositoryDigest,
		SourceRepositoryRef:                 in.SourceRepositoryRef,
		SourceRepositoryIdentifier:          in.SourceRepositoryIdentifier,
		SourceRepositoryOwnerURI:            in.SourceRepositoryOwnerURI,
		SourceRepositoryOwnerIdentifier:     in.SourceRepositoryOwnerIdentifier,
		BuildConfigURI:                      in.BuildConfigURI,
		BuildConfigDigest:                   in.BuildConfigDigest,
		BuildTrigger:                        in.BuildTrigger,
		RunInvocationURI:                    in.RunInvocationURI,
		SourceRepositoryVisibilityAtSigning: in.SourceRepositoryVisibilityAtSigning,
	}
}
