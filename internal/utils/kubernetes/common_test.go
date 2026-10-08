package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/config"
	testAction "github.com/securesign/operator/internal/testing/action"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestCalculateHostname(t *testing.T) {
	tests := []struct {
		name     string
		template string
		svcName  string
		ns       string
		expected string
	}{
		{
			name:     "default template produces static .local hostname",
			template: "%[1]s.local",
			svcName:  "rekor-server",
			ns:       "test-ns",
			expected: "rekor-server.local",
		},
		{
			name:     "namespace-scoped template includes namespace",
			template: "%[1]s.%[2]s.127.0.0.1.nip.io",
			svcName:  "rekor-server",
			ns:       "test-ns",
			expected: "rekor-server.test-ns.127.0.0.1.nip.io",
		},
		{
			name:     "custom template with different format",
			template: "%[1]s-%[2]s.example.com",
			svcName:  "fulcio-server",
			ns:       "my-namespace",
			expected: "fulcio-server-my-namespace.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := config.IngressHostTemplate
			origOpenshift := config.Openshift
			t.Cleanup(func() {
				config.IngressHostTemplate = original
				config.Openshift = origOpenshift
			})

			config.IngressHostTemplate = tt.template
			config.Openshift = false

			result, err := CalculateHostname(context.Background(), nil, tt.svcName, tt.ns)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestCalculateHostname_OpenShift(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(configv1.AddToScheme(scheme))

	tests := []struct {
		name     string
		domain   string
		svcName  string
		ns       string
		expected string
	}{
		{
			name:     "OpenShift hostname includes namespace and cluster domain",
			domain:   "apps.cluster.example.com",
			svcName:  "rekor-server",
			ns:       "test-ns",
			expected: "rekor-server-test-ns.apps.cluster.example.com",
		},
		{
			name:     "OpenShift hostname with different service and namespace",
			domain:   "apps.ocp.internal",
			svcName:  "fulcio-server",
			ns:       "secure-sign",
			expected: "fulcio-server-secure-sign.apps.ocp.internal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origOpenshift := config.Openshift
			t.Cleanup(func() { config.Openshift = origOpenshift })
			config.Openshift = true

			cli := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(&configv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
					Spec:       configv1.IngressSpec{Domain: tt.domain},
				}).
				Build()

			result, err := CalculateHostname(context.Background(), cli, tt.svcName, tt.ns)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestCalculateHostname_OpenShift_MissingIngress(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(configv1.AddToScheme(scheme))

	origOpenshift := config.Openshift
	t.Cleanup(func() { config.Openshift = origOpenshift })
	config.Openshift = true

	cli := fake.NewClientBuilder().WithScheme(scheme).Build()

	_, err := CalculateHostname(context.Background(), cli, "rekor-server", "test-ns")
	if err == nil {
		t.Fatal("expected error when cluster Ingress is missing, got nil")
	}
}

func TestCreate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		mutateErr error
		intercept interceptor.Funcs
		wantErr   bool
	}{
		{
			name: "applies mutate fns and creates, without probing for an existing object",
			intercept: interceptor.Funcs{
				Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
					return fmt.Errorf("Get should never be called by Create")
				},
			},
		},
		{
			name:      "mutate fn error is returned and Create is not called",
			mutateErr: fmt.Errorf("mutate failed"),
			intercept: interceptor.Funcs{
				Create: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.CreateOption) error {
					return fmt.Errorf("Create should not be called when a mutate fn fails")
				},
			},
			wantErr: true,
		},
		{
			name: "client Create error is propagated",
			intercept: interceptor.Funcs{
				Create: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.CreateOption) error {
					return fmt.Errorf("api server unavailable")
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := gomega.NewWithT(t)
			c := testAction.FakeClientBuilder().
				WithInterceptorFuncs(tt.intercept).
				Build()

			obj := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-secret-",
					Namespace:    "default",
				},
			}

			err := Create(t.Context(), c, obj,
				func(s *corev1.Secret) error {
					if tt.mutateErr != nil {
						return tt.mutateErr
					}
					s.Labels = map[string]string{"foo": "bar"}
					return nil
				},
			)

			if tt.wantErr {
				g.Expect(err).To(gomega.HaveOccurred())
				return
			}
			g.Expect(err).ToNot(gomega.HaveOccurred())
			g.Expect(obj.Name).ToNot(gomega.BeEmpty())
			g.Expect(obj.Labels).To(gomega.Equal(map[string]string{"foo": "bar"}))
		})
	}
}
func TestApply(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	serviceApplyConfig := func(name, ns string, port int32, labels map[string]string) *corev1ac.ServiceApplyConfiguration {
		return corev1ac.Service(name, ns).
			WithLabels(labels).
			WithSpec(corev1ac.ServiceSpec().
				WithSelector(labels).
				WithPorts(
					corev1ac.ServicePort().
						WithName("http").
						WithProtocol(v1.ProtocolTCP).
						WithPort(port).
						WithTargetPort(intstr.FromInt32(port)),
				),
			)
	}

	tests := []struct {
		name      string
		existing  []client.Object
		applyObj  *corev1ac.ServiceApplyConfiguration
		intercept interceptor.Funcs
		verify    func(gomega.Gomega, client.WithWatch, bool, error)
	}{
		{
			name:     "create: new object reports changed",
			applyObj: serviceApplyConfig("test-svc", "default", 80, map[string]string{"app": "test"}),
			verify: func(g gomega.Gomega, cli client.WithWatch, changed bool, err error) {
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(changed).To(gomega.BeTrue())

				svc := &v1.Service{}
				g.Expect(cli.Get(ctx, types.NamespacedName{Name: "test-svc", Namespace: "default"}, svc)).To(gomega.Succeed())
				g.Expect(svc.Labels).To(gomega.HaveKeyWithValue("app", "test"))
				g.Expect(svc.Spec.Ports).To(gomega.ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Port": gomega.Equal(int32(80)),
				})))
				g.Expect(hasFieldManager(svc.ManagedFields, FieldManager)).To(gomega.BeTrue(),
					"expected %q to own fields on create", FieldManager)
			},
		},
		{
			name: "update: changed spec reports changed",
			existing: []client.Object{
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{Name: "http", Port: 80, Protocol: v1.ProtocolTCP, TargetPort: intstr.FromInt32(80)}},
					},
				},
			},
			applyObj: serviceApplyConfig("test-svc", "default", 443, map[string]string{"app": "test"}),
			verify: func(g gomega.Gomega, cli client.WithWatch, changed bool, err error) {
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(changed).To(gomega.BeTrue())

				svc := &v1.Service{}
				g.Expect(cli.Get(ctx, types.NamespacedName{Name: "test-svc", Namespace: "default"}, svc)).To(gomega.Succeed())
				g.Expect(svc.Spec.Ports).To(gomega.ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Port": gomega.Equal(int32(443)),
				})))
			},
		},
		{
			name: "pause annotation true: skip apply",
			existing: []client.Object{
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-svc",
						Namespace: "default",
						Annotations: map[string]string{
							annotations.PausedReconciliation: "true",
						},
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{Name: "http", Port: 80, Protocol: v1.ProtocolTCP, TargetPort: intstr.FromInt32(80)}},
					},
				},
			},
			applyObj: serviceApplyConfig("test-svc", "default", 443, map[string]string{"app": "test"}),
			verify: func(g gomega.Gomega, cli client.WithWatch, changed bool, err error) {
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(changed).To(gomega.BeFalse())

				svc := &v1.Service{}
				g.Expect(cli.Get(ctx, types.NamespacedName{Name: "test-svc", Namespace: "default"}, svc)).To(gomega.Succeed())
				g.Expect(svc.Spec.Ports).To(gomega.ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Port": gomega.Equal(int32(80)),
				})))
			},
		},
		{
			name: "pause annotation false: apply proceeds",
			existing: []client.Object{
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-svc",
						Namespace: "default",
						Annotations: map[string]string{
							annotations.PausedReconciliation: "false",
						},
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{Name: "http", Port: 80, Protocol: v1.ProtocolTCP, TargetPort: intstr.FromInt32(80)}},
					},
				},
			},
			applyObj: serviceApplyConfig("test-svc", "default", 443, map[string]string{"app": "test"}),
			verify: func(g gomega.Gomega, cli client.WithWatch, changed bool, err error) {
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(changed).To(gomega.BeTrue())

				svc := &v1.Service{}
				g.Expect(cli.Get(ctx, types.NamespacedName{Name: "test-svc", Namespace: "default"}, svc)).To(gomega.Succeed())
				g.Expect(svc.Spec.Ports).To(gomega.ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Port": gomega.Equal(int32(443)),
				})))
			},
		},
		{
			name: "force ownership: takes fields from foreign manager",
			existing: []client.Object{
				withManagedFields(&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-svc",
						Namespace: "default",
						Labels:    map[string]string{"foreign": "label"},
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{Name: "http", Port: 80, Protocol: v1.ProtocolTCP, TargetPort: intstr.FromInt32(80)}},
					},
				}, "foreign-controller", metav1.ManagedFieldsOperationApply),
			},
			applyObj: serviceApplyConfig("test-svc", "default", 443, map[string]string{"app": "ours"}),
			verify: func(g gomega.Gomega, cli client.WithWatch, changed bool, err error) {
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(changed).To(gomega.BeTrue())

				svc := &v1.Service{}
				g.Expect(cli.Get(ctx, types.NamespacedName{Name: "test-svc", Namespace: "default"}, svc)).To(gomega.Succeed())
				g.Expect(svc.Labels).To(gomega.HaveKeyWithValue("app", "ours"))
				g.Expect(svc.Labels).To(gomega.HaveKeyWithValue("foreign", "label"))
				g.Expect(svc.Spec.Ports).To(gomega.ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Port": gomega.Equal(int32(443)),
				})))
				g.Expect(hasFieldManager(svc.ManagedFields, FieldManager)).To(gomega.BeTrue(),
					"expected %q to own fields after ForceOwnership", FieldManager)
			},
		},
		{
			name: "legacy CSA manager: migration converts ownership",
			existing: []client.Object{
				withManagedFields(&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-svc",
						Namespace: "default",
						Labels:    map[string]string{"app": "legacy"},
					},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{Name: "http", Port: 80, Protocol: v1.ProtocolTCP, TargetPort: intstr.FromInt32(80)}},
					},
				}, LegacyFieldManager, metav1.ManagedFieldsOperationUpdate),
			},
			applyObj: serviceApplyConfig("test-svc", "default", 443, map[string]string{"app": "migrated"}),
			verify: func(g gomega.Gomega, cli client.WithWatch, changed bool, err error) {
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(changed).To(gomega.BeTrue())

				svc := &v1.Service{}
				g.Expect(cli.Get(ctx, types.NamespacedName{Name: "test-svc", Namespace: "default"}, svc)).To(gomega.Succeed())
				g.Expect(svc.Labels).To(gomega.HaveKeyWithValue("app", "migrated"))
				g.Expect(svc.Spec.Ports).To(gomega.ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Port": gomega.Equal(int32(443)),
				})))
				g.Expect(hasFieldManager(svc.ManagedFields, FieldManager)).To(gomega.BeTrue(),
					"expected %q to own fields after legacy migration", FieldManager)
				g.Expect(hasFieldManager(svc.ManagedFields, LegacyFieldManager)).To(gomega.BeFalse(),
					"expected legacy %q to be removed after migration", LegacyFieldManager)
			},
		},
		{
			name:     "client apply error is propagated",
			applyObj: serviceApplyConfig("test-svc", "default", 80, map[string]string{"app": "test"}),
			intercept: interceptor.Funcs{
				Apply: func(_ context.Context, _ client.WithWatch, _ runtime.ApplyConfiguration, _ ...client.ApplyOption) error {
					return fmt.Errorf("api server unavailable")
				},
			},
			verify: func(g gomega.Gomega, _ client.WithWatch, _ bool, err error) {
				g.Expect(err).To(gomega.HaveOccurred())
				g.Expect(err.Error()).To(gomega.ContainSubstring("api server unavailable"))
			},
		},
		{
			name:     "client get error is propagated",
			applyObj: serviceApplyConfig("test-svc", "default", 80, map[string]string{"app": "test"}),
			intercept: interceptor.Funcs{
				Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
					return fmt.Errorf("api server unavailable")
				},
			},
			verify: func(g gomega.Gomega, _ client.WithWatch, _ bool, err error) {
				g.Expect(err).To(gomega.HaveOccurred())
				g.Expect(err.Error()).To(gomega.ContainSubstring("api server unavailable"))
			},
		},
		{
			name: "owner reference is preserved through apply",
			existing: []client.Object{
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{{Name: "http", Port: 80, Protocol: v1.ProtocolTCP, TargetPort: intstr.FromInt32(80)}},
					},
				},
			},
			applyObj: serviceApplyConfig("test-svc", "default", 443, map[string]string{"app": "test"}).
				WithOwnerReferences(metav1ac.OwnerReference().
					WithAPIVersion("v1").
					WithKind("ConfigMap").
					WithName("owner").
					WithUID("owner-uid").
					WithController(true).
					WithBlockOwnerDeletion(true),
				),
			verify: func(g gomega.Gomega, cli client.WithWatch, changed bool, err error) {
				g.Expect(err).ToNot(gomega.HaveOccurred())
				g.Expect(changed).To(gomega.BeTrue())

				svc := &v1.Service{}
				g.Expect(cli.Get(ctx, types.NamespacedName{Name: "test-svc", Namespace: "default"}, svc)).To(gomega.Succeed())
				g.Expect(svc.OwnerReferences).To(gomega.HaveLen(1))
				g.Expect(svc.OwnerReferences[0].Name).To(gomega.Equal("owner"))
				g.Expect(*svc.OwnerReferences[0].Controller).To(gomega.BeTrue())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := gomega.NewWithT(t)

			builder := testAction.FakeClientBuilder().WithReturnManagedFields()
			if len(tt.existing) > 0 {
				builder = builder.WithObjects(tt.existing...)
			}
			if tt.intercept.Get != nil || tt.intercept.Apply != nil || tt.intercept.Patch != nil || tt.intercept.Create != nil {
				builder = builder.WithInterceptorFuncs(tt.intercept)
			}
			cli := builder.Build()

			live := &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"}}
			changed, err := Apply(ctx, cli, tt.applyObj, live)

			tt.verify(g, cli, changed, err)
		})
	}
}

func hasFieldManager(managedFields []metav1.ManagedFieldsEntry, manager string) bool {
	for _, mf := range managedFields {
		if mf.Manager == manager {
			return true
		}
	}
	return false
}

func withManagedFields(obj *v1.Service, manager string, op metav1.ManagedFieldsOperationType) *v1.Service {
	fieldsJSON, _ := json.Marshal(map[string]interface{}{
		"f:metadata": map[string]interface{}{
			"f:labels": map[string]interface{}{},
		},
		"f:spec": map[string]interface{}{
			"f:ports": map[string]interface{}{},
		},
	})
	raw := json.RawMessage(fieldsJSON)
	obj.ManagedFields = []metav1.ManagedFieldsEntry{
		{
			Manager:    manager,
			Operation:  op,
			APIVersion: "v1",
			FieldsType: "FieldsV1",
			FieldsV1:   &metav1.FieldsV1{Raw: raw},
		},
	}
	return obj
}
