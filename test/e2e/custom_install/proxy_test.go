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
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"text/template"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var _ = Describe("Securesign install in proxy-env", Ordered, func() {
	cli, _ := support.CreateClient()

	var namespace *v1.Namespace
	var s *v1alpha1.Securesign
	var hostname string

	Describe("Successful installation with fake-proxy env", func() {
		BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
			namespace = new
		}))

		BeforeAll(func(ctx SpecContext) {
			hostname = fmt.Sprintf("%s.%s.svc", "proxy", namespace.Name)
			createProxyServer(ctx, cli, hostname, namespace.Name)
			installOperator(ctx, cli, namespace.Name, withProxy(hostname))
		})

		It("Install securesign", func(ctx SpecContext) {
			s = securesign.Create(namespace.Name, "test",
				securesign.WithDefaults(),
				func(v *v1alpha1.Securesign) {
					v.Spec.Fulcio.Config = v1alpha1.FulcioConfig{
						OIDCIssuers: []v1alpha1.OIDCIssuer{
							{
								ClientID:  "sigstore",
								IssuerURL: "https://oauth2.sigstore.dev/auth",
								Issuer:    "https://oauth2.sigstore.dev/auth",
								Type:      "email",
							},
						},
					}
				},
			)
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("All components are running", func(ctx SpecContext) {
			tas.VerifyAllComponents(ctx, cli, s, true)
		})

		It("OIDC connection run through proxy", func(ctx SpecContext) {
			// we need to create clientSet
			clientSet, err := kubernetes.NewForConfig(config.GetConfigOrDie())
			if err != nil {
				Fail(err.Error())
			}

			Eventually(func(ctx context.Context) string {
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
			}).WithContext(ctx).Should(ContainSubstring("CONNECT oauth2.sigstore.dev:443"))
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

func createProxyServer(ctx context.Context, cli runtimeCli.Client, hostname string, ns string) {
	type templateData struct {
		Namespace string
	}

	crt := deploymentCertificate(hostname, ns)
	Expect(cli.Create(ctx, crt)).To(Succeed())

	tmpl, err := template.ParseFS(testdata, "testdata/proxy.yaml")
	if err != nil {
		Fail(err.Error())
	}

	var processedYaml bytes.Buffer
	err = tmpl.Execute(&processedYaml, templateData{
		Namespace: ns,
	})
	if err != nil {
		Fail(err.Error())
	}

	decoder := yaml.NewYAMLOrJSONDecoder(&processedYaml, 4096)
	for {
		var unstructuredObj unstructured.Unstructured
		err = decoder.Decode(&unstructuredObj)
		if err != nil {
			// io.EOF means we've read all the documents in the YAML file
			if errors.Is(err, io.EOF) {
				break
			}
			Fail(err.Error())
		}

		// Skip empty objects that can result from comments or empty documents
		if unstructuredObj.Object == nil {
			continue
		}

		err = cli.Create(ctx, &unstructuredObj)
		if err != nil {
			Fail(err.Error())
		}
	}
}

func withProxy(hostname string) func(pod *v1.Pod) {
	var noProxy = []string{
		".cluster.local",
		".svc",
		"localhost",
		"10.0.0.0/16",
		"172.30.0.0/16",
		"10.128.0.0/14",
		"10.96.0.0/12",
		"127.0.0.1",
	}

	return func(pod *v1.Pod) {
		pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env,
			v1.EnvVar{
				Name:  "HTTP_PROXY",
				Value: hostname,
			},
			v1.EnvVar{
				Name:  "NO_PROXY",
				Value: strings.Join(noProxy, ","),
			},
			v1.EnvVar{
				Name:  "HTTPS_PROXY",
				Value: hostname,
			},
		)
	}
}
