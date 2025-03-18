//go:build custom_install

package custom_install

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/tas"
	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/yaml"
)

const httpdTlsMountPath = "/etc/tls/private"

var noProxy = []string{
	".cluster.local",
	".svc",
	"localhost",
}

var _ = Describe("Securesign install in proxy-env", Ordered, func() {
	cli, _ := support.CreateClient()
	ctx := context.TODO()

	var namespace *v1.Namespace
	var securesign *v1alpha1.Securesign
	var hostname string

	AfterEach(func() {
		if CurrentSpecReport().Failed() && support.IsCIEnvironment() {
			support.DumpNamespace(ctx, cli, namespace.Name)
		}
	})

	Describe("Successful installation with fake-proxy env", func() {
		BeforeAll(func() {
			namespace = support.CreateTestNamespace(ctx, cli)
			hostname = fmt.Sprintf("%s.%s.svc", "proxy", namespace.Name)

			createProxyServer(ctx, cli, hostname, namespace.Name)

			installOperatorWithProxyConf(ctx, cli, hostname, namespace.Name)

			DeferCleanup(func() {
				_ = cli.Delete(ctx, namespace)
			})
		})

		It("Install securesign", func() {
			securesign = &v1alpha1.Securesign{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace.Name,
					Name:      "test",
					Annotations: map[string]string{
						"rhtas.redhat.com/metrics": "false",
					},
				},
				Spec: v1alpha1.SecuresignSpec{
					Rekor: v1alpha1.RekorSpec{
						ExternalAccess: v1alpha1.ExternalAccess{
							Enabled: true,
						},
					},
					Fulcio: v1alpha1.FulcioSpec{
						ExternalAccess: v1alpha1.ExternalAccess{
							Enabled: true,
						},
						Config: v1alpha1.FulcioConfig{
							OIDCIssuers: []v1alpha1.OIDCIssuer{
								{
									ClientID:  "sigstore",
									IssuerURL: "https://oauth2.sigstore.dev/auth",
									Issuer:    "https://oauth2.sigstore.dev/auth",
									Type:      "email",
								},
							}},
						Certificate: v1alpha1.FulcioCert{
							OrganizationName:  "MyOrg",
							OrganizationEmail: "my@email.org",
							CommonName:        "fulcio",
						},
					},
					Tuf: v1alpha1.TufSpec{
						ExternalAccess: v1alpha1.ExternalAccess{
							Enabled: true,
						},
					},
					Ctlog:    v1alpha1.CTlogSpec{},
					Trillian: v1alpha1.TrillianSpec{},
					TimestampAuthority: &v1alpha1.TimestampAuthoritySpec{
						ExternalAccess: v1alpha1.ExternalAccess{
							Enabled: true,
						},
						Signer: v1alpha1.TimestampAuthoritySigner{
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
						},
						NTPMonitoring: v1alpha1.NTPMonitoring{
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
						},
					},
				},
			}
			Expect(cli.Create(ctx, securesign)).To(Succeed())
		})

		It("All components are running", func() {
			tas.VerifyAllComponents(ctx, cli, securesign, true)
		})

		It("OIDC connection run through proxy", func() {
			// we need to create clientSet
			clientSet, err := kubernetes.NewForConfig(config.GetConfigOrDie())
			if err != nil {
				Fail(err.Error())
			}

			Eventually(func() string {
				request := clientSet.CoreV1().Pods(namespace.Name).GetLogs("proxy", &v1.PodLogOptions{})
				podLogs, err := request.Stream(ctx)
				if err != nil {
					Fail(err.Error())
				}
				defer func() { _ = podLogs.Close() }()

				buf := new(bytes.Buffer)
				_, err = io.Copy(buf, podLogs)
				if err != nil {
					Fail(err.Error())
				}
				return buf.String()
			}).Should(ContainSubstring("CONNECT oauth2.sigstore.dev:443"))
		})
	})
})

