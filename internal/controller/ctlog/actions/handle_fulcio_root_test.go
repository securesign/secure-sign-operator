package actions

import (
	"context"
	_ "embed"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//go:embed testdata/fulcio_root_cert.pem
var testCert string

func readyFulcio() *rhtasv1.Fulcio {
	return &rhtasv1.Fulcio{
		ObjectMeta: metav1.ObjectMeta{Name: "fulcio", Namespace: "default"},
		Spec: rhtasv1.FulcioSpec{
			Certificate: rhtasv1.FulcioCert{
				CommonName:        "test",
				OrganizationName:  "test",
				OrganizationEmail: "test@test.com",
			},
		},
		Status: rhtasv1.FulcioStatus{
			CertificateChain: testCert,
			Conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
			},
		},
	}
}

func TestCertCan_Handle(t *testing.T) {

	type env struct {
		phase        state.State
		certificates []rhtasv1.SecretKeySelector
		objects      []client.Object
		status       rhtasv1.CTlogStatus
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
				phase: state.Creating,
				certificates: []rhtasv1.SecretKeySelector{
					{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"},
						Key:                  "key",
					},
				},
				status: rhtasv1.CTlogStatus{
					RootCertificates: []rhtasv1.SecretKeySelector{
						{
							LocalObjectReference: rhtasv1.LocalObjectReference{Name: "fake"},
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
				phase: state.Creating,
				certificates: []rhtasv1.SecretKeySelector{
					{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"},
						Key:                  "key",
					},
				},
				status: rhtasv1.CTlogStatus{},
			},
			want: want{
				canHandle: true,
			},
		},
		{
			name: "autodiscovery new fulcio-cert",
			env: env{
				phase:        state.Creating,
				certificates: nil,
				status:       rhtasv1.CTlogStatus{},
				objects: []client.Object{
					readyFulcio(),
				},
			},
			want: want{
				canHandle: true,
			},
		},
		{
			name: "autodiscovery update fulcio-cert - ready phase",
			env: env{
				phase:        state.Ready,
				certificates: nil,
				status: rhtasv1.CTlogStatus{
					RootCertificates: []rhtasv1.SecretKeySelector{
						{
							LocalObjectReference: rhtasv1.LocalObjectReference{Name: "fake"},
							Key:                  "fake",
						},
					},
				},
				objects: []client.Object{
					readyFulcio(),
				},
			},
			want: want{
				canHandle: true,
			},
		},
		{
			name: "autodiscovery already resolved — still handles for cert rotation",
			env: env{
				phase:        state.Ready,
				certificates: nil,
				status: rhtasv1.CTlogStatus{
					RootCertificates: []rhtasv1.SecretKeySelector{
						{
							LocalObjectReference: rhtasv1.LocalObjectReference{Name: "ctlog-fulcio-root-instance"},
							Key:                  "cert",
						},
					},
				},
				objects: []client.Object{
					readyFulcio(),
				},
			},
			want: want{
				canHandle: true,
			},
		},
		{
			name: "pending phase",
			env: env{
				phase: state.Pending,
			},
			want: want{
				canHandle: false,
			},
		},
		{
			name: "matching cert-set",
			env: env{
				phase: state.Creating,
				certificates: []rhtasv1.SecretKeySelector{
					{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"},
						Key:                  "key",
					},
				},
				status: rhtasv1.CTlogStatus{
					RootCertificates: []rhtasv1.SecretKeySelector{
						{
							LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"},
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

			instance := rhtasv1.CTlog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "instance",
					Namespace: "default",
				},
				Spec: rhtasv1.CTlogSpec{
					RootCertificates: tt.env.certificates,
				},
				Status: tt.env.status,
			}
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:   constants.ReadyCondition,
				Reason: tt.env.phase.String(),
			})

			if got := a.CanHandle(context.TODO(), &instance); !reflect.DeepEqual(got, tt.want.canHandle) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want.canHandle)
			}
		})
	}
}
func TestCert_Handle(t *testing.T) {

	type env struct {
		certificates []rhtasv1.SecretKeySelector
		objects      []client.Object
		status       rhtasv1.CTlogStatus
	}
	type want struct {
		result *action.Result
		verify func(Gomega, rhtasv1.CTlogStatus, client.WithWatch, <-chan watch.Event)
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
				status: rhtasv1.CTlogStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Reason: state.Creating.String()},
					},
				},
				objects: []client.Object{
					readyFulcio(),
				},
			},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, status rhtasv1.CTlogStatus, cli client.WithWatch, configWatch <-chan watch.Event) {
					g.Expect(status.ServerConfigRef).Should(BeNil())

					g.Expect(status.RootCertificates).To(HaveLen(1))
					g.Expect(status.RootCertificates[0].Name).To(Equal(fmt.Sprintf(fulcioRootSecretFormat, "instance")))
					g.Expect(status.RootCertificates[0].Key).To(Equal(fulcioRootCertKey))

					secret := &v1.Secret{}
					g.Expect(cli.Get(context.TODO(), client.ObjectKey{Namespace: "default", Name: fmt.Sprintf(fulcioRootSecretFormat, "instance")}, secret)).To(Succeed())
					g.Expect(secret.Data).To(HaveKeyWithValue(fulcioRootCertKey, []byte(testCert)))

					g.Expect(meta.IsStatusConditionTrue(status.Conditions, CertCondition)).To(BeTrue())
				},
			},
		},
		{
			name: "autodiscover missing cert",
			env: env{
				certificates: nil,
				status: rhtasv1.CTlogStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Reason: state.Creating.String()},
					},
				},
			},
			want: want{
				result: testAction.RequeueAfter(5 * time.Second),
				verify: func(g Gomega, status rhtasv1.CTlogStatus, cli client.WithWatch, configWatch <-chan watch.Event) {
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
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "secret",
							Namespace: "default",
						},
						Data: map[string][]byte{"key": nil},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "secret-2",
							Namespace: "default",
						},
						Data: map[string][]byte{"key": nil},
					},
				},

				certificates: []rhtasv1.SecretKeySelector{
					{
						Key:                  "key",
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"},
					},
					{
						Key:                  "key",
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret-2"},
					},
				},
				status: rhtasv1.CTlogStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Reason: state.Creating.String()},
					},
				},
			},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, status rhtasv1.CTlogStatus, cli client.WithWatch, configWatch <-chan watch.Event) {
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
				certificates: []rhtasv1.SecretKeySelector{
					{
						Key:                  "key",
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "my-secret"},
					},
				},
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-secret",
							Namespace: "default",
						},
						Data: map[string][]byte{"key": nil},
					},
					readyFulcio(),
				},
				status: rhtasv1.CTlogStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Reason: state.Creating.String()},
					},
				},
			},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, status rhtasv1.CTlogStatus, cli client.WithWatch, configWatch <-chan watch.Event) {
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
				certificates: []rhtasv1.SecretKeySelector{
					{
						Key:                  "key",
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "my-secret"},
					},
				},
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "my-secret",
							Namespace: "default",
						},
						Data: map[string][]byte{"key": nil},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ctlog-config",
							Namespace: "default",
							Labels:    map[string]string{labels.LabelResource: serverConfigResourceName},
						},
						Data: map[string][]byte{},
					},
				},
				status: rhtasv1.CTlogStatus{
					ServerConfigRef: &rhtasv1.LocalObjectReference{Name: "ctlog-config"},
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Reason: state.Creating.String()},
					},
				},
			},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, status rhtasv1.CTlogStatus, cli client.WithWatch, configWatch <-chan watch.Event) {
					g.Expect(status.RootCertificates).Should(HaveLen(1))
					g.Expect(status.RootCertificates[0].Key).Should(Equal("key"))
					g.Expect(status.RootCertificates[0].Name).Should(Equal("my-secret"))

					g.Expect(meta.IsStatusConditionTrue(status.Conditions, CertCondition)).To(BeTrue())

					g.Expect(meta.IsStatusConditionFalse(status.Conditions, ConfigCondition)).Should(BeTrue())
				},
			},
		},
		{
			name: "autodiscovery cert rotation — content changed, updates secret and invalidates config",
			env: env{
				certificates: nil,
				objects: []client.Object{
					readyFulcio(),
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf(fulcioRootSecretFormat, "instance"),
							Namespace: "default",
						},
						Data: map[string][]byte{fulcioRootCertKey: []byte("-----BEGIN CERTIFICATE-----\nOLDCERT\n-----END CERTIFICATE-----\n")},
					},
				},
				status: rhtasv1.CTlogStatus{
					RootCertificates: []rhtasv1.SecretKeySelector{
						{
							LocalObjectReference: rhtasv1.LocalObjectReference{Name: fmt.Sprintf(fulcioRootSecretFormat, "instance")},
							Key:                  fulcioRootCertKey,
						},
					},
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Reason: state.Ready.String()},
					},
				},
			},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, status rhtasv1.CTlogStatus, cli client.WithWatch, configWatch <-chan watch.Event) {
					secret := &v1.Secret{}
					g.Expect(cli.Get(context.TODO(), client.ObjectKey{Namespace: "default", Name: fmt.Sprintf(fulcioRootSecretFormat, "instance")}, secret)).To(Succeed())
					g.Expect(secret.Data[fulcioRootCertKey]).To(Equal([]byte(testCert)))

					g.Expect(meta.IsStatusConditionTrue(status.Conditions, CertCondition)).To(BeTrue())
					g.Expect(meta.IsStatusConditionFalse(status.Conditions, ConfigCondition)).To(BeTrue())
				},
			},
		},
		{
			name: "autodiscovery no rotation — content unchanged, continues without config invalidation",
			env: env{
				certificates: nil,
				objects: []client.Object{
					readyFulcio(),
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf(fulcioRootSecretFormat, "instance"),
							Namespace: "default",
						},
						Data: map[string][]byte{fulcioRootCertKey: []byte(testCert)},
					},
				},
				status: rhtasv1.CTlogStatus{
					RootCertificates: []rhtasv1.SecretKeySelector{
						{
							LocalObjectReference: rhtasv1.LocalObjectReference{Name: fmt.Sprintf(fulcioRootSecretFormat, "instance")},
							Key:                  fulcioRootCertKey,
						},
					},
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Reason: state.Ready.String()},
					},
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(g Gomega, status rhtasv1.CTlogStatus, cli client.WithWatch, configWatch <-chan watch.Event) {
					g.Expect(meta.FindStatusCondition(status.Conditions, ConfigCondition)).To(BeNil())
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := context.TODO()
			instance := &rhtasv1.CTlog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "instance",
					Namespace: "default",
				},
				Spec: rhtasv1.CTlogSpec{
					RootCertificates: tt.env.certificates,
				},
				Status: tt.env.status,
			}
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:   constants.ReadyCondition,
				Reason: state.Creating.String(),
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
				find := &rhtasv1.CTlog{}
				g.Expect(c.Get(ctx, client.ObjectKeyFromObject(instance), find)).To(Succeed())
				tt.want.verify(g, find.Status, c, configSecretWatch.ResultChan())
			}
		})
	}
}
