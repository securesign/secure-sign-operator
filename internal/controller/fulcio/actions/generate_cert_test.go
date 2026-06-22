package actions

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	_ "embed"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/fulcio/utils"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	"github.com/securesign/operator/internal/utils/kubernetes"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rhtasv1 "github.com/securesign/operator/api/v1"
)

func TestGenerateCert_Handle(t *testing.T) {
	ctx := context.TODO()
	g := NewWithT(t)
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	g.Expect(err).ToNot(HaveOccurred())
	pemKey, err := utils.CreateCAKey(key)
	g.Expect(err).ToNot(HaveOccurred())
	type env struct {
		certSpec       rhtasv1.FulcioCert
		generation     int64
		readyCondition string
		status         rhtasv1.FulcioStatus
		objects        []client.Object
	}
	type want struct {
		canHandle     bool
		result        *action.Result
		certCondition metav1.ConditionStatus
		verify        func(Gomega, rhtasv1.FulcioStatus, client.WithWatch)
	}
	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "generate new cert with default values",
			env: env{
				certSpec: rhtasv1.FulcioCert{
					OrganizationName:  "RH",
					OrganizationEmail: "jdoe@redhat.com",
				},
				status: rhtasv1.FulcioStatus{},
			},
			want: want{
				canHandle:     true,
				result:        testAction.Return(),
				certCondition: metav1.ConditionTrue,
				verify: func(g Gomega, fulcio rhtasv1.FulcioStatus, cli client.WithWatch) {
					g.Expect(fulcio.Certificate.CommonName).ToNot(BeEmpty())
					g.Expect(fulcio.Certificate.PrivateKeyPasswordRef).To(BeNil())
					g.Expect(fulcio.Certificate.PrivateKeyRef.Name).ToNot(BeEmpty())
					g.Expect(fulcio.Certificate.CARef.Name).ToNot(BeEmpty())

					scr, err := kubernetes.FindSecret(ctx, cli, "default", FulcioCALabel)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(scr.Name).To(Equal(fulcio.Certificate.CARef.Name))

					// Verify cert metadata is stored in secret annotations
					secret, err := kubernetes.GetSecret(cli, "default", scr.Name)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(secret.Annotations[labels.LabelNamespace+"/commonName"]).ToNot(BeEmpty())
					g.Expect(secret.Annotations[labels.LabelNamespace+"/organizationEmail"]).To(Equal("jdoe@redhat.com"))
					g.Expect(secret.Annotations[labels.LabelNamespace+"/organizationName"]).To(Equal("RH"))
				},
			},
		},
		{
			name: "generate new cert with missing private key",
			env: env{
				certSpec: rhtasv1.FulcioCert{
					OrganizationName:  "RH",
					OrganizationEmail: "jdoe@redhat.com",
					PrivateKeyRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{
							Name: "fulcio-private",
						},
						Key: "private",
					},
				},
				status: rhtasv1.FulcioStatus{},
			},
			want: want{
				canHandle:     true,
				result:        testAction.RequeueAfter(5 * time.Second),
				certCondition: metav1.ConditionFalse,
				verify: func(g Gomega, fulcio rhtasv1.FulcioStatus, cli client.WithWatch) {
					g.Expect(fulcio.Certificate.PrivateKeyRef.Name).ToNot(BeEmpty())
					g.Expect(fulcio.Certificate.CARef).To(BeNil())

					_, err := kubernetes.FindSecret(ctx, cli, "default", FulcioCALabel)
					g.Expect(errors.IsNotFound(err)).To(BeTrue())
				},
			},
		},
		{
			name: "generate new cert with provided private key",
			env: env{
				certSpec: rhtasv1.FulcioCert{
					OrganizationName:  "RH",
					OrganizationEmail: "jdoe@redhat.com",
					PrivateKeyRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{
							Name: "fulcio-private",
						},
						Key: "private",
					},
				},
				status: rhtasv1.FulcioStatus{},
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: "fulcio-private", Namespace: "default"},
						Data:       map[string][]byte{"private": pemKey},
					},
				},
			},
			want: want{
				canHandle:     true,
				result:        testAction.Return(),
				certCondition: metav1.ConditionTrue,
				verify: func(g Gomega, fulcio rhtasv1.FulcioStatus, cli client.WithWatch) {
					g.Expect(fulcio.Certificate.PrivateKeyRef.Name).To(Equal("fulcio-private"))
					g.Expect(fulcio.Certificate.CARef.Name).ToNot(BeEmpty())

					scr, err := kubernetes.FindSecret(ctx, cli, "default", FulcioCALabel)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(scr.Name).To(Equal(fulcio.Certificate.CARef.Name))
				},
			},
		},
		{
			name: "email update",
			env: env{
				certSpec: rhtasv1.FulcioCert{
					OrganizationName:  "RH",
					OrganizationEmail: "jdoe1@redhat.com",
				},
				status: rhtasv1.FulcioStatus{
					Certificate: &rhtasv1.FulcioCertStatus{
						CARef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{
								Name: "certificate-old",
							},
							Key: "cert",
						},
					}},
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: "certificate-old", Namespace: "default"},
						Data:       map[string][]byte{"private": pemKey},
					},
				},
			},
			want: want{
				canHandle:     true,
				result:        testAction.Return(),
				certCondition: metav1.ConditionTrue,
				verify: func(g Gomega, fulcio rhtasv1.FulcioStatus, cli client.WithWatch) {
					g.Expect(fulcio.Certificate.CARef.Name).ToNot(Equal("certificate-old"))

					scr, err := kubernetes.FindSecret(ctx, cli, "default", FulcioCALabel)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(scr.Name).To(Equal(fulcio.Certificate.CARef.Name))

					// old secret should not be removed
					g.Expect(cli.Get(ctx, client.ObjectKey{Name: "certificate-old", Namespace: "default"}, &v1.Secret{})).To(Succeed())
				},
			},
		},
		{
			name: "private key update",
			env: env{
				certSpec: rhtasv1.FulcioCert{
					OrganizationName:  "RH",
					OrganizationEmail: "jdoe@redhat.com",
					PrivateKeyRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{
							Name: "fulcio-private-new",
						},
						Key: "private",
					},
				},
				status: rhtasv1.FulcioStatus{
					Certificate: &rhtasv1.FulcioCertStatus{
						PrivateKeyRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{
								Name: "fulcio-private-old",
							},
							Key: "private",
						},
						CARef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{
								Name: "certificate-old",
							},
							Key: "cert",
						},
					},
				},
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: "certificate-old", Namespace: "default"},
						Data:       map[string][]byte{"private": pemKey},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: "fulcio-private-new", Namespace: "default"},
						Data:       map[string][]byte{"private": pemKey},
					},
				},
			},
			want: want{
				canHandle:     true,
				result:        testAction.Return(),
				certCondition: metav1.ConditionTrue,
				verify: func(g Gomega, fulcio rhtasv1.FulcioStatus, cli client.WithWatch) {
					g.Expect(fulcio.Certificate.PrivateKeyRef.Name).To(Equal("fulcio-private-new"))
					g.Expect(fulcio.Certificate.CARef.Name).ToNot(Equal("certificate-old"))

					scr, err := kubernetes.FindSecret(ctx, cli, "default", FulcioCALabel)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(scr.Name).To(Equal(fulcio.Certificate.CARef.Name))

					// old secret should not be removed
					g.Expect(cli.Get(ctx, client.ObjectKey{Name: "certificate-old", Namespace: "default"}, &v1.Secret{})).To(Succeed())
				},
			},
		},
		{
			name: "password update",
			env: env{
				certSpec: rhtasv1.FulcioCert{
					OrganizationName:  "RH",
					OrganizationEmail: "jdoe@redhat.com",
					PrivateKeyPasswordRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{
							Name: "fulcio-password-new",
						},
						Key: "password",
					},
				},
				status: rhtasv1.FulcioStatus{
					Certificate: &rhtasv1.FulcioCertStatus{
						PrivateKeyPasswordRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{
								Name: "fulcio-password-old",
							},
							Key: "private",
						},
						CARef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{
								Name: "certificate-old",
							},
							Key: "cert",
						},
					},
				},
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: "certificate-old", Namespace: "default"},
						Data:       map[string][]byte{"private": pemKey},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: "fulcio-password-new", Namespace: "default"},
						Data:       map[string][]byte{"password": pemKey},
					},
				},
			},
			want: want{
				canHandle:     true,
				result:        testAction.Return(),
				certCondition: metav1.ConditionTrue,
				verify: func(g Gomega, fulcio rhtasv1.FulcioStatus, cli client.WithWatch) {
					g.Expect(fulcio.Certificate.PrivateKeyPasswordRef.Name).To(Equal("fulcio-password-new"))
					g.Expect(fulcio.Certificate.CARef.Name).ToNot(Equal("certificate-old"))

					scr, err := kubernetes.FindSecret(ctx, cli, "default", FulcioCALabel)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(scr.Name).To(Equal(fulcio.Certificate.CARef.Name))

					// old secret should not be removed
					g.Expect(cli.Get(ctx, client.ObjectKey{Name: "certificate-old", Namespace: "default"}, &v1.Secret{})).To(Succeed())
				},
			},
		},
		{
			name: "reuse existing cert secret",
			env: env{
				certSpec: rhtasv1.FulcioCert{
					CommonName:        "fulcio.local",
					OrganizationName:  "RH",
					OrganizationEmail: "jdoe@redhat.com",
					PrivateKeyPasswordRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{
							Name: "fulcio-password",
						},
						Key: "password",
					},
				},
				status: rhtasv1.FulcioStatus{},
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: "fulcio-password", Namespace: "default"},
						Data:       map[string][]byte{"password": pemKey},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name: "fulcio-cert", Namespace: "default",
							Annotations: map[string]string{
								labels.LabelNamespace + "/commonName":        "fulcio.local",
								labels.LabelNamespace + "/organizationEmail": "jdoe@redhat.com",
								labels.LabelNamespace + "/organizationName":  "RH",
								labels.LabelNamespace + "/passwordKeyRef":    "fulcio-password",
							},
							Labels: map[string]string{FulcioCALabel: "cert"},
						},
						Data: map[string][]byte{"cert": []byte("fake")},
					},
				},
			},
			want: want{
				canHandle:     true,
				result:        testAction.Return(),
				certCondition: metav1.ConditionTrue,
				verify: func(g Gomega, fulcio rhtasv1.FulcioStatus, cli client.WithWatch) {
					g.Expect(fulcio.Certificate.CARef.Name).To(Equal("fulcio-cert"))
					g.Expect(fulcio.Certificate.CommonName).To(Equal("fulcio.local"))

					scr, err := kubernetes.FindSecret(ctx, cli, "default", FulcioCALabel)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(scr.Name).To(Equal(fulcio.Certificate.CARef.Name))
				},
			},
		},
		{
			name: "generation bump with matching secret keeps existing keys",
			env: env{
				certSpec: rhtasv1.FulcioCert{
					CommonName:        "fulcio.local",
					OrganizationName:  "RH",
					OrganizationEmail: "jdoe@redhat.com",
				},
				readyCondition: state.Ready.String(),
				generation:     2,
				status: rhtasv1.FulcioStatus{
					Certificate: &rhtasv1.FulcioCertStatus{
						PrivateKeyRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{Name: "fulcio-cert"},
							Key:                  "private",
						},
						CARef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{Name: "fulcio-cert"},
							Key:                  "cert",
						},
						CommonName: "fulcio.local",
					},
					Conditions: []metav1.Condition{
						{Type: CertCondition, Reason: "Resolved", Status: metav1.ConditionTrue, ObservedGeneration: 1},
					},
				},
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name: "fulcio-cert", Namespace: "default",
							Annotations: map[string]string{
								labels.LabelNamespace + "/commonName":        "fulcio.local",
								labels.LabelNamespace + "/organizationEmail": "jdoe@redhat.com",
								labels.LabelNamespace + "/organizationName":  "RH",
							},
							Labels: map[string]string{FulcioCALabel: "cert"},
						},
						Data: map[string][]byte{"private": pemKey, "cert": []byte("fake")},
					},
				},
			},
			want: want{
				canHandle:     true,
				result:        testAction.Return(),
				certCondition: metav1.ConditionTrue,
				verify: func(g Gomega, fulcio rhtasv1.FulcioStatus, cli client.WithWatch) {
					g.Expect(fulcio.Certificate.PrivateKeyRef.Name).To(Equal("fulcio-cert"))
					g.Expect(fulcio.Certificate.CARef.Name).To(Equal("fulcio-cert"))
					g.Expect(fulcio.Certificate.CommonName).To(Equal("fulcio.local"))

					rc := meta.FindStatusCondition(fulcio.Conditions, constants.ReadyCondition)
					g.Expect(rc).ToNot(BeNil())
					g.Expect(rc.Reason).To(Equal(state.Ready.String()))

					cc := meta.FindStatusCondition(fulcio.Conditions, CertCondition)
					g.Expect(cc).ToNot(BeNil())
					g.Expect(cc.ObservedGeneration).To(Equal(int64(2)))
				},
			},
		},
		{
			name: "spec change in Ready state transitions to Pending",
			env: env{
				certSpec: rhtasv1.FulcioCert{
					OrganizationName:  "RH",
					OrganizationEmail: "jdoe-new@redhat.com",
				},
				generation:     2,
				readyCondition: state.Ready.String(),
				status: rhtasv1.FulcioStatus{
					Certificate: &rhtasv1.FulcioCertStatus{
						CARef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{Name: "fulcio-cert-old"},
							Key:                  "cert",
						},
						CommonName: "fulcio.default.svc.local",
					},
					Conditions: []metav1.Condition{
						{Type: CertCondition, Reason: "Resolved", Status: metav1.ConditionTrue, ObservedGeneration: 1},
					},
				},
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name: "fulcio-cert-old", Namespace: "default",
							Annotations: map[string]string{
								labels.LabelNamespace + "/commonName":        "fulcio.default.svc.local",
								labels.LabelNamespace + "/organizationEmail": "jdoe@redhat.com",
								labels.LabelNamespace + "/organizationName":  "RH",
							},
							Labels: map[string]string{FulcioCALabel: "cert"},
						},
						Data: map[string][]byte{"cert": []byte("fake")},
					},
				},
			},
			want: want{
				canHandle:     true,
				result:        testAction.Return(),
				certCondition: metav1.ConditionFalse,
				verify: func(g Gomega, fulcio rhtasv1.FulcioStatus, cli client.WithWatch) {
					rc := meta.FindStatusCondition(fulcio.Conditions, constants.ReadyCondition)
					g.Expect(rc).ToNot(BeNil())
					g.Expect(rc.Reason).To(Equal(state.Pending.String()))

					cc := meta.FindStatusCondition(fulcio.Conditions, CertCondition)
					g.Expect(cc).ToNot(BeNil())
					g.Expect(cc.Reason).To(Equal(state.Creating.String()))
					g.Expect(cc.ObservedGeneration).To(Equal(int64(2)))
				},
			},
		},
		{
			name: "cert resolved - do not handle",
			env: env{
				certSpec: rhtasv1.FulcioCert{
					CommonName:        "fulcio.local",
					OrganizationName:  "RH",
					OrganizationEmail: "jdoe@redhat.com",
					PrivateKeyPasswordRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{
							Name: "fulcio-password",
						},
						Key: "password",
					},
				},
				generation: 3,
				status: rhtasv1.FulcioStatus{
					Certificate: &rhtasv1.FulcioCertStatus{
						PrivateKeyPasswordRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{
								Name: "fulcio-password",
							},
							Key: "password",
						},
						CARef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{
								Name: "fulcio-cert",
							},
							Key: "cert",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:               CertCondition,
							Reason:             "Resolved",
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 3,
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

			instance := &rhtasv1.Fulcio{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "instance",
					Namespace:  "default",
					Generation: tt.env.generation,
				},
				Spec: rhtasv1.FulcioSpec{
					Certificate: tt.env.certSpec,
				},
				Status: tt.env.status,
			}
			readyReason := state.Pending.String()
			if tt.env.readyCondition != "" {
				readyReason = tt.env.readyCondition
			}
			instance.SetCondition(metav1.Condition{
				Type:   constants.ReadyCondition,
				Reason: readyReason,
			})

			c := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).
				WithObjects(tt.env.objects...).
				Build()

			a := testAction.PrepareAction(c, NewHandleCertAction())
			g.Expect(tt.want.canHandle).To(Equal(a.CanHandle(ctx, instance)))

			if tt.want.canHandle {

				g.Expect(a.Handle(ctx, instance)).To(Equal(tt.want.result))
				g.Expect(meta.IsStatusConditionPresentAndEqual(instance.Status.Conditions, CertCondition, tt.want.certCondition)).To(BeTrue())

				found := &rhtasv1.Fulcio{}
				g.Expect(c.Get(ctx, client.ObjectKeyFromObject(instance), found)).To(Succeed())
				tt.want.verify(g, found.Status, c)
			}
		})
	}
}

