package securesign

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/condition"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Verify(ctx context.Context, cli client.Client, namespace string, name string) {
	Eventually(Get).WithContext(ctx).
		WithArguments(cli, namespace, name).
		Should(
			And(
				Not(BeNil()),
				WithTransform(condition.IsReady, BeTrue()),
			))
}

func Get(ctx context.Context, cli client.Client, ns string, name string) *v1alpha1.Securesign {
	instance := &v1alpha1.Securesign{}
	if e := cli.Get(ctx, types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}, instance); errors.IsNotFound(e) {
		return nil
	}
	return instance
}

type Opts func(*v1alpha1.Securesign)

func Create(namespace, name string, opts ...Opts) *v1alpha1.Securesign {
	obj := &v1alpha1.Securesign{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				annotations.Metrics: "false",
			},
		},
	}

	for _, opt := range opts {
		opt(obj)
	}

	return obj
}

func WithDefaults() Opts {
	return func(s *v1alpha1.Securesign) {
		WithTSA()(s)
		WithGeneratedCerts()(s)
		WithManagedDatabase()(s)
		WithExternalAccess()(s)
		WithDefaultOIDC()(s)
		WithNTPMonitoring()(s)
	}
}

func WithExternalAccess() Opts {
	return func(s *v1alpha1.Securesign) {
		s.Spec.Rekor.ExternalAccess.Enabled = true
		s.Spec.Tuf.ExternalAccess.Enabled = true
		s.Spec.Fulcio.ExternalAccess.Enabled = true
		if s.Spec.TimestampAuthority != nil {
			s.Spec.TimestampAuthority.ExternalAccess.Enabled = true
		}
	}
}

func WithMonitoring() Opts {
	return func(s *v1alpha1.Securesign) {
		s.Spec.Rekor.Monitoring.Enabled = true
		s.Spec.Fulcio.Monitoring.Enabled = true
		s.Spec.Trillian.Monitoring.Enabled = true
		s.Spec.Ctlog.Monitoring.Enabled = true
		if s.Spec.TimestampAuthority != nil {
			s.Spec.TimestampAuthority.Monitoring.Enabled = true
		}
	}
}

func WithSearchUI() Opts {
	return func(s *v1alpha1.Securesign) {
		s.Spec.Rekor.RekorSearchUI.Enabled = ptr.To(true)
	}
}

func WithoutSearchUI() Opts {
	return func(s *v1alpha1.Securesign) {
		s.Spec.Rekor.RekorSearchUI.Enabled = ptr.To(false)
	}
}

func WithDefaultOIDC() Opts {
	return func(s *v1alpha1.Securesign) {
		s.Spec.Fulcio.Config = v1alpha1.FulcioConfig{
			OIDCIssuers: []v1alpha1.OIDCIssuer{
				{
					ClientID:  support.OidcClientID(),
					IssuerURL: support.OidcIssuerUrl(),
					Issuer:    support.OidcIssuerUrl(),
					Type:      "email",
				},
			}}
	}
}

func WithManagedDatabase() Opts {
	return func(s *v1alpha1.Securesign) {
		s.Spec.Trillian.Db.Create = ptr.To(true)
		s.Spec.Trillian.Db.Pvc = v1alpha1.Pvc{
			Retain: ptr.To(false),
		}
	}
}

func WithExternalDatabase(secretName string) Opts {
	return func(s *v1alpha1.Securesign) {
		s.Spec.Trillian.Db.Create = ptr.To(false)
		s.Spec.Trillian.Db.DatabaseSecretRef = &v1alpha1.LocalObjectReference{
			Name: secretName,
		}
	}
}

func WithGeneratedCerts() Opts {
	return func(s *v1alpha1.Securesign) {
		s.Spec.Fulcio.Certificate = v1alpha1.FulcioCert{
			OrganizationName:  "MyOrg",
			OrganizationEmail: "my@email.org",
			CommonName:        "fulcio",
		}

		if s.Spec.TimestampAuthority != nil {
			s.Spec.TimestampAuthority.Signer = v1alpha1.TimestampAuthoritySigner{
				CertificateChain: v1alpha1.CertificateChain{
					RootCA: &v1alpha1.TsaCertificateAuthority{
						OrganizationName:  "MyOrg",
						OrganizationEmail: "my@email.org",
						CommonName:        "tsa.hostname",
					},
					IntermediateCA: []*v1alpha1.TsaCertificateAuthority{
						{
							OrganizationName:  "MyOrg",
							OrganizationEmail: "my@email.org",
							CommonName:        "tsa.hostname",
						},
					},
					LeafCA: &v1alpha1.TsaCertificateAuthority{
						OrganizationName:  "MyOrg",
						OrganizationEmail: "my@email.org",
						CommonName:        "tsa.hostname",
					},
				},
			}
		}
	}
}

