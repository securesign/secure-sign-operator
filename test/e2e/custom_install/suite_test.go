//go:build custom_install

package custom_install

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"embed"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/test/e2e/support"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"
)

//go:embed testdata/*
var testdata embed.FS

const (
	managerPodName     = "manager"
	webhookServiceName = "webhook-service"
	webhookSecretName  = "webhook-server-cert"
	webhookMWCName     = "mutating-webhook-configuration"
)

func TestCustomInstall(t *testing.T) {
	t.Setenv("TUF_ROOT", t.TempDir())
	RegisterFailHandler(Fail)
	log.SetLogger(GinkgoLogr)
	SetDefaultEventuallyTimeout(time.Duration(3) * time.Minute)
	SetDefaultEventuallyPollingInterval(1 * time.Second)
	EnforceDefaultTimeoutsWhenUsingContexts()
	RunSpecs(t, "With customized install")

	// print whole stack in case of failure
	format.MaxLength = 0
}

func uninstallOperator(ctx context.Context, cli runtimeCli.Client, namespace string) {
	cleanupWebhookInfra(ctx, cli, namespace)

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managerPodName,
			Namespace: namespace,
		},
	}
	_ = cli.Delete(ctx, pod)
	Eventually(func(ctx context.Context) error {
		return cli.Get(ctx, runtimeCli.ObjectKeyFromObject(pod), &v1.Pod{})
	}).WithContext(ctx).Should(And(HaveOccurred(), WithTransform(errors.IsNotFound, BeTrue())))
}

func installOperator(ctx context.Context, cli runtimeCli.Client, ns string, opts ...optManagerPod) {
	for _, o := range rbac(ns) {
		c := o.DeepCopyObject().(runtimeCli.Object)
		if e := cli.Get(ctx, runtimeCli.ObjectKeyFromObject(o), c); !errors.IsNotFound(e) {
			Expect(cli.Delete(ctx, o)).To(Succeed())
		}
		Expect(cli.Create(ctx, o)).To(Succeed())
	}
	createWebhookInfra(ctx, cli, ns)

	pod := managerPod(ns, opts...)
	Expect(cli.Create(ctx, pod)).To(Succeed())

	Eventually(func(ctx context.Context) bool {
		Expect(cli.Get(ctx, runtimeCli.ObjectKeyFromObject(pod), pod)).To(Succeed())
		for _, c := range pod.Status.Conditions {
			if c.Type == v1.PodReady {
				return c.Status == v1.ConditionTrue
			}
		}
		return false
	}).WithContext(ctx).Should(BeTrue())

	// wait for webhook server to accept connections
	Eventually(func(ctx context.Context) error {
		probe := &rhtasv1.Trillian{
			ObjectMeta: metav1.ObjectMeta{Name: "webhook-probe", Namespace: ns},
		}
		return cli.Create(ctx, probe, &runtimeCli.CreateOptions{DryRun: []string{metav1.DryRunAll}})
	}).WithContext(ctx).Should(Succeed())
}

type optManagerPod func(pod *v1.Pod)

func managerPod(ns string, opts ...optManagerPod) *v1.Pod {
	image, ok := os.LookupEnv("TEST_MANAGER_IMAGE")
	Expect(ok).To(BeTrue(), "TEST_MANAGER_IMAGE variable not set")
	Expect(image).ToNot(BeEmpty(), "TEST_MANAGER_IMAGE can't be empty")

	pod := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      managerPodName,
			Labels: map[string]string{
				"control-plane": "operator-controller-manager",
			},
		},
		Spec: v1.PodSpec{
			SecurityContext: &v1.PodSecurityContext{
				RunAsNonRoot: ptr.To(true),
				SeccompProfile: &v1.SeccompProfile{
					Type: v1.SeccompProfileTypeRuntimeDefault,
				},
			},
			Containers: []v1.Container{
				{
					Name:    "manager",
					Image:   image,
					Command: []string{"/manager"},
					SecurityContext: &v1.SecurityContext{
						AllowPrivilegeEscalation: ptr.To(false),
						Capabilities: &v1.Capabilities{
							Drop: []v1.Capability{"ALL"},
						},
					},
					Env: []v1.EnvVar{
						{
							Name:  "OPENSHIFT",
							Value: support.EnvOrDefault("OPENSHIFT", "false"),
						},
						{
							Name:  "INGRESS_HOST_TEMPLATE",
							Value: support.EnvOrDefault("INGRESS_HOST_TEMPLATE", "%[1]s.local"),
						},
					},
					Ports: []v1.ContainerPort{
						{
							ContainerPort: 9443,
							Name:          "webhook-server",
							Protocol:      v1.ProtocolTCP,
						},
					},
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      "cert",
							MountPath: "/tmp/k8s-webhook-server/serving-certs",
							ReadOnly:  true,
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
			Volumes: []v1.Volume{
				{
					Name: "cert",
					VolumeSource: v1.VolumeSource{
						Secret: &v1.SecretVolumeSource{
							SecretName:  webhookSecretName,
							DefaultMode: ptr.To(int32(420)),
						},
					},
				},
			},
			ServiceAccountName: "operator-controller-manager",
		},
	}

	for _, opt := range opts {
		opt(pod)
	}

	return pod
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

func generateWebhookCert(namespace string) (caPEM, certPEM, keyPEM []byte) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "webhook-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	Expect(err).NotTo(HaveOccurred())
	caCert, err := x509.ParseCertificate(caCertDER)
	Expect(err).NotTo(HaveOccurred())

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: fmt.Sprintf("%s.%s.svc", webhookServiceName, namespace)},
		DNSNames: []string{
			fmt.Sprintf("%s.%s.svc", webhookServiceName, namespace),
			fmt.Sprintf("%s.%s.svc.cluster.local", webhookServiceName, namespace),
		},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	}
	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	Expect(err).NotTo(HaveOccurred())

	caPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCertDER})
	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	Expect(err).NotTo(HaveOccurred())
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDER})
	return caPEM, certPEM, keyPEM
}

