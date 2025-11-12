//go:build custom_install

package custom_install

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/template"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

		AfterAll(func(ctx SpecContext) {
			_ = cli.Delete(ctx, s)
			// wait until object has been deleted. Manager need to handle finalizer
			Eventually(func(ctx context.Context) error {
				return cli.Get(ctx, runtimeCli.ObjectKeyFromObject(s), &v1alpha1.Securesign{})
			}).WithContext(ctx).Should(And(HaveOccurred(), WithTransform(apierrors.IsNotFound, BeTrue())))
			uninstallOperator(ctx, cli, namespace.Name)
		})

		BeforeAll(func(ctx SpecContext) {
			hostname = fmt.Sprintf("%s.%s.svc", "proxy", namespace.Name)
			createProxyServer(ctx, cli, namespace.Name)
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
			Expect(err).NotTo(HaveOccurred())
			request := clientSet.CoreV1().Pods(namespace.Name).GetLogs("proxy", &v1.PodLogOptions{})

			Eventually(func(g Gomega, ctx context.Context) (string, error) {
				podLogs, err := request.Stream(ctx)
				if err != nil {
					return "", err
				}
				defer func() {
					if err = podLogs.Close(); err != nil {
						GinkgoLogr.Error(err, err.Error())
					}
				}()

				buf := new(bytes.Buffer)
				_, err = io.Copy(buf, podLogs)
				if err != nil {
					return "", err
				}
				return buf.String(), nil
			}).WithContext(ctx).Should(ContainSubstring("CONNECT oauth2.sigstore.dev:443"))
		})
	})
})

func createProxyServer(ctx context.Context, cli runtimeCli.Client, ns string) {
	type templateData struct {
		Namespace string
	}

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
		"127.0.0.1",
	}

	url := fmt.Sprintf("http://%s:80", hostname)

	return func(pod *v1.Pod) {
		pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env,
			v1.EnvVar{
				Name:  "HTTP_PROXY",
				Value: url,
			},
			v1.EnvVar{
				Name:  "NO_PROXY",
				Value: strings.Join(noProxy, ","),
			},
			v1.EnvVar{
				Name:  "HTTPS_PROXY",
				Value: url,
			},
		)
	}
}