func deploymentCertificate(hostname string, ns string) *v1.Secret {
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test"},
		},
		DNSNames:              []string{hostname},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	// generate the certificate private key
	certPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).ToNot(HaveOccurred())

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(certPrivateKey)
	// encode for storing into a Secret
	privateKeyPem := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: privateKeyBytes,
		},
	)
	certBytes, err := x509.CreateCertificate(rand.Reader, cert, cert, &certPrivateKey.PublicKey, certPrivateKey)
	Expect(err).ToNot(HaveOccurred())

	// encode for storing into a Secret
	certPem := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	return &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      "tls-secret",
		},
		Type: v1.SecretTypeTLS,
		Data: map[string][]byte{
			v1.TLSCertKey:       certPem,
			v1.TLSPrivateKeyKey: privateKeyPem,
		},
	}
}
func newHTTPDConfig(ns, hostname string) *v1.ConfigMap {
	return &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      "httpd-config",
		},
		Data: map[string]string{
			"httpd.conf": fmt.Sprintf(`
ServerRoot "/etc/httpd

PidFile /var/run/httpd/httpd.pid"

LoadModule mpm_event_module /usr/local/apache2/modules/mod_mpm_event.so
LoadModule authn_core_module /usr/local/apache2/modules/mod_authn_core.so
LoadModule authz_core_module /usr/local/apache2/modules/mod_authz_core.so
LoadModule proxy_module /usr/local/apache2/modules/mod_proxy.so
LoadModule proxy_http_module /usr/local/apache2/modules/mod_proxy_http.so
LoadModule proxy_connect_module /usr/local/apache2/modules/mod_proxy_connect.so
LoadModule headers_module /usr/local/apache2/modules/mod_headers.so
LoadModule setenvif_module /usr/local/apache2/modules/mod_setenvif.so
LoadModule version_module /usr/local/apache2/modules/mod_version.so
LoadModule log_config_module /usr/local/apache2/modules/mod_log_config.so
LoadModule env_module /usr/local/apache2/modules/mod_env.so
LoadModule unixd_module /usr/local/apache2/modules/mod_unixd.so
LoadModule status_module /usr/local/apache2/modules/mod_status.so
LoadModule autoindex_module /usr/local/apache2/modules/mod_autoindex.so
LoadModule ssl_module /usr/local/apache2/modules/mod_ssl.so

Mutex posixsem

LogFormat "%%h %%l %%u %%t \"%%r\" %%>s %%b" common
CustomLog /dev/stdout common
ErrorLog /dev/stderr

LogLevel info

Listen 8080
Listen 8443

ServerName %s

ProxyRequests On
ProxyVia Off

<VirtualHost *:8443>
  SSLEngine on

  SSLCertificateFile "%s/%s"
  SSLCertificateKeyFile "%s/%s"

  AllowEncodedSlashes NoDecode
</VirtualHost>
`,
				hostname, httpdTlsMountPath, v1.TLSCertKey, httpdTlsMountPath, v1.TLSPrivateKeyKey,
			),
		},
	}
}

func newHTTPDPod(ns, configName, secretName string) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      "proxy",
			Labels:    map[string]string{"name": "proxy"},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:    "httpd",
					Image:   "mirror.gcr.io/httpd:2.4.46",
					Command: []string{"httpd", "-f", "/etc/httpd/httpd.conf", "-DFOREGROUND"},
					Ports: []v1.ContainerPort{
						{
							Name:          "http",
							ContainerPort: 8080,
						},
						{
							Name:          "https",
							ContainerPort: 8443,
						},
					},
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      "tls",
							MountPath: httpdTlsMountPath,
							ReadOnly:  true,
						},
						{
							Name:      "httpd-conf",
							MountPath: "/etc/httpd",
							ReadOnly:  true,
						},
						{
							Name:      "httpd-run",
							MountPath: "/var/run/httpd",
						},
					},
				},
			},
			Volumes: []v1.Volume{
				{
					Name: "tls",
					VolumeSource: v1.VolumeSource{
						Secret: &v1.SecretVolumeSource{
							SecretName: secretName,
						},
					},
				},
				{
					Name: "httpd-conf",
					VolumeSource: v1.VolumeSource{
						ConfigMap: &v1.ConfigMapVolumeSource{
							LocalObjectReference: v1.LocalObjectReference{
								Name: configName,
							},
						},
					},
				},
				{
					Name: "httpd-run",
					VolumeSource: v1.VolumeSource{
						EmptyDir: &v1.EmptyDirVolumeSource{},
					},
				},
			},
		},
	}
}

func newHTTPDService(deployment *v1.Pod) *v1.Service {
	return &v1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: deployment.Namespace,
			Name:      deployment.Name,
		},
		Spec: v1.ServiceSpec{
			Selector: deployment.Labels,
			Ports: []v1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromString("http"),
				},
				{
					Name:       "https",
					Port:       443,
					TargetPort: intstr.FromString("https"),
				},
			},
		},
	}
}

