package securesign

import (
	"context"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
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

func Get(ctx context.Context, cli client.Client, ns string, name string) *rhtasv1.Securesign {
	instance := &rhtasv1.Securesign{}
	if e := cli.Get(ctx, types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}, instance); errors.IsNotFound(e) {
		return nil
	}
	return instance
}

type Opts func(*rhtasv1.Securesign)

func Create(namespace, name string, opts ...Opts) *rhtasv1.Securesign {
	obj := &rhtasv1.Securesign{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	for _, opt := range opts {
		opt(obj)
	}

	return obj
}

func WithDefaults() Opts {
	return func(s *rhtasv1.Securesign) {
		WithTSA()(s)
		WithGeneratedCerts()(s)
		WithManagedDatabase()(s)
		WithExternalAccess()(s)
		WithDefaultOIDC()(s)
		WithNTPMonitoring()(s)
	}
}

func WithExternalAccess() Opts {
	return func(s *rhtasv1.Securesign) {
		s.Spec.Rekor.ExternalAccess.Enabled = true
		s.Spec.Tuf.ExternalAccess.Enabled = true
		s.Spec.Fulcio.ExternalAccess.Enabled = true
		if s.Spec.TimestampAuthority != nil {
			s.Spec.TimestampAuthority.ExternalAccess.Enabled = true
		}
	}
}

func WithMonitoring() Opts {
	return func(s *rhtasv1.Securesign) {
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
	return func(s *rhtasv1.Securesign) {
		s.Spec.Rekor.RekorSearchUI.Enabled = ptr.To(true)
	}
}

func WithoutSearchUI() Opts {
	return func(s *rhtasv1.Securesign) {
		s.Spec.Rekor.RekorSearchUI.Enabled = ptr.To(false)
	}
}

func WithDefaultOIDC() Opts {
	return func(s *rhtasv1.Securesign) {
		s.Spec.Fulcio.Config = rhtasv1.FulcioConfig{
			OIDCIssuers: []rhtasv1.OIDCIssuer{
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
	return func(s *rhtasv1.Securesign) {
		s.Spec.Trillian.Db.Create = ptr.To(true)
		s.Spec.Trillian.Db.Pvc = rhtasv1.Pvc{
			Retain: ptr.To(false),
		}
	}
}

func WithExternalDatabase(secretName string) Opts {
	return func(s *rhtasv1.Securesign) {
		s.Spec.Trillian.Db.Create = ptr.To(false)
		s.Spec.Trillian.Db.DatabaseSecretRef = &rhtasv1.LocalObjectReference{
			Name: secretName,
		}
	}
}

func WithGeneratedCerts() Opts {
	return func(s *rhtasv1.Securesign) {
		s.Spec.Fulcio.Certificate = rhtasv1.FulcioCert{
			OrganizationName:  "MyOrg",
			OrganizationEmail: "my@email.org",
			CommonName:        "fulcio",
		}

		if s.Spec.TimestampAuthority != nil {
			s.Spec.TimestampAuthority.Signer = rhtasv1.TimestampAuthoritySigner{
				CertificateChain: rhtasv1.CertificateChain{
					RootCA: &rhtasv1.TsaCertificateAuthority{
						OrganizationName:  "MyOrg",
						OrganizationEmail: "my@email.org",
						CommonName:        "tsa.hostname",
					},
					IntermediateCA: []*rhtasv1.TsaCertificateAuthority{
						{
							OrganizationName:  "MyOrg",
							OrganizationEmail: "my@email.org",
							CommonName:        "tsa.hostname",
						},
					},
					LeafCA: &rhtasv1.TsaCertificateAuthority{
						OrganizationName:  "MyOrg",
						OrganizationEmail: "my@email.org",
						CommonName:        "tsa.hostname",
					},
				},
			}
		}
	}
}

func WithProvidedEncryptedCerts() Opts {
	return func(s *rhtasv1.Securesign) {
		s.Spec.Rekor.Signer = rhtasv1.RekorSigner{
			KMS: "secret",
			KeyRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{
					Name: "my-rekor-secret",
				},
				Key: "private",
			},
			PasswordRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{
					Name: "my-rekor-secret",
				},
				Key: "password",
			},
		}

		s.Spec.Fulcio.Certificate = rhtasv1.FulcioCert{
			PrivateKeyRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{
					Name: "my-fulcio-secret",
				},
				Key: "private",
			},
			PrivateKeyPasswordRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{
					Name: "my-fulcio-secret",
				},
				Key: "password",
			},
			CARef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{
					Name: "my-fulcio-secret",
				},
				Key: "cert",
			},
		}

		s.Spec.Ctlog.PrivateKeyRef = &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{
				Name: "my-ctlog-secret",
			},
			Key: "private",
		}
		s.Spec.Ctlog.PrivateKeyPasswordRef = &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{
				Name: "my-ctlog-secret",
			},
			Key: "password",
		}
		s.Spec.Ctlog.RootCertificates = []rhtasv1.SecretKeySelector{
			{
				LocalObjectReference: rhtasv1.LocalObjectReference{
					Name: "my-fulcio-secret",
				},
				Key: "cert",
			},
		}

		if s.Spec.TimestampAuthority != nil {
			s.Spec.TimestampAuthority.Signer = rhtasv1.TimestampAuthoritySigner{
				CertificateChain: rhtasv1.CertificateChain{
					CertificateChainRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{
							Name: "test-tsa-secret",
						},
						Key: "certificateChain",
					},
				},
				File: &rhtasv1.File{
					PrivateKeyRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{
							Name: "test-tsa-secret",
						},
						Key: "leafPrivateKey",
					},
					PasswordRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{
							Name: "test-tsa-secret",
						},
						Key: "leafPrivateKeyPassword",
					},
				},
			}
		}
	}
}