func TestGenerateCert_CanHandle(t *testing.T) {
	tests := []struct {
		name       string
		generation int64
		status     rhtasv1.FulcioStatus
		canHandle  bool
	}{
		{
			name: "nil certificate",
			status: rhtasv1.FulcioStatus{
				Certificate: nil,
			},
			canHandle: true,
		},
		{
			name: "cert condition not true",
			status: rhtasv1.FulcioStatus{
				Certificate: &rhtasv1.FulcioCertStatus{},
				Conditions: []metav1.Condition{
					{Type: CertCondition, Reason: state.Creating.String(), Status: metav1.ConditionFalse, ObservedGeneration: 1},
				},
			},
			generation: 1,
			canHandle:  true,
		},
		{
			name: "generation mismatch",
			status: rhtasv1.FulcioStatus{
				Certificate: &rhtasv1.FulcioCertStatus{},
				Conditions: []metav1.Condition{
					{Type: CertCondition, Reason: "Resolved", Status: metav1.ConditionTrue, ObservedGeneration: 1},
				},
			},
			generation: 2,
			canHandle:  true,
		},
		{
			name: "no generation change",
			status: rhtasv1.FulcioStatus{
				Certificate: &rhtasv1.FulcioCertStatus{},
				Conditions: []metav1.Condition{
					{Type: CertCondition, Reason: "Resolved", Status: metav1.ConditionTrue, ObservedGeneration: 3},
				},
			},
			generation: 3,
			canHandle:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &rhtasv1.Fulcio{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "instance",
					Namespace:  "default",
					Generation: tt.generation,
				},
				Status: tt.status,
			}
			instance.SetCondition(metav1.Condition{
				Type:   constants.ReadyCondition,
				Reason: state.Ready.String(),
			})

			c := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).
				Build()

			a := testAction.PrepareAction(c, NewHandleCertAction())

			g := NewWithT(t)
			g.Expect(a.CanHandle(context.TODO(), instance)).To(Equal(tt.canHandle))
		})
	}
}