func createProxyServer(ctx context.Context, cli runtimeCli.Client, hostname string, ns string) {
	crt := deploymentCertificate(hostname, ns)
	Expect(cli.Create(ctx, crt)).To(Succeed())
	cm := newHTTPDConfig(ns, hostname)
	Expect(cli.Create(ctx, cm)).To(Succeed())
	proxy := newHTTPDPod(ns, cm.Name, crt.Name)
	Expect(cli.Create(ctx, proxy)).To(Succeed())
	Eventually(func(g Gomega) v1.PodPhase {
		g.Expect(cli.Get(ctx, types.NamespacedName{
			Name:      proxy.Name,
			Namespace: ns,
		}, proxy)).To(Succeed())
		return proxy.Status.Phase
	}).Should(Equal(v1.PodRunning))

	svc := newHTTPDService(proxy)
	Expect(cli.Create(ctx, svc)).To(Succeed())
}

func rbac(ns string) []runtimeCli.Object {
	files := []string{"service_account.yaml", "role.yaml", "role_binding.yaml"}
	var objects = make([]runtimeCli.Object, 0)

	for _, f := range files {
		bytes, err := os.ReadFile("../../../config/rbac/" + f)
		if err != nil {
			Fail(err.Error())
		}
		u := &unstructured.Unstructured{Object: map[string]interface{}{}}
		if err := yaml.Unmarshal(bytes, &u); err != nil {
			Fail(err.Error())
		}
		u.SetNamespace(ns)

		// we need to create binding with our namespaced serviceAccount
		if u.GetObjectKind().GroupVersionKind().Kind == "ClusterRoleBinding" {
			binding := &v12.ClusterRoleBinding{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, binding); err == nil {
				for i := range binding.Subjects {
					binding.Subjects[i].Namespace = ns
				}
				objects = append(objects, binding)
			} else {
				Fail(err.Error())
			}
		} else {
			objects = append(objects, u)
		}
	}

	return objects
}
func installOperatorPodWithProxyConf(proxyHostname string, ns string) *v1.Pod {
	image, ok := os.LookupEnv("TEST_MANAGER_IMAGE")
	Expect(ok).To(BeTrue(), "TEST_MANAGER_IMAGE variable not set")
	Expect(image).ToNot(BeEmpty(), "TEST_MANAGER_IMAGE can't be empty")

	return &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      "manager",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:    "manager",
					Image:   image,
					Command: []string{"/manager"},
					Env: []v1.EnvVar{
						{
							Name:  "HTTP_PROXY",
							Value: proxyHostname,
						},
						{
							Name:  "NO_PROXY",
							Value: strings.Join(noProxy, ","),
						},
						{
							Name:  "HTTPS_PROXY",
							Value: proxyHostname,
						},
						{
							Name:  "OPENSHIFT",
							Value: support.EnvOrDefault("OPENSHIFT", "false"),
						},
					},
					LivenessProbe: &v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							HTTPGet: &v1.HTTPGetAction{
								Path: "/healthz",
								Port: intstr.IntOrString{IntVal: 8081},
							},
						},
						InitialDelaySeconds: 15,
						PeriodSeconds:       20,
					},
					ReadinessProbe: &v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							HTTPGet: &v1.HTTPGetAction{
								Path: "/readyz",
								Port: intstr.IntOrString{IntVal: 8081},
							},
						},
						InitialDelaySeconds: 5,
						PeriodSeconds:       10,
					},
				},
			},
			ServiceAccountName: "operator-controller-manager",
		},
	}
}

func installOperatorWithProxyConf(ctx context.Context, cli runtimeCli.Client, proxyHostname string, ns string) {
	for _, o := range rbac(ns) {
		c := o.DeepCopyObject().(runtimeCli.Object)
		if e := cli.Get(ctx, runtimeCli.ObjectKeyFromObject(o), c); !errors.IsNotFound(e) {
			Expect(cli.Delete(ctx, o)).To(Succeed())
		}
		Expect(cli.Create(ctx, o)).To(Succeed())
	}
	Expect(cli.Create(ctx, installOperatorPodWithProxyConf(proxyHostname, ns))).To(Succeed())
}