var webhookResources = []struct {
	resource string
	path     string
	name     string
}{
	{"ctlogs", "/mutate-rhtas-redhat-com-v1-ctlog", "mctlog.rhtas.redhat.com"},
	{"fulcios", "/mutate-rhtas-redhat-com-v1-fulcio", "mfulcio.rhtas.redhat.com"},
	{"rekors", "/mutate-rhtas-redhat-com-v1-rekor", "mrekor.rhtas.redhat.com"},
	{"securesigns", "/mutate-rhtas-redhat-com-v1-securesign", "msecuresign.rhtas.redhat.com"},
	{"timestampauthorities", "/mutate-rhtas-redhat-com-v1-timestampauthority", "mtimestampauthority.rhtas.redhat.com"},
	{"trillians", "/mutate-rhtas-redhat-com-v1-trillian", "mtrillian.rhtas.redhat.com"},
	{"tufs", "/mutate-rhtas-redhat-com-v1-tuf", "mtuf.rhtas.redhat.com"},
}

func createWebhookInfra(ctx context.Context, cli runtimeCli.Client, ns string) {
	caPEM, certPEM, keyPEM := generateWebhookCert(ns)

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      webhookSecretName,
			Namespace: ns,
		},
		Type: v1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": certPEM,
			"tls.key": keyPEM,
		},
	}
	Expect(cli.Create(ctx, secret)).To(Succeed())

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      webhookServiceName,
			Namespace: ns,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{
				Port:       443,
				TargetPort: intstr.FromInt32(9443),
				Protocol:   v1.ProtocolTCP,
			}},
			Selector: map[string]string{
				"control-plane": "operator-controller-manager",
			},
		},
	}
	Expect(cli.Create(ctx, svc)).To(Succeed())

	fail := admissionregistrationv1.Fail
	equivalent := admissionregistrationv1.Equivalent
	none := admissionregistrationv1.SideEffectClassNone
	webhooks := make([]admissionregistrationv1.MutatingWebhook, 0, len(webhookResources))
	for _, wh := range webhookResources {
		webhooks = append(webhooks, admissionregistrationv1.MutatingWebhook{
			Name:                    wh.name,
			AdmissionReviewVersions: []string{"v1"},
			ClientConfig: admissionregistrationv1.WebhookClientConfig{
				Service: &admissionregistrationv1.ServiceReference{
					Name:      webhookServiceName,
					Namespace: ns,
					Path:      ptr.To(wh.path),
				},
				CABundle: caPEM,
			},
			FailurePolicy: &fail,
			MatchPolicy:   &equivalent,
			SideEffects:   &none,
			Rules: []admissionregistrationv1.RuleWithOperations{{
				Operations: []admissionregistrationv1.OperationType{
					admissionregistrationv1.Create,
					admissionregistrationv1.Update,
				},
				Rule: admissionregistrationv1.Rule{
					APIGroups:   []string{"rhtas.redhat.com"},
					APIVersions: []string{"v1"},
					Resources:   []string{wh.resource},
				},
			}},
		})
	}
	mwc := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: webhookMWCName,
		},
		Webhooks: webhooks,
	}
	Expect(cli.Create(ctx, mwc)).To(Succeed())

	for _, wh := range webhookResources {
		patchCRDWebhook(ctx, cli, wh.resource+".rhtas.redhat.com", ns, caPEM)
	}
}

func patchCRDWebhook(ctx context.Context, cli runtimeCli.Client, name, ns string, caBundle []byte) {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	Expect(cli.Get(ctx, runtimeCli.ObjectKey{Name: name}, crd)).To(Succeed())

	before := crd.DeepCopy()
	crd.Spec.Conversion.Webhook.ClientConfig.Service.Namespace = ns
	crd.Spec.Conversion.Webhook.ClientConfig.CABundle = caBundle
	Expect(cli.Patch(ctx, crd, runtimeCli.MergeFrom(before))).To(Succeed())
}

func cleanupWebhookInfra(ctx context.Context, cli runtimeCli.Client, ns string) {
	mwc := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: webhookMWCName},
	}
	_ = cli.Delete(ctx, mwc)

	_ = cli.Delete(ctx, &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: webhookSecretName, Namespace: ns}})
	_ = cli.Delete(ctx, &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: webhookServiceName, Namespace: ns}})

	for _, wh := range webhookResources {
		crd := &apiextensionsv1.CustomResourceDefinition{}
		if err := cli.Get(ctx, runtimeCli.ObjectKey{Name: wh.resource + ".rhtas.redhat.com"}, crd); err != nil {
			continue
		}
		before := crd.DeepCopy()
		crd.Spec.Conversion.Webhook.ClientConfig.CABundle = nil
		_ = cli.Patch(ctx, crd, runtimeCli.MergeFrom(before))
	}
}
