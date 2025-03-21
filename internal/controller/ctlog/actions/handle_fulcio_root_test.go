package actions

import (
	"context"
	"reflect"
	"testing"

	testAction "github.com/securesign/operator/internal/testing/action"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/fulcio/actions"
	"github.com/securesign/operator/internal/controller/labels"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCertCan_Handle(t *testing.T) {

	type env struct {
		phase        string
		certificates []v1alpha1.SecretKeySelector
		objects      []client.Object
		status       v1alpha1.CTlogStatus
	}
	type want struct {
		canHandle bool
	}
	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "update spec key",
			env: env{
				phase: constants.Creating,
				certificates: []v1alpha1.SecretKeySelector{
					{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "secret"},
						Key:                  "key",
					},
				},
				status: v1alpha1.CTlogStatus{
					RootCertificates: []v1alpha1.SecretKeySelector{
						{
							LocalObjectReference: v1alpha1.LocalObjectReference{Name: "fake"},
							Key:                  "fake",
						},
					},
				},
			},
			want: want{
				canHandle: true,
			},
		},
		{
			name: "new spec key",
			env: env{
				phase: constants.Creating,
				certificates: []v1alpha1.SecretKeySelector{
					{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "secret"},
						Key:                  "key",
					},
				},
				status: v1alpha1.CTlogStatus{},
			},
			want: want{
				canHandle: true,
			},
		},
		{
			name: "autodiscovery new fulcio-cert",
			env: env{
				phase:        constants.Creating,
				certificates: nil,
				status:       v1alpha1.CTlogStatus{},
				objects: []client.Object{
					kubernetes.CreateSecret("secret", "default",
						map[string][]byte{"key": nil}, map[string]string{actions.FulcioCALabel: "key"}),
				},
			},
			want: want{
				canHandle: true,
			},
		},
		{
			name: "autodiscovery update fulcio-cert - ready phase",
			env: env{
				phase:        constants.Ready,
				certificates: nil,
				status: v1alpha1.CTlogStatus{
					RootCertificates: []v1alpha1.SecretKeySelector{
						{
							LocalObjectReference: v1alpha1.LocalObjectReference{Name: "fake"},
							Key:                  "fake",
						},
					},
				},
				objects: []client.Object{
					kubernetes.CreateSecret("secret", "default",
						map[string][]byte{"key": nil}, map[string]string{actions.FulcioCALabel: "key"}),
				},
			},
			want: want{
				canHandle: true,
			},
		},
		{
			name: "pending phase",
			env: env{
				phase: constants.Pending,
			},
			want: want{
				canHandle: false,
			},
		},
		{
			name: "matching cert-set",
			env: env{
				phase: constants.Creating,
				certificates: []v1alpha1.SecretKeySelector{
					{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "secret"},
						Key:                  "key",
					},
				},
				status: v1alpha1.CTlogStatus{
					RootCertificates: []v1alpha1.SecretKeySelector{
						{
							LocalObjectReference: v1alpha1.LocalObjectReference{Name: "secret"},
							Key:                  "key",
						},
					},
				},
			},
			want: want{
				canHandle: false,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			c := testAction.FakeClientBuilder().
				WithObjects(tt.env.objects...).
				Build()
			a := testAction.PrepareAction(c, NewHandleFulcioCertAction())

			instance := v1alpha1.CTlog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "instance",
					Namespace: "default",
				},
				Spec: v1alpha1.CTlogSpec{
					RootCertificates: tt.env.certificates,
				},
				Status: tt.env.status,
			}
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:   constants.Ready,
				Reason: tt.env.phase,
			})

			if got := a.CanHandle(context.TODO(), &instance); !reflect.DeepEqual(got, tt.want.canHandle) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want.canHandle)
			}
		})
	}
}
func TestCert_Handle(t *testing.T) {

	type env struct {
		certificates []v1alpha1.SecretKeySelector
		objects      []client.Object
		status       v1alpha1.CTlogStatus
	}
	type want struct {
		result *action.Result
		verify func(Gomega, v1alpha1.CTlogStatus, client.WithWatch, <-chan watch.Event)
	}
	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "autodiscover new fulcio-cert",
			env: env{
				certificates: nil,
				status: v1alpha1.CTlogStatus{
					Conditions: []metav1.Condition{
						{Type: constants.Ready, Reason: constants.Creating},
					},
				},
				objects: []client.Object{
					kubernetes.CreateSecret("secret", "default",
						map[string][]byte{"key": nil}, map[string]string{actions.FulcioCALabel: "key"}),
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, status v1alpha1.CTlogStatus, cli client.WithWatch, configWatch <-chan watch.Event) {
					g.Expect(status.ServerConfigRef).Should(BeNil())

					g.Expect(status.RootCertificates).To(HaveLen(1))
					g.Expect(status.RootCertificates).To(ContainElement(v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "secret"},
						Key:                  "key",
					}))

					g.Expect(meta.IsStatusConditionTrue(status.Conditions, CertCondition)).To(BeTrue())
				},
			},
		},
		{
			name: "autodiscover missing cert",
			env: env{
				certificates: nil,
				status: v1alpha1.CTlogStatus{
					Conditions: []metav1.Condition{
						{Type: constants.Ready, Reason: constants.Creating},
					},
				},
			},
			want: want{
				result: testAction.Requeue(),
				verify: func(g Gomega, status v1alpha1.CTlogStatus, cli client.WithWatch, configWatch <-chan watch.Event) {
					g.Expect(status.ServerConfigRef).Should(BeNil())

					g.Expect(status.RootCertificates).To(BeEmpty())

					g.Expect(meta.IsStatusConditionTrue(status.Conditions, CertCondition)).To(BeFalse())
				},
			},
		},
		{
			name: "configured",
			env: env{
				objects: []client.Object{
					kubernetes.CreateSecret("secret", "default", map[string][]byte{"key": nil}, map[string]string{}),
					kubernetes.CreateSecret("secret-2", "default", map[string][]byte{"key": nil}, map[string]string{}),
				},

				certificates: []v1alpha1.SecretKeySelector{
					{
						Key:                  "key",
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "secret"},
					},
					{
						Key:                  "key",
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "secret-2"},
					},
				},
				status: v1alpha1.CTlogStatus{
					Conditions: []metav1.Condition{
						{Type: constants.Ready, Reason: constants.Creating},
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, status v1alpha1.CTlogStatus, cli client.WithWatch, configWatch <-chan watch.Event) {
					g.Expect(status.ServerConfigRef).Should(BeNil())

					g.Expect(status.RootCertificates).Should(HaveLen(2))
					g.Expect(status.RootCertificates[0].Key).Should(Equal("key"))
					g.Expect(status.RootCertificates[0].Name).Should(Equal("secret"))
					g.Expect(status.RootCertificates[1].Key).Should(Equal("key"))
					g.Expect(status.RootCertificates[1].Name).Should(Equal("secret-2"))

					g.Expect(meta.IsStatusConditionTrue(status.Conditions, CertCondition)).To(BeTrue())
				},
			},
		},
		{
			name: "configured take priority",
			env: env{
				certificates: []v1alpha1.SecretKeySelector{
					{
						Key:                  "key",
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "my-secret"},
					},
				},
				objects: []client.Object{
					kubernetes.CreateSecret("my-secret", "default",
						map[string][]byte{"key": nil}, map[string]string{}),
					kubernetes.CreateSecret("incorrect-secret", "default",
						map[string][]byte{"key": nil}, map[string]string{actions.FulcioCALabel: "key"}),
				},
				status: v1alpha1.CTlogStatus{
					Conditions: []metav1.Condition{
						{Type: constants.Ready, Reason: constants.Creating},
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, status v1alpha1.CTlogStatus, cli client.WithWatch, configWatch <-chan watch.Event) {
					g.Expect(status.ServerConfigRef).Should(BeNil())

					g.Expect(status.RootCertificates).Should(HaveLen(1))
					g.Expect(status.RootCertificates[0].Key).Should(Equal("key"))
					g.Expect(status.RootCertificates[0].Name).Should(Equal("my-secret"))

					g.Expect(meta.IsStatusConditionTrue(status.Conditions, CertCondition)).To(BeTrue())
				},
			},
		},
		{
			name: "invalidate server config",
			env: env{
				certificates: []v1alpha1.SecretKeySelector{
					{
						Key:                  "key",
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "my-secret"},
					},
				},
				objects: []client.Object{
					kubernetes.CreateSecret("my-secret", "default", map[string][]byte{"key": nil}, map[string]string{}),
					kubernetes.CreateSecret("ctlog-config", "default", map[string][]byte{}, map[string]string{labels.LabelResource: serverConfigResourceName}),
				},
				status: v1alpha1.CTlogStatus{
					ServerConfigRef: &v1alpha1.LocalObjectReference{Name: "ctlog-config"},
					Conditions: []metav1.Condition{
						{Type: constants.Ready, Reason: constants.Creating},
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, status v1alpha1.CTlogStatus, cli client.WithWatch, configWatch <-chan watch.Event) {
					g.Expect(status.RootCertificates).Should(HaveLen(1))
					g.Expect(status.RootCertificates[0].Key).Should(Equal("key"))
					g.Expect(status.RootCertificates[0].Name).Should(Equal("my-secret"))

					g.Expect(meta.IsStatusConditionTrue(status.Conditions, CertCondition)).To(BeTrue())

					// Config condition should be invalidated
					g.Expect(meta.IsStatusConditionFalse(status.Conditions, ConfigCondition)).Should(BeTrue())
				},
			},
		},
		{
			name: "autodiscovery - add new, keep old cert",
			env: env{
				objects: []client.Object{
					kubernetes.CreateSecret("old", "default", map[string][]byte{"key": nil}, map[string]string{}),
					kubernetes.CreateSecret("new", "default", map[string][]byte{"key": nil}, map[string]string{actions.FulcioCALabel: "key"}),
				},
				status: v1alpha1.CTlogStatus{
					RootCertificates: []v1alpha1.SecretKeySelector{
						{
							LocalObjectReference: v1alpha1.LocalObjectReference{Name: "old"},
							Key:                  "key",
						},
					},
					Conditions: []metav1.Condition{
						{Type: constants.Ready, Reason: constants.Creating},
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, status v1alpha1.CTlogStatus, cli client.WithWatch, configWatch <-chan watch.Event) {
					g.Expect(status.ServerConfigRef).Should(BeNil())

					g.Expect(status.RootCertificates).Should(HaveLen(2))
					g.Expect(status.RootCertificates).
						Should(And(
							ContainElement(WithTransform(func(ks v1alpha1.SecretKeySelector) string { return ks.Name }, Equal("old"))),
							ContainElement(WithTransform(func(ks v1alpha1.SecretKeySelector) string { return ks.Name }, Equal("new"))),
						))
					g.Expect(meta.IsStatusConditionTrue(status.Conditions, CertCondition)).To(BeTrue())
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := context.TODO()
			instance := &v1alpha1.CTlog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "instance",
					Namespace: "default",
				},
				Spec: v1alpha1.CTlogSpec{
					RootCertificates: tt.env.certificates,
				},
				Status: tt.env.status,
			}
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:   constants.Ready,
				Reason: constants.Creating,
			})

			c := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).
				WithObjects(tt.env.objects...).
				Build()

			configSecretWatch, err := c.Watch(ctx, &v1.SecretList{}, client.InNamespace("default"), client.MatchingLabels{labels.LabelResource: serverConfigResourceName})
			g.Expect(err).To(Not(HaveOccurred()))

			a := testAction.PrepareAction(c, NewHandleFulcioCertAction())

			if got := a.Handle(ctx, instance); !reflect.DeepEqual(got, tt.want.result) {
				t.Errorf("Handle() = %v, want %v", got, tt.want.result)
			}
			configSecretWatch.Stop()
			if tt.want.verify != nil {
				find := &v1alpha1.CTlog{}
				g.Expect(c.Get(ctx, client.ObjectKeyFromObject(instance), find)).To(Succeed())
				tt.want.verify(g, find.Status, c, configSecretWatch.ResultChan())
			}
		})
	}
}
