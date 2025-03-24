package actions

import (
	"context"
	"reflect"
	"testing"

	"github.com/securesign/operator/internal/controller/common"
	"github.com/securesign/operator/internal/controller/ctlog/utils"
	"github.com/securesign/operator/internal/controller/labels"
	testAction "github.com/securesign/operator/internal/testing/action"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestKeysCan_Handle(t *testing.T) {

	type env struct {
		phase   string
		spec    v1alpha1.CTlogSpec
		objects []client.Object
		status  v1alpha1.CTlogStatus
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
			name: "generate new",
			env: env{
				phase:  constants.Creating,
				spec:   v1alpha1.CTlogSpec{},
				status: v1alpha1.CTlogStatus{},
			},
			want: want{
				canHandle: true,
			},
		},
		{
			name: "new spec key",
			env: env{
				phase: constants.Creating,
				spec: v1alpha1.CTlogSpec{
					PrivateKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "obj"},
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
			name: "spec change",
			env: env{
				phase: constants.Creating,
				spec: v1alpha1.CTlogSpec{
					PrivateKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "new"},
						Key:                  "key",
					},
				},
				status: v1alpha1.CTlogStatus{
					PrivateKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "old"},
						Key:                  "key",
					},
					PublicKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "old"},
						Key:                  "pub",
					},
					PrivateKeyPasswordRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "old"},
						Key:                  "password",
					},
				},
			},
			want: want{
				canHandle: true,
			},
		},
		{
			name: "generated keys-no change",
			env: env{
				phase: constants.Ready,
				spec:  v1alpha1.CTlogSpec{},
				status: v1alpha1.CTlogStatus{
					PrivateKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "old"},
						Key:                  "key",
					},
					PublicKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "old"},
						Key:                  "pub",
					},
					PrivateKeyPasswordRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "old"},
						Key:                  "password",
					},
				},
			},
			want: want{
				canHandle: false,
			},
		},
		{
			name: "spec keys-no change",
			env: env{
				phase: constants.Creating,
				spec: v1alpha1.CTlogSpec{
					PrivateKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "old"},
						Key:                  "key",
					},
					PublicKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "old"},
						Key:                  "pub",
					},
					PrivateKeyPasswordRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "old"},
						Key:                  "password",
					},
				},
				status: v1alpha1.CTlogStatus{
					PrivateKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "old"},
						Key:                  "key",
					},
					PublicKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "old"},
						Key:                  "pub",
					},
					PrivateKeyPasswordRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "old"},
						Key:                  "password",
					},
				},
			},
			want: want{
				canHandle: false,
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			c := testAction.FakeClientBuilder().
				WithObjects(tt.env.objects...).
				Build()
			a := testAction.PrepareAction(c, NewHandleKeysAction())

			instance := v1alpha1.CTlog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "instance",
					Namespace: "default",
				},
				Spec:   tt.env.spec,
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
func TestKeys_Handle(t *testing.T) {
	g := NewWithT(t)
	noPassKeyConf, err := utils.CreatePrivateKey(nil)
	g.Expect(err).To(Not(HaveOccurred()))

	encryptedKeyConf, err := utils.CreatePrivateKey(common.GeneratePassword(8))
	g.Expect(err).To(Not(HaveOccurred()))
	type env struct {
		spec    v1alpha1.CTlogSpec
		objects []client.Object
		status  v1alpha1.CTlogStatus
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
			name: "autodiscovery-find existing private key",
			env: env{
				spec:   v1alpha1.CTlogSpec{},
				status: v1alpha1.CTlogStatus{},
				objects: []client.Object{
					kubernetes.CreateSecret("secret", "default",
						map[string][]byte{"key": noPassKeyConf.PrivateKey}, map[string]string{CTLogPrivateLabel: "key"}),
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, status v1alpha1.CTlogStatus, cli client.WithWatch, configWatch <-chan watch.Event) {
					g.Expect(status.ServerConfigRef).Should(BeNil())

					g.Expect(status.PrivateKeyRef).To(Not(BeNil()))
					g.Expect(status.PrivateKeyRef.Name).To(Equal("secret"))

					// public key should be autogenerated
					g.Expect(status.PublicKeyRef).To(Not(BeNil()))

					// do not generate password for existing private key
					g.Expect(status.PrivateKeyPasswordRef).To(BeNil())
				},
			},
		},
		{
			name: "autodiscovery - select key private based on passwordRef, unlabel others",
			env: env{
				spec: v1alpha1.CTlogSpec{
					PrivateKeyPasswordRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "encrypted"},
						Key:                  "password",
					},
				},
				status: v1alpha1.CTlogStatus{},
				objects: []client.Object{
					// invalid private key
					kubernetes.CreateSecret("invalid", "default",
						map[string][]byte{
							"private": noPassKeyConf.PrivateKey,
							"public":  noPassKeyConf.PublicKey,
						}, map[string]string{
							CTLogPrivateLabel: "private",
							CTLPubLabel:       "public",
						}),

					// matching secret
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "encrypted",
							Namespace: "default",
							Labels: map[string]string{
								CTLogPrivateLabel: "private",
								CTLPubLabel:       "public",
							},
							Annotations: map[string]string{
								passwordKeyRefAnnotation: "encrypted",
								privateKeyRefAnnotation:  "encrypted",
							},
						},
						Data: map[string][]byte{"private": encryptedKeyConf.PrivateKey, "password": encryptedKeyConf.PrivateKeyPass, "public": encryptedKeyConf.PublicKey},
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, status v1alpha1.CTlogStatus, cli client.WithWatch, configWatch <-chan watch.Event) {
					g.Expect(status.ServerConfigRef).Should(BeNil())

					g.Expect(status.PrivateKeyRef).To(Not(BeNil()))
					g.Expect(status.PrivateKeyRef.Name).To(Equal("encrypted"))

					g.Expect(status.PublicKeyRef).To(Not(BeNil()))

					g.Expect(status.PrivateKeyPasswordRef).To(Not(BeNil()))
					g.Expect(status.PrivateKeyPasswordRef.Name).To(Equal("encrypted"))

					scr, err := kubernetes.GetSecret(cli, "default", "invalid")
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(scr.Labels).ToNot(HaveKey(CTLogPrivateLabel))
				},
			},
		},
		{
			name: "autodiscovery - select key private based on privateKey, unlabel others",
			env: env{
				spec: v1alpha1.CTlogSpec{
					PrivateKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "valid"},
						Key:                  "private",
					},
				},
				status: v1alpha1.CTlogStatus{},
				objects: []client.Object{
					// invalid private key
					kubernetes.CreateSecret("invalid", "default",
						map[string][]byte{
							"private": noPassKeyConf.PrivateKey,
							"public":  noPassKeyConf.PublicKey,
						}, map[string]string{
							CTLogPrivateLabel: "private",
							CTLPubLabel:       "public",
						}),

					// matching secret
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "valid",
							Namespace: "default",
							Labels: map[string]string{
								CTLogPrivateLabel: "private",
								CTLPubLabel:       "public",
							},
							Annotations: map[string]string{
								privateKeyRefAnnotation: "valid",
							},
						},
						Data: map[string][]byte{
							"private": noPassKeyConf.PrivateKey,
							"public":  noPassKeyConf.PublicKey},
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, status v1alpha1.CTlogStatus, cli client.WithWatch, configWatch <-chan watch.Event) {
					g.Expect(status.ServerConfigRef).Should(BeNil())

					g.Expect(status.PrivateKeyPasswordRef).To(BeNil())

					g.Expect(status.PrivateKeyRef).To(Not(BeNil()))
					g.Expect(status.PrivateKeyRef.Name).To(Equal("valid"))

					g.Expect(status.PublicKeyRef).To(Not(BeNil()))
					g.Expect(status.PublicKeyRef.Name).To(Equal("valid"))

					scr, err := kubernetes.GetSecret(cli, "default", "invalid")
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(scr.Labels).ToNot(HaveKey(CTLPubLabel))
				},
			},
		},
		{
			name: "spec keys",
			env: env{
				spec: v1alpha1.CTlogSpec{
					PrivateKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "secret"},
						Key:                  "private",
					},
					PrivateKeyPasswordRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "secret"},
						Key:                  "password",
					},
					PublicKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "secret"},
						Key:                  "public",
					},
				},
				status: v1alpha1.CTlogStatus{},
				objects: []client.Object{
					// invalid private key
					kubernetes.CreateSecret("invalid", "default",
						map[string][]byte{
							"private": noPassKeyConf.PrivateKey,
							"public":  noPassKeyConf.PublicKey,
						}, map[string]string{
							CTLogPrivateLabel: "private",
							CTLPubLabel:       "public",
						}),

					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "secret",
							Namespace: "default",
						},
						Data: map[string][]byte{
							"private":  encryptedKeyConf.PrivateKey,
							"public":   encryptedKeyConf.PublicKey,
							"password": encryptedKeyConf.PrivateKeyPass,
						},
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, status v1alpha1.CTlogStatus, cli client.WithWatch, configWatch <-chan watch.Event) {
					g.Expect(status.ServerConfigRef).Should(BeNil())

					g.Expect(status.PrivateKeyPasswordRef).To(Not(BeNil()))
					g.Expect(status.PrivateKeyPasswordRef.Name).To(Equal("secret"))

					g.Expect(status.PrivateKeyRef).To(Not(BeNil()))
					g.Expect(status.PrivateKeyRef.Name).To(Equal("secret"))

					g.Expect(status.PublicKeyRef).To(Not(BeNil()))
					g.Expect(status.PublicKeyRef.Name).To(Equal("secret"))
				},
			},
		},
		{
			name: "generate password-encrypted key",
			env: env{
				spec: v1alpha1.CTlogSpec{
					PrivateKeyPasswordRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "secret"},
						Key:                  "password",
					},
				},
				status: v1alpha1.CTlogStatus{},
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "secret",
							Namespace: "default",
						},
						Data: map[string][]byte{
							"password": encryptedKeyConf.PrivateKeyPass,
						},
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, status v1alpha1.CTlogStatus, cli client.WithWatch, configWatch <-chan watch.Event) {
					g.Expect(status.ServerConfigRef).Should(BeNil())

					g.Expect(status.PrivateKeyPasswordRef).To(Not(BeNil()))
					g.Expect(status.PrivateKeyPasswordRef.Name).To(Equal("secret"))

					g.Expect(status.PrivateKeyRef).To(Not(BeNil()))
					g.Expect(status.PrivateKeyRef.Name).To(Not(Equal("secret")))

					g.Expect(status.PublicKeyRef).To(Not(BeNil()))
					g.Expect(status.PublicKeyRef.Name).To(Not(Equal("secret")))
				},
			},
		},
		{
			name: "generate public key for spec private",
			env: env{
				spec: v1alpha1.CTlogSpec{
					PrivateKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "secret"},
						Key:                  "private",
					},
					PrivateKeyPasswordRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{Name: "secret"},
						Key:                  "password",
					},
				},
				status: v1alpha1.CTlogStatus{},
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "secret",
							Namespace: "default",
						},
						Data: map[string][]byte{
							"private":  encryptedKeyConf.PrivateKey,
							"password": encryptedKeyConf.PrivateKeyPass,
						},
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, status v1alpha1.CTlogStatus, cli client.WithWatch, configWatch <-chan watch.Event) {
					g.Expect(status.ServerConfigRef).Should(BeNil())

					g.Expect(status.PrivateKeyPasswordRef).To(Not(BeNil()))
					g.Expect(status.PrivateKeyRef.Name).To(Equal("secret"))

					g.Expect(status.PrivateKeyRef).To(Not(BeNil()))
					g.Expect(status.PrivateKeyRef.Name).To(Equal("secret"))

					g.Expect(status.PublicKeyRef).To(Not(BeNil()))
					g.Expect(status.PublicKeyRef.Name).To(Not(Equal("secret")))

					scr, err := kubernetes.GetSecret(cli, "default", status.PublicKeyRef.Name)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(scr.Labels).To(HaveKey(CTLPubLabel))
					g.Expect(scr.Data["public"]).To(Equal(encryptedKeyConf.PublicKey))
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
				Spec:   tt.env.spec,
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

			a := testAction.PrepareAction(c, NewHandleKeysAction())

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
