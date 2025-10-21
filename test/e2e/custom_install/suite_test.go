//go:build custom_install

package custom_install

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/securesign/operator/test/e2e/support"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/utils/ptr"

	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

//go:embed testdata/*
var testdata embed.FS

const managerPodName = "manager"

func TestCustomInstall(t *testing.T) {
	RegisterFailHandler(Fail)
	log.SetLogger(GinkgoLogr)
	SetDefaultEventuallyTimeout(time.Duration(3) * time.Minute)
	EnforceDefaultTimeoutsWhenUsingContexts()
	RunSpecs(t, "With customized install")

	// print whole stack in case of failure
	format.MaxLength = 0
}

func uninstallOperator(ctx context.Context, cli runtimeCli.Client, namespace string) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managerPodName,
			Namespace: namespace,
		},
	}
	_ = cli.Delete(ctx, pod)
	Eventually(func(ctx context.Context) error {
		return cli.Get(ctx, runtimeCli.ObjectKeyFromObject(pod), &v1.Pod{})
	}).WithContext(ctx).Should(And(HaveOccurred(), WithTransform(apierrors.IsNotFound, BeTrue())))
}

func installOperator(ctx context.Context, cli runtimeCli.Client, ns string, opts ...optManagerPod) {
	for _, o := range rbac(ns) {
		c := o.DeepCopyObject().(runtimeCli.Object)
		if e := cli.Get(ctx, runtimeCli.ObjectKeyFromObject(o), c); !apierrors.IsNotFound(e) {
			Expect(cli.Delete(ctx, o)).To(Succeed())
		}
		Expect(cli.Create(ctx, o)).To(Succeed())
	}

	for _, o := range webhookInfra(ns) {
		c := o.DeepCopyObject().(runtimeCli.Object)
		if e := cli.Get(ctx, runtimeCli.ObjectKeyFromObject(o), c); !apierrors.IsNotFound(e) {
			Expect(cli.Delete(ctx, o)).To(Succeed())
		}
		Expect(cli.Create(ctx, o)).To(Succeed())
	}

	Expect(cli.Create(ctx, managerPod(ns, opts...))).To(Succeed())

	time.Sleep(1 * time.Minute)

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
					Ports: []v1.ContainerPort{
						{
							ContainerPort: 9443,
							Name:          "webhook-port",
						},
					},
					Env: []v1.EnvVar{
						{
							Name:  "OPENSHIFT",
							Value: support.EnvOrDefault("OPENSHIFT", "false"),
						},
					},
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      "webhook-cert",
							ReadOnly:  true,
							MountPath: "/tmp/k8s-webhook-server/serving-certs",
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
			Volumes: []v1.Volume{
				{
					Name: "webhook-cert",
					VolumeSource: v1.VolumeSource{
						Secret: &v1.SecretVolumeSource{
							SecretName: "webhook-server-tls",
						},
					},
				},
			},
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
		if err := yamlutil.Unmarshal(bytes, &u.Object); err != nil {
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

const (
	CertResourcesPath    = "../../../config/overlays/kubernetes/cert_resources.yaml"
	WebhookServicePath   = "../../../config/webhook/service.yaml"
	WebhookConfigPath    = "../../../config/webhook/webhook.yaml"
	CertManagerPatchPath = "../../../config/overlays/kubernetes/kubernetes_webhook_patch.yaml"
)

func applyCertManagerAnnotationPatch(u *unstructured.Unstructured, ns string) {
	patchBytes, err := os.ReadFile(CertManagerPatchPath)
	if err != nil {
		Fail(fmt.Errorf("failed to read CertManager patch file: %w", err).Error())
	}

	patch := &admissionv1.ValidatingWebhookConfiguration{}

	if err := yamlutil.Unmarshal(patchBytes, patch); err != nil {
		Fail(fmt.Errorf("failed to unmarshal patch YAML: %w", err).Error())
	}

	baseAnnotations := u.GetAnnotations()
	if baseAnnotations == nil {
		baseAnnotations = make(map[string]string)
	}

	const injectionAnnotationKey = "cert-manager.io/inject-ca-from"
	originalAnnotationValue := patch.GetAnnotations()[injectionAnnotationKey]

	parts := strings.Split(originalAnnotationValue, "/")
	certName := parts[len(parts)-1]

	newAnnotationValue := fmt.Sprintf("%s/%s", ns, certName)

	baseAnnotations[injectionAnnotationKey] = newAnnotationValue

	u.SetAnnotations(baseAnnotations)

	GinkgoWriter.Printf("Patched Webhook Config %s with Cert-Manager annotation.\n", u.GetName())
}

func webhookInfra(ns string) []runtimeCli.Object {
	files := []string{
		CertResourcesPath,  // Cert-Manager Issuer & Certificate
		WebhookServicePath, // The namespaced Service
		WebhookConfigPath,  // The cluster-scoped ValidatingWebhookConfiguration
	}
	var objects = make([]runtimeCli.Object, 0)

	for _, f := range files {
		bytes, err := os.ReadFile(f)
		if err != nil {
			Fail(fmt.Errorf("failed to read file %s: %w", f, err).Error())
		}

		decoder := yamlutil.NewYAMLOrJSONDecoder(strings.NewReader(string(bytes)), 4096)

		for {
			u := &unstructured.Unstructured{Object: make(map[string]interface{})}

			if err := decoder.Decode(&u.Object); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				Fail(fmt.Errorf("failed to decode YAML from %s: %w", f, err).Error())
			}

			kind := u.GetKind()

			if kind != "ValidatingWebhookConfiguration" {
				u.SetNamespace(ns)
			}

			if kind == "ValidatingWebhookConfiguration" {
				applyCertManagerAnnotationPatch(u, ns)
				webhooks, found, err := unstructured.NestedSlice(u.Object, "webhooks")
				if !found || err != nil {
					Fail(fmt.Errorf("webhook config structure missing 'webhooks' slice: %w", err).Error())
				}

				webhook := webhooks[0].(map[string]interface{})
				clientConfig := webhook["clientConfig"].(map[string]interface{})
				service := clientConfig["service"].(map[string]interface{})

				service["namespace"] = ns

				webhooks[0] = webhook
				err = unstructured.SetNestedSlice(u.Object, webhooks, "webhooks")

				if err != nil {
					Fail(fmt.Errorf("failed to set Namespace on ValidatingWebhookConfiguration resource: %w", err).Error())
				}

			}

			if kind == "Certificate" {
				const dynamicServiceName = "controller-manager-webhook-service"

				fqdn1 := fmt.Sprintf("%s.%s.svc", dynamicServiceName, ns)
				fqdn2 := fmt.Sprintf("%s.%s.svc.cluster.local", dynamicServiceName, ns)

				err := unstructured.SetNestedStringSlice(
					u.Object,
					[]string{fqdn1, fqdn2},
					"spec",
					"dnsNames",
				)
				if err != nil {
					Fail(fmt.Errorf("failed to set dnsNames on Certificate resource: %w", err).Error())
				}
			}

			objects = append(objects, u)
		}
	}

	return objects
}
