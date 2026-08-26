package securesign

import (
	"context"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/trillian/dbsecret"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/condition"
	"github.com/securesign/operator/test/e2e/support/postgresql"
	core "k8s.io/api/core/v1"
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
		WithIngress()(s)
		WithDefaultOIDC()(s)
		WithNTPMonitoring()(s)
	}
}

func WithFipsDefaults(namespace string) Opts {
	return func(s *rhtasv1.Securesign) {
		WithTSA()(s)
		WithGeneratedCerts()(s)
		WithExternalPostgresDB(namespace, postgresql.DefaultSecretName)(s)
		WithIngress()(s)
		WithDefaultOIDC()(s)
		WithNTPMonitoring()(s)
	}
}

func ChooseDefaults(fipsEnabled bool, namespace string) Opts {
	if fipsEnabled {
		return WithFipsDefaults(namespace)
	}
	return WithDefaults()
}

func WithIngress() Opts {
	return func(s *rhtasv1.Securesign) {
		s.Spec.Rekor.Ingress.Enabled = ptr.To(true)
		s.Spec.Tuf.Ingress.Enabled = ptr.To(true)
		s.Spec.Fulcio.Ingress.Enabled = ptr.To(true)
		s.Spec.Ctlog.Ingress.Enabled = ptr.To(true)
		if s.Spec.TimestampAuthority != nil {
			s.Spec.TimestampAuthority.Ingress.Enabled = ptr.To(true)
		}
	}
}

func WithoutMonitoring() Opts {
	return func(s *rhtasv1.Securesign) {
		s.Spec.Rekor.Monitoring.Metrics.Enabled = ptr.To(false)
		s.Spec.Rekor.Monitoring.ServiceMonitor.Enabled = ptr.To(false)
		s.Spec.Fulcio.Monitoring.Metrics.Enabled = ptr.To(false)
		s.Spec.Fulcio.Monitoring.ServiceMonitor.Enabled = ptr.To(false)
		s.Spec.Trillian.Monitoring.Metrics.Enabled = ptr.To(false)
		s.Spec.Trillian.Monitoring.ServiceMonitor.Enabled = ptr.To(false)
		s.Spec.Ctlog.Monitoring.Metrics.Enabled = ptr.To(false)
		s.Spec.Ctlog.Monitoring.ServiceMonitor.Enabled = ptr.To(false)
		if s.Spec.TimestampAuthority != nil {
			s.Spec.TimestampAuthority.Monitoring.Metrics.Enabled = ptr.To(false)
			s.Spec.TimestampAuthority.Monitoring.ServiceMonitor.Enabled = ptr.To(false)
		}
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
		s.Spec.Trillian.Auth = dbsecret.DbSecretToAuth(&rhtasv1.LocalObjectReference{
			Name: secretName,
		})
	}
}

func WithExternalPostgresDB(namespace, secretName string) Opts {
	return func(s *rhtasv1.Securesign) {
		s.Spec.Trillian.Db.Create = ptr.To(false)
		s.Spec.Trillian.Db.Provider = postgresql.Provider
		s.Spec.Trillian.Db.Uri = postgresql.ConnectionURI
		s.Spec.Trillian.Auth = &rhtasv1.Auth{
			Env: postgresql.AuthEnvVars(namespace, secretName),
		}
	}
}

func WithGeneratedCerts() Opts {
	return func(s *rhtasv1.Securesign) {
		s.Spec.Fulcio.Signer = rhtasv1.FulcioSigner{
			CertificateChain: rhtasv1.FulcioCertificateChain{
				OrganizationName:  "MyOrg",
				OrganizationEmail: "my@email.org",
				CommonName:        "fulcio",
			},
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

func WithProvidedCerts() Opts {
	return func(s *rhtasv1.Securesign) {
		s.Spec.Rekor.Signer = rhtasv1.RekorSigner{
			Type: rhtasv1.RekorSignerTypeSecret,
			KeyRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{
					Name: "my-rekor-secret",
				},
				Key: "private",
			},
		}

		s.Spec.Fulcio.Signer = rhtasv1.FulcioSigner{
			File: &rhtasv1.FulcioFile{
				PrivateKeyRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{
						Name: "my-fulcio-secret",
					},
					Key: "private",
				},
			},
			CertificateChain: rhtasv1.FulcioCertificateChain{
				CertificateChainRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{
						Name: "my-fulcio-secret",
					},
					Key: "cert",
				},
			},
		}

		s.Spec.Ctlog.Signer = rhtasv1.CTlogSigner{
			Type: "file",
			File: &rhtasv1.CTlogFile{
				PrivateKeyRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{
						Name: "my-ctlog-secret",
					},
					Key: "private",
				},
			},
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
				Enabled: ptr.To(true),
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
		s.Spec.Ctlog.Replicas = replicas
		s.Spec.TimestampAuthority.Replicas = replicas
		s.Spec.Tuf.Replicas = replicas
		s.Spec.Trillian.LogServer.Replicas = replicas
		s.Spec.Trillian.LogSigner.Replicas = replicas
	}
}

