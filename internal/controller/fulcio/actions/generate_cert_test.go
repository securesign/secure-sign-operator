package actions

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	_ "embed"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/fulcio/utils"
	"github.com/securesign/operator/internal/labels"
	testAction "github.com/securesign/operator/internal/testing/action"
	"github.com/securesign/operator/internal/utils/kubernetes"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func TestGenerateCert_Handle(t *testing.T) {
	g := NewWithT(t)
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	g.Expect(err).ToNot(HaveOccurred())
	pemKey, err := utils.CreateCAKey(key, nil)
	g.Expect(err).ToNot(HaveOccurred())
	type env struct {
		certSpec rhtasv1alpha1.FulcioCert
		status   rhtasv1alpha1.FulcioStatus
		objects  []client.Object
	}
	type want struct {
		canHandle     bool
		result        *action.Result
		certCondition metav1.ConditionStatus
		verify        func(context.Context, Gomega, rhtasv1alpha1.FulcioStatus, client.WithWatch)
	}
	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "generate new cert with default values",
			env: env{
				certSpec: rhtasv1alpha1.FulcioCert{
					OrganizationName:  "RH",
					OrganizationEmail: "jdoe@redhat.com",
				},
				status: rhtasv1alpha1.FulcioStatus{},
			},
			want: want{
				canHandle:     true,
				result:        testAction.StatusUpdate(),
				certCondition: metav1.ConditionTrue,
				verify: func(ctx context.Context, g Gomega, fulcio rhtasv1alpha1.FulcioStatus, cli client.WithWatch) {
					g.Expect(fulcio.Certificate.CommonName).ToNot(BeEmpty())
					g.Expect(fulcio.Certificate.OrganizationEmail).To(Equal("jdoe@redhat.com"))
					g.Expect(fulcio.Certificate.OrganizationName).To(Equal("RH"))
					g.Expect(fulcio.Certificate.PrivateKeyPasswordRef.Name).ToNot(BeEmpty())
					g.Expect(fulcio.Certificate.PrivateKeyRef.Name).ToNot(BeEmpty())
					g.Expect(fulcio.Certificate.CARef.Name).ToNot(BeEmpty())

					scr, err := kubernetes.FindSecret(ctx, cli, "default", FulcioCALabel)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(scr.Name).To(Equal(fulcio.Certificate.CARef.Name))
				},
			},
		},
		{
			name: "generate new cert with missing private key",
			env: env{
				certSpec: rhtasv1alpha1.FulcioCert{
					OrganizationName:  "RH",
					OrganizationEmail: "jdoe@redhat.com",
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{
						LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
							Name: "fulcio-private",
						},
						Key: "private",
					},
				},
				status: rhtasv1alpha1.FulcioStatus{},
			},
			want: want{
				canHandle:     true,
				result:        testAction.Requeue(),
				certCondition: metav1.ConditionFalse,
				verify: func(ctx context.Context, g Gomega, fulcio rhtasv1alpha1.FulcioStatus, cli client.WithWatch) {
					g.Expect(fulcio.Certificate.CommonName).ToNot(BeEmpty())
					g.Expect(fulcio.Certificate.OrganizationEmail).To(Equal("jdoe@redhat.com"))
					g.Expect(fulcio.Certificate.OrganizationName).To(Equal("RH"))
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
				certSpec: rhtasv1alpha1.FulcioCert{
					OrganizationName:  "RH",
					OrganizationEmail: "jdoe@redhat.com",
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{
						LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
							Name: "fulcio-private",
						},
						Key: "private",
					},
				},
				status: rhtasv1alpha1.FulcioStatus{},
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: "fulcio-private", Namespace: "default"},
						Data:       map[string][]byte{"private": pemKey},
					},
				},
			},
			want: want{
				canHandle:     true,
				result:        testAction.StatusUpdate(),
				certCondition: metav1.ConditionTrue,
				verify: func(ctx context.Context, g Gomega, fulcio rhtasv1alpha1.FulcioStatus, cli client.WithWatch) {
					g.Expect(fulcio.Certificate.CommonName).ToNot(BeEmpty())
					g.Expect(fulcio.Certificate.OrganizationEmail).To(Equal("jdoe@redhat.com"))
					g.Expect(fulcio.Certificate.OrganizationName).To(Equal("RH"))
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
				certSpec: rhtasv1alpha1.FulcioCert{
					OrganizationName:  "RH",
					OrganizationEmail: "jdoe1@redhat.com",
				},
				status: rhtasv1alpha1.FulcioStatus{
					Certificate: &rhtasv1alpha1.FulcioCert{
						OrganizationName:  "RH",
						OrganizationEmail: "jdoe@redhat.com",
						CARef: &rhtasv1alpha1.SecretKeySelector{
							LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
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
				result:        testAction.StatusUpdate(),
				certCondition: metav1.ConditionTrue,
				verify: func(ctx context.Context, g Gomega, fulcio rhtasv1alpha1.FulcioStatus, cli client.WithWatch) {
					g.Expect(fulcio.Certificate.CommonName).ToNot(BeEmpty())
					g.Expect(fulcio.Certificate.OrganizationEmail).To(Equal("jdoe1@redhat.com"))
					g.Expect(fulcio.Certificate.OrganizationName).To(Equal("RH"))
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
				certSpec: rhtasv1alpha1.FulcioCert{
					OrganizationName:  "RH",
					OrganizationEmail: "jdoe@redhat.com",
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{
						LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
							Name: "fulcio-private-new",
						},
						Key: "private",
					},
				},
				status: rhtasv1alpha1.FulcioStatus{
					Certificate: &rhtasv1alpha1.FulcioCert{
						OrganizationName:  "RH",
						OrganizationEmail: "jdoe@redhat.com",
						PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{
							LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
								Name: "fulcio-private-old",
							},
							Key: "private",
						},
						CARef: &rhtasv1alpha1.SecretKeySelector{
							LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
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
				result:        testAction.StatusUpdate(),
				certCondition: metav1.ConditionTrue,
				verify: func(ctx context.Context, g Gomega, fulcio rhtasv1alpha1.FulcioStatus, cli client.WithWatch) {
					g.Expect(fulcio.Certificate.CommonName).ToNot(BeEmpty())
					g.Expect(fulcio.Certificate.OrganizationEmail).To(Equal("jdoe@redhat.com"))
					g.Expect(fulcio.Certificate.OrganizationName).To(Equal("RH"))
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
				certSpec: rhtasv1alpha1.FulcioCert{
					OrganizationName:  "RH",
					OrganizationEmail: "jdoe@redhat.com",
					PrivateKeyPasswordRef: &rhtasv1alpha1.SecretKeySelector{
						LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
							Name: "fulcio-password-new",
						},
						Key: "password",
					},
				},
				status: rhtasv1alpha1.FulcioStatus{
					Certificate: &rhtasv1alpha1.FulcioCert{
						OrganizationName:  "RH",
						OrganizationEmail: "jdoe@redhat.com",
						PrivateKeyPasswordRef: &rhtasv1alpha1.SecretKeySelector{
							LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
								Name: "fulcio-password-old",
							},
							Key: "private",
						},
						CARef: &rhtasv1alpha1.SecretKeySelector{
							LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
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
				result:        testAction.StatusUpdate(),
				certCondition: metav1.ConditionTrue,
				verify: func(ctx context.Context, g Gomega, fulcio rhtasv1alpha1.FulcioStatus, cli client.WithWatch) {
					g.Expect(fulcio.Certificate.CommonName).ToNot(BeEmpty())
					g.Expect(fulcio.Certificate.OrganizationEmail).To(Equal("jdoe@redhat.com"))
					g.Expect(fulcio.Certificate.OrganizationName).To(Equal("RH"))
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
				certSpec: rhtasv1alpha1.FulcioCert{
					CommonName:        "fulcio.local",
					OrganizationName:  "RH",
					OrganizationEmail: "jdoe@redhat.com",
					PrivateKeyPasswordRef: &rhtasv1alpha1.SecretKeySelector{
						LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
							Name: "fulcio-password",
						},
						Key: "password",
					},
				},
				status: rhtasv1alpha1.FulcioStatus{},
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
				result:        testAction.StatusUpdate(),
				certCondition: metav1.ConditionTrue,
				verify: func(ctx context.Context, g Gomega, fulcio rhtasv1alpha1.FulcioStatus, cli client.WithWatch) {
					g.Expect(fulcio.Certificate.CommonName).To(Equal("fulcio.local"))
					g.Expect(fulcio.Certificate.CARef.Name).To(Equal("fulcio-cert"))

					scr, err := kubernetes.FindSecret(ctx, cli, "default", FulcioCALabel)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(scr.Name).To(Equal(fulcio.Certificate.CARef.Name))
				},
			},
		},
		{
			name: "cert resolved - do not handle",
			env: env{
				certSpec: rhtasv1alpha1.FulcioCert{
					CommonName:        "fulcio.local",
					OrganizationName:  "RH",
					OrganizationEmail: "jdoe@redhat.com",
					PrivateKeyPasswordRef: &rhtasv1alpha1.SecretKeySelector{
						LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
							Name: "fulcio-password",
						},
						Key: "password",
					},
				},
				status: rhtasv1alpha1.FulcioStatus{
					Certificate: &rhtasv1alpha1.FulcioCert{
						CommonName:        "fulcio.local",
						OrganizationName:  "RH",
						OrganizationEmail: "jdoe@redhat.com",
						PrivateKeyPasswordRef: &rhtasv1alpha1.SecretKeySelector{
							LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
								Name: "fulcio-password",
							},
							Key: "password",
						},
						CARef: &rhtasv1alpha1.SecretKeySelector{
							LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
								Name: "fulcio-cert",
							},
							Key: "cert",
						},
					},
					Conditions: []metav1.Condition{
						{
							Type:   CertCondition,
							Reason: "Resolved",
							Status: metav1.ConditionTrue,
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
			ctx := t.Context()
			instance := &rhtasv1alpha1.Fulcio{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "instance",
					Namespace: "default",
				},
				Spec: rhtasv1alpha1.FulcioSpec{
					Certificate: tt.env.certSpec,
				},
				Status: tt.env.status,
			}
			instance.SetCondition(metav1.Condition{
				Type:   constants.Ready,
				Reason: constants.Pending,
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
				g.Expect(err).ShouldNot(HaveOccurred())

				found := &rhtasv1alpha1.Fulcio{}
				g.Expect(c.Get(ctx, client.ObjectKeyFromObject(instance), found)).To(Succeed())
				tt.want.verify(ctx, g, found.Status, c)
			}
		})
	}
}