func WithProvidedCerts() Opts {
	return func(s *v1alpha1.Securesign) {
		s.Spec.Rekor.Signer = v1alpha1.RekorSigner{
			KMS: "secret",
			KeyRef: &v1alpha1.SecretKeySelector{
				LocalObjectReference: v1alpha1.LocalObjectReference{
					Name: "my-rekor-secret",
				},
				Key: "private",
			},
		}

		s.Spec.Fulcio.Certificate = v1alpha1.FulcioCert{
			PrivateKeyRef: &v1alpha1.SecretKeySelector{
				LocalObjectReference: v1alpha1.LocalObjectReference{
					Name: "my-fulcio-secret",
				},
				Key: "private",
			},
			PrivateKeyPasswordRef: &v1alpha1.SecretKeySelector{
				LocalObjectReference: v1alpha1.LocalObjectReference{
					Name: "my-fulcio-secret",
				},
				Key: "password",
			},
			CARef: &v1alpha1.SecretKeySelector{
				LocalObjectReference: v1alpha1.LocalObjectReference{
					Name: "my-fulcio-secret",
				},
				Key: "cert",
			},
		}

		s.Spec.Ctlog.PrivateKeyRef = &v1alpha1.SecretKeySelector{
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: "my-ctlog-secret",
			},
			Key: "private",
		}
		s.Spec.Ctlog.RootCertificates = []v1alpha1.SecretKeySelector{
			{
				LocalObjectReference: v1alpha1.LocalObjectReference{
					Name: "my-fulcio-secret",
				},
				Key: "cert",
			},
		}

		if s.Spec.TimestampAuthority != nil {
			s.Spec.TimestampAuthority.Signer = v1alpha1.TimestampAuthoritySigner{
				CertificateChain: v1alpha1.CertificateChain{
					CertificateChainRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "test-tsa-secret",
						},
						Key: "certificateChain",
					},
				},
				File: &v1alpha1.File{
					PrivateKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "test-tsa-secret",
						},
						Key: "leafPrivateKey",
					},
					PasswordRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "test-tsa-secret",
						},
						Key: "leafPrivateKeyPassword",
					},
				},
			}
		}
	}
}

func WithTSA() Opts {
	return func(s *v1alpha1.Securesign) {
		s.Spec.TimestampAuthority = &v1alpha1.TimestampAuthoritySpec{}
	}
}

func WithNTPMonitoring() Opts {
	return func(s *v1alpha1.Securesign) {
		if s.Spec.TimestampAuthority != nil {
			s.Spec.TimestampAuthority.NTPMonitoring = v1alpha1.NTPMonitoring{
				Enabled: true,
				Config: &v1alpha1.NtpMonitoringConfig{
					RequestAttempts: 3,
					RequestTimeout:  5,
					NumServers:      4,
					ServerThreshold: 3,
					MaxTimeDelta:    6,
					Period:          60,
					Servers:         []string{"time.apple.com", "time.google.com", "time-a-b.nist.gov", "time-b-b.nist.gov", "gbg1.ntp.se"},
				},
			}
		}
	}
}

func WithNFSPVC() Opts {
	return func(s *v1alpha1.Securesign) {
		pvcConf := v1alpha1.Pvc{
			Retain: ptr.To(false),
			Size:   ptr.To(resource.MustParse("100Mi")),
			AccessModes: []v1alpha1.PersistentVolumeAccessMode{
				"ReadWriteMany",
			},
			StorageClass: "nfs-csi",
		}

		s.Spec.Rekor.Pvc = pvcConf
		s.Spec.Tuf.Pvc = v1alpha1.TufPvc{
			Retain:       pvcConf.Retain,
			Size:         pvcConf.Size,
			AccessModes:  pvcConf.AccessModes,
			StorageClass: pvcConf.StorageClass,
		}
	}
}