func WithPKCS11Signer(namespace string) Opts {
	return func(s *rhtasv1.Securesign) {
		WithTSA()(s)
		WithIngress()(s)
		WithDefaultOIDC()(s)

		// --- Fulcio PKCS#11 signer ---
		s.Spec.Fulcio.Signer = rhtasv1.FulcioSigner{
			Type: rhtasv1.SignerTypePKCS11,
			PKCS11: &rhtasv1.FulcioPKCS11Config{
				KeyID: ptr.To(int64(99)),
				ConfigRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{
						Name: "fulcio-pkcs11-config",
					},
					Key: "crypto11.conf",
				},
			},
			CertificateChain: rhtasv1.FulcioCertificateChain{
				CertificateChainRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{
						Name: "fulcio-root-ca",
					},
					Key: "cert.pem",
				},
			},
		}
		s.Spec.Fulcio.Auth = &rhtasv1.Auth{
			Env: []core.EnvVar{
				{
					Name:  "SOFTHSM2_CONF",
					Value: "/etc/softhsm/softhsm2.conf",
				},
			},
		}

		// --- CTLog PKCS#11 signer ---
		s.Spec.Ctlog.Signer = rhtasv1.CTlogSigner{
			Type: rhtasv1.SignerTypePKCS11,
			PKCS11: &rhtasv1.CTlogPKCS11Config{
				ModulePath: "/usr/lib64/pkcs11/libsofthsm2.so",
				TokenLabel: "PKCS11CA",
				PinSecretRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{
						Name: "hsm-credentials",
					},
					Key: "pin",
				},
				PublicKeyRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{
						Name: "ctlog-public-key",
					},
					Key: "public.pem",
				},
			},
		}
		s.Spec.Ctlog.Auth = &rhtasv1.Auth{
			Env: []core.EnvVar{
				{
					Name:  "SOFTHSM2_CONF",
					Value: "/etc/softhsm/softhsm2.conf",
				},
			},
		}

		// --- CTLog root certificates (Fulcio's CA) ---
		s.Spec.Ctlog.RootCertificates = []rhtasv1.SecretKeySelector{
			{
				LocalObjectReference: rhtasv1.LocalObjectReference{
					Name: "fulcio-root-ca",
				},
				Key: "cert.pem",
			},
		}

		// --- Fulcio init containers, volumes, volumeMounts ---
		s.Spec.Fulcio.InitContainers = []rhtasv1.InitContainerSpec{
			{
				Name:    "hsm-lib-export",
				Image:   "quay.io/securesign/softhsm-init:1.0-test",
				Command: []string{"cp", "/usr/lib64/pkcs11/libsofthsm2.so", "/var/run/hsm-lib/"},
				VolumeMounts: []core.VolumeMount{
					{
						Name:      "hsm-lib",
						MountPath: "/var/run/hsm-lib",
					},
				},
			},
		}

		s.Spec.Fulcio.Volumes = []rhtasv1.AdditionalVolume{
			{
				Name: "softhsm-config",
				AdditionalVolumeSource: rhtasv1.AdditionalVolumeSource{
					ConfigMap: &core.ConfigMapVolumeSource{
						LocalObjectReference: core.LocalObjectReference{
							Name: "softhsm-config",
						},
					},
				},
			},
			{
				Name: "hsm-tokens",
				AdditionalVolumeSource: rhtasv1.AdditionalVolumeSource{
					PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
						ClaimName: "hsm-tokens-pvc",
					},
				},
			},
		}

		s.Spec.Fulcio.VolumeMounts = []core.VolumeMount{
			{
				Name:      "softhsm-config",
				MountPath: "/etc/softhsm",
				ReadOnly:  true,
			},
		}

		// --- CTLog init containers, volumes, volumeMounts ---
		s.Spec.Ctlog.InitContainers = []rhtasv1.InitContainerSpec{
			{
				Name:    "hsm-lib-export",
				Image:   "quay.io/securesign/softhsm-init:1.0-test",
				Command: []string{"cp", "/usr/lib64/pkcs11/libsofthsm2.so", "/var/run/hsm-lib/"},
				VolumeMounts: []core.VolumeMount{
					{
						Name:      "hsm-lib",
						MountPath: "/var/run/hsm-lib",
					},
				},
			},
		}

		s.Spec.Ctlog.Volumes = []rhtasv1.AdditionalVolume{
			{
				Name: "softhsm-config",
				AdditionalVolumeSource: rhtasv1.AdditionalVolumeSource{
					ConfigMap: &core.ConfigMapVolumeSource{
						LocalObjectReference: core.LocalObjectReference{
							Name: "softhsm-config",
						},
					},
				},
			},
			{
				Name: "hsm-tokens",
				AdditionalVolumeSource: rhtasv1.AdditionalVolumeSource{
					PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
						ClaimName: "ctlog-hsm-tokens-pvc",
					},
				},
			},
		}

		s.Spec.Ctlog.VolumeMounts = []core.VolumeMount{
			{
				Name:      "softhsm-config",
				MountPath: "/etc/softhsm",
				ReadOnly:  true,
			},
		}

		// --- TSA generated certs ---
		if s.Spec.TimestampAuthority != nil {
			s.Spec.TimestampAuthority.Signer = rhtasv1.TimestampAuthoritySigner{
				CertificateChain: rhtasv1.CertificateChain{
					RootCA: &rhtasv1.TsaCertificateAuthority{
						OrganizationName: "RHTAS",
						CommonName:       "tsa-root",
					},
					IntermediateCA: []*rhtasv1.TsaCertificateAuthority{
						{
							OrganizationName: "RHTAS",
							CommonName:       "tsa-intermediate",
						},
					},
					LeafCA: &rhtasv1.TsaCertificateAuthority{
						OrganizationName: "RHTAS",
						CommonName:       "tsa-leaf",
					},
				},
			}
		}

		// --- NTP Monitoring ---
		WithNTPMonitoring()(s)
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

		s.Spec.Rekor.Attestations.Pvc = pvcConf
		s.Spec.Tuf.Pvc = rhtasv1.Pvc{
			Retain:       pvcConf.Retain,
			Size:         pvcConf.Size,
			AccessModes:  pvcConf.AccessModes,
			StorageClass: pvcConf.StorageClass,
		}
	}
}