func WithProvidedUnencryptedCerts() Opts {
	return func(s *rhtasv1.Securesign) {
		s.Spec.Rekor.Signer = rhtasv1.RekorSigner{
			KMS: "secret",
			KeyRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{
					Name: "my-rekor-secret",
				},
				Key: "private",
			},
		}

		s.Spec.Fulcio.Certificate = rhtasv1.FulcioCert{
			PrivateKeyRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{
					Name: "my-fulcio-secret",
				},
				Key: "private",
			},
			CARef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{
					Name: "my-fulcio-secret",
				},
				Key: "cert",
			},
		}

		s.Spec.Ctlog.PrivateKeyRef = &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{
				Name: "my-ctlog-secret",
			},
			Key: "private",
		}
		s.Spec.Ctlog.RootCertificates = []rhtasv1.SecretKeySelector{
			{
				LocalObjectReference: rhtasv1.LocalObjectReference{
					Name: "my-fulcio-secret",
				},
				Key: "cert",
			},
		}

		if s.Spec.TimestampAuthority != nil {
			s.Spec.TimestampAuthority.Signer = rhtasv1.TimestampAuthoritySigner{
				CertificateChain: rhtasv1.CertificateChain{
					CertificateChainRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{
							Name: "test-tsa-secret",
						},
						Key: "certificateChain",
					},
				},
				File: &rhtasv1.File{
					PrivateKeyRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{
							Name: "test-tsa-secret",
						},
						Key: "leafPrivateKey",
					},
				},
			}
		}
	}
}

func WithTSA() Opts {
	return func(s *rhtasv1.Securesign) {
		s.Spec.TimestampAuthority = &rhtasv1.TimestampAuthoritySpec{}
	}
}

func WithNTPMonitoring() Opts {
	return func(s *rhtasv1.Securesign) {
		if s.Spec.TimestampAuthority != nil {
			s.Spec.TimestampAuthority.NTPMonitoring = rhtasv1.NTPMonitoring{
				Enabled: true,
				Config: &rhtasv1.NtpMonitoringConfig{
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

func WithReplicas(replicas *int32) Opts {
	return func(s *rhtasv1.Securesign) {
		s.Spec.Fulcio.Replicas = replicas
		s.Spec.Rekor.Replicas = replicas
		s.Spec.Rekor.RekorSearchUI.Replicas = replicas
		s.Spec.Ctlog.Replicas = replicas
		s.Spec.TimestampAuthority.Replicas = replicas
		s.Spec.Tuf.Replicas = replicas
		s.Spec.Trillian.LogServer.Replicas = replicas
		s.Spec.Trillian.LogSigner.Replicas = replicas
	}
}

func WithNFSPVC() Opts {
	return func(s *rhtasv1.Securesign) {
		pvcConf := rhtasv1.Pvc{
			Retain: ptr.To(false),
			Size:   ptr.To(resource.MustParse("100Mi")),
			AccessModes: []rhtasv1.PersistentVolumeAccessMode{
				"ReadWriteMany",
			},
			StorageClass: "nfs-csi",
		}

		s.Spec.Rekor.Pvc = pvcConf
		s.Spec.Tuf.Pvc = rhtasv1.TufPvc{
			Retain:       pvcConf.Retain,
			Size:         pvcConf.Size,
			AccessModes:  pvcConf.AccessModes,
			StorageClass: pvcConf.StorageClass,
		}
	}
}
