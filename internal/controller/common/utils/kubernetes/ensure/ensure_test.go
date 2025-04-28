package ensure

import (
	"context"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	consolev1 "github.com/openshift/api/console/v1"
	routev1 "github.com/openshift/api/route/v1"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/annotations"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_Ensure(t *testing.T) {
	addAnnotations := func(object client.Object, annotations map[string]string) client.Object {
		object.SetAnnotations(annotations)
		return object
	}

	ctx := context.Background()
	type env struct {
		objects []client.Object
	}
	tests := []struct {
		name   string
		object client.Object
		verify func(Gomega, client.WithWatch, controllerutil.OperationResult, error)
		env    env
	}{
		{
			name:   "create new object",
			object: kubernetes.CreateService("default", "service", "http", 80, 80, map[string]string{}),
			verify: func(g Gomega, cli client.WithWatch, result controllerutil.OperationResult, err error) {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(result).To(Equal(controllerutil.OperationResultCreated))
				nn := types.NamespacedName{
					Namespace: "default",
					Name:      "service",
				}
				obj := &v1.Service{}
				g.Expect(cli.Get(context.TODO(), nn, obj)).To(Succeed())
				g.Expect(obj.Spec.Ports).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Port":       Equal(int32(80)),
					"TargetPort": Equal(intstr.FromInt32(80)),
				})))
			},
		},
		{
			name: "update: labels",
			env: env{
				objects: []client.Object{
					kubernetes.CreateService("default", "service", "http", 80, 80, map[string]string{
						"old": "label",
					}),
				},
			},
			object: kubernetes.CreateService("default", "service", "http", 80, 80, map[string]string{
				"new": "label",
			}),
			verify: func(g Gomega, cli client.WithWatch, result controllerutil.OperationResult, err error) {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(result).To(Equal(controllerutil.OperationResultUpdated))
				nn := types.NamespacedName{
					Namespace: "default",
					Name:      "service",
				}
				obj := &v1.Service{}
				g.Expect(cli.Get(context.TODO(), nn, obj)).To(Succeed())
				g.Expect(obj.Labels).Should(HaveKeyWithValue("new", "label"))
				g.Expect(obj.Labels).ShouldNot(HaveKey("old"))

				g.Expect(obj.Spec.Ports).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Port":       Equal(int32(80)),
					"TargetPort": Equal(intstr.FromInt32(80)),
				})))
			},
		},
		{
			name: "remove managed label",
			env: env{
				objects: []client.Object{
					kubernetes.CreateService("default", "service", "http", 80, 80, map[string]string{
						"managed":   "value",
						"unmanaged": "value",
					}),
				},
			},
			object: kubernetes.CreateService("default", "service", "http", 80, 80, map[string]string{
				"unmanaged": "value",
			}),
			verify: func(g Gomega, cli client.WithWatch, result controllerutil.OperationResult, err error) {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(result).To(Equal(controllerutil.OperationResultUpdated))
				nn := types.NamespacedName{
					Namespace: "default",
					Name:      "service",
				}
				obj := &v1.Service{}
				g.Expect(cli.Get(context.TODO(), nn, obj)).To(Succeed())
				g.Expect(obj.Labels).Should(HaveKeyWithValue("unmanaged", "value"))
				g.Expect(obj.Labels).ShouldNot(HaveKeyWithValue("managed", "value"))
				g.Expect(obj.Spec.Ports).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Port":       Equal(int32(80)),
					"TargetPort": Equal(intstr.FromInt32(80)),
				})))
			},
		},
		{
			name: "update: annotations",
			env: env{
				objects: []client.Object{
					addAnnotations(
						kubernetes.CreateService("default", "service", "http", 80, 80, map[string]string{}),
						map[string]string{
							"old": "annotation",
						},
					),
				},
			},
			object: addAnnotations(
				kubernetes.CreateService("default", "service", "http", 80, 80, map[string]string{}),
				map[string]string{
					"new": "annotation",
				}),
			verify: func(g Gomega, cli client.WithWatch, result controllerutil.OperationResult, err error) {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(result).To(Equal(controllerutil.OperationResultUpdated))
				nn := types.NamespacedName{
					Namespace: "default",
					Name:      "service",
				}
				obj := &v1.Service{}
				g.Expect(cli.Get(context.TODO(), nn, obj)).To(Succeed())
				g.Expect(obj.Annotations).Should(HaveKeyWithValue("new", "annotation"))
				g.Expect(obj.Annotations).ShouldNot(HaveKey("old"))

				g.Expect(obj.Spec.Ports).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Port":       Equal(int32(80)),
					"TargetPort": Equal(intstr.FromInt32(80)),
				})))
			},
		},
		{
			name: "remove managed annotation",
			env: env{
				objects: []client.Object{
					addAnnotations(
						kubernetes.CreateService("default", "service", "http", 80, 80, map[string]string{}),
						map[string]string{
							"managed":   "value",
							"unmanaged": "value",
						},
					),
				},
			},
			object: addAnnotations(kubernetes.CreateService("default", "service", "http", 80, 80, map[string]string{}), map[string]string{}),
			verify: func(g Gomega, cli client.WithWatch, result controllerutil.OperationResult, err error) {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(result).To(Equal(controllerutil.OperationResultUpdated))
				nn := types.NamespacedName{
					Namespace: "default",
					Name:      "service",
				}
				obj := &v1.Service{}
				g.Expect(cli.Get(context.TODO(), nn, obj)).To(Succeed())
				g.Expect(obj.Annotations).Should(HaveKeyWithValue("unmanaged", "value"))
				g.Expect(obj.Annotations).ShouldNot(HaveKeyWithValue("managed", "value"))

				g.Expect(obj.Spec.Ports).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Port":       Equal(int32(80)),
					"TargetPort": Equal(intstr.FromInt32(80)),
				})))
			},
		},
		{
			name: "update: different spec",
			env: env{
				objects: []client.Object{
					kubernetes.CreateService("default", "service", "http", 80, 80, map[string]string{}),
				},
			},
			object: kubernetes.CreateService("default", "service", "https", 443, 443, map[string]string{}),
			verify: func(g Gomega, cli client.WithWatch, result controllerutil.OperationResult, err error) {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(result).To(Equal(controllerutil.OperationResultUpdated))
				nn := types.NamespacedName{
					Namespace: "default",
					Name:      "service",
				}
				obj := &v1.Service{}
				g.Expect(cli.Get(context.TODO(), nn, obj)).To(Succeed())

				g.Expect(obj.Spec.Ports).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Port":       Equal(int32(443)),
					"TargetPort": Equal(intstr.FromInt32(443)),
				})))
				g.Expect(obj.Spec.Ports).ShouldNot(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Port":       Equal(int32(80)),
					"TargetPort": Equal(intstr.FromInt32(80)),
				})))
			},
		},
		{
			name: "not update: status",
			env: env{
				objects: []client.Object{
					&rhtasv1alpha1.Securesign{
						ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
						Status: rhtasv1alpha1.SecuresignStatus{
							RekorStatus: rhtasv1alpha1.SecuresignRekorStatus{
								Url: "old status",
							},
						},
					},
				},
			},
			object: &rhtasv1alpha1.Securesign{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Status: rhtasv1alpha1.SecuresignStatus{
					RekorStatus: rhtasv1alpha1.SecuresignRekorStatus{
						Url: "new status",
					},
				},
			},
			verify: func(g Gomega, cli client.WithWatch, result controllerutil.OperationResult, err error) {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(result).To(Equal(controllerutil.OperationResultNone))
				nn := types.NamespacedName{
					Namespace: "default",
					Name:      "test",
				}
				obj := &rhtasv1alpha1.Securesign{}
				g.Expect(cli.Get(context.TODO(), nn, obj)).To(Succeed())
				g.Expect(obj.Status.RekorStatus.Url).To(Equal("old status"))
			},
		},
		{
			name: "not update: same spec",
			env: env{
				objects: []client.Object{
					kubernetes.CreateService("default", "service", "http", 80, 80, map[string]string{}),
				},
			},
			object: kubernetes.CreateService("default", "service", "http", 80, 80, map[string]string{}),
			verify: func(g Gomega, cli client.WithWatch, result controllerutil.OperationResult, err error) {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(result).To(Equal(controllerutil.OperationResultNone))
				nn := types.NamespacedName{
					Namespace: "default",
					Name:      "service",
				}
				obj := &v1.Service{}
				g.Expect(cli.Get(context.TODO(), nn, obj)).To(Succeed())

				g.Expect(obj.Spec.Ports).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Port":       Equal(int32(80)),
					"TargetPort": Equal(intstr.FromInt32(80)),
				})))
			},
		},
		{
			name: "not updated: pause reconciliation == true",
			env: env{
				objects: []client.Object{
					addAnnotations(
						kubernetes.CreateService("default", "service", "http", 80, 80, map[string]string{}),
						map[string]string{
							annotations.PausedReconciliation: "true",
						}),
				},
			},
			object: kubernetes.CreateService("default", "service", "http", 443, 443, map[string]string{}),
			verify: func(g Gomega, cli client.WithWatch, result controllerutil.OperationResult, err error) {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(result).To(Equal(controllerutil.OperationResultNone))
				nn := types.NamespacedName{
					Namespace: "default",
					Name:      "service",
				}
				obj := &v1.Service{}
				g.Expect(cli.Get(context.TODO(), nn, obj)).To(Succeed())

				g.Expect(obj.Spec.Ports).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Port":       Equal(int32(80)),
					"TargetPort": Equal(intstr.FromInt32(80)),
				})))
			},
		},
		{
			name: "updated: pause reconciliation == false",
			env: env{
				objects: []client.Object{
					addAnnotations(
						kubernetes.CreateService("default", "service", "http", 80, 80, map[string]string{}),
						map[string]string{
							annotations.PausedReconciliation: "false",
						}),
				},
			},
			object: kubernetes.CreateService("default", "service", "http", 443, 443, map[string]string{}),
			verify: func(g Gomega, cli client.WithWatch, result controllerutil.OperationResult, err error) {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(result).To(Equal(controllerutil.OperationResultUpdated))
				nn := types.NamespacedName{
					Namespace: "default",
					Name:      "service",
				}
				obj := &v1.Service{}
				g.Expect(cli.Get(context.TODO(), nn, obj)).To(Succeed())

				g.Expect(obj.Spec.Ports).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Port":       Equal(int32(443)),
					"TargetPort": Equal(intstr.FromInt32(443)),
				})))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			c := fakeClientBuilder().
				WithObjects(tt.env.objects...).
				WithStatusSubresource(tt.env.objects...).
				Build()

			managed := []string{"new", "old", "managed"}
			got, err := kubernetes.CreateOrUpdate(ctx, c, tt.object.DeepCopyObject().(client.Object),
				Labels[client.Object](managed, tt.object.GetLabels()),
				Annotations[client.Object](managed, tt.object.GetAnnotations()),
				func(obj client.Object) error {
					var (
						svc *v1.Service
						ok  bool
					)
					if svc, ok = obj.(*v1.Service); ok {
						// svc object
						svc.Spec.Ports = tt.object.(*v1.Service).Spec.Ports
					}

					return nil

				},
			)
			tt.verify(g, c, got, err)
		})
	}
}

func fakeClientBuilder() *fake.ClientBuilder {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(rhtasv1alpha1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	utilruntime.Must(v1.AddToScheme(scheme))
	utilruntime.Must(consolev1.AddToScheme(scheme))
	utilruntime.Must(apiextensions.AddToScheme(scheme))
	cl := fake.NewClientBuilder().WithScheme(scheme)
	return cl
}
