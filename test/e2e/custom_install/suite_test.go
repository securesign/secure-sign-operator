//go:build custom_install

package custom_install

import (
	"context"
	"embed"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/securesign/operator/test/e2e/support"
	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/api/rbac/v1"
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
	Expect(cli.Create(ctx, managerPod(ns, opts...))).To(Succeed())
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
					Env: []v1.EnvVar{
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
