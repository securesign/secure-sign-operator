package actions

import (
	"context"
	_ "embed"
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"

	ctlogUtils "github.com/securesign/operator/internal/controller/ctlog/utils"
	"github.com/securesign/operator/internal/testing/errors"

	"github.com/onsi/gomega/gstruct"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/utils/ptr"

	. "github.com/onsi/gomega"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	testAction "github.com/securesign/operator/internal/testing/action"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	//go:embed testdata/private_key.pem
	privateKey []byte
	//go:embed testdata/public_key.pem
	publicKey []byte
	//go:embed testdata/cert.pem
	cert []byte
)

func TestServerConfig_CanHandle(t *testing.T) {
	tests := []struct {
		name                  string
		status                metav1.ConditionStatus
		canHandle             bool
		serverConfigRef       *rhtasv1alpha1.LocalObjectReference
		statusServerConfigRef *rhtasv1alpha1.LocalObjectReference
		observedGeneration    int64
		generation            int64
	}{
		{
			name:                  "ConditionTrue: spec.serverConfigRef is not nil and status.serverConfigRef is nil",
			status:                metav1.ConditionTrue,
			canHandle:             true,
			serverConfigRef:       &rhtasv1alpha1.LocalObjectReference{Name: "config"},
			statusServerConfigRef: nil,
		},
		{
			name:                  "ConditionTrue: spec.serverConfigRef is nil and status.serverConfigRef is not nil",
			status:                metav1.ConditionTrue,
			canHandle:             false,
			serverConfigRef:       nil,
			statusServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "config"},
		},
		{
			name:                  "ConditionTrue: spec.serverConfigRef is nil and status.serverConfigRef is nil",
			status:                metav1.ConditionTrue,
			canHandle:             true,
			serverConfigRef:       nil,
			statusServerConfigRef: nil,
		},
		{
			name:                  "ConditionTrue: spec.serverConfigRef != status.serverConfigRef",
			status:                metav1.ConditionTrue,
			canHandle:             true,
			serverConfigRef:       &rhtasv1alpha1.LocalObjectReference{Name: "new_config"},
			statusServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "old_config"},
		},
		{
			name:                  "ConditionTrue: spec.serverConfigRef == status.serverConfigRef",
			status:                metav1.ConditionTrue,
			canHandle:             false,
			serverConfigRef:       &rhtasv1alpha1.LocalObjectReference{Name: "config"},
			statusServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "config"},
		},
		{
			name:                  "ConditionTrue: observedGeneration == generation",
			status:                metav1.ConditionTrue,
			canHandle:             false,
			statusServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "config"},
			observedGeneration:    1,
			generation:            1,
		},
		{
			name:                  "ConditionTrue: observedGeneration != generation",
			status:                metav1.ConditionTrue,
			canHandle:             true,
			statusServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "config"},
			observedGeneration:    1,
			generation:            2,
		},
		{
			name:                  "empty condition",
			status:                "",
			canHandle:             false,
			statusServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "config"},
			observedGeneration:    1,
			generation:            1,
		},
		{
			name:                  "ConditionUnknown",
			status:                metav1.ConditionUnknown,
			canHandle:             true,
			statusServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "config"},
			observedGeneration:    1,
			generation:            1,
		},
		{
			name:                  "ConditionFalse",
			status:                metav1.ConditionFalse,
			canHandle:             true,
			statusServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "config"},
			observedGeneration:    1,
			generation:            1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testAction.FakeClientBuilder().Build()
			a := testAction.PrepareAction(c, NewServerConfigAction())
			instance := rhtasv1alpha1.CTlog{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "default",
					Generation: tt.generation,
				},
				Spec: rhtasv1alpha1.CTlogSpec{
					ServerConfigRef: tt.serverConfigRef,
				},
				Status: rhtasv1alpha1.CTlogStatus{
					ServerConfigRef: tt.statusServerConfigRef,
				},
			}
			if tt.status != "" {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:               ConfigCondition,
					Status:             tt.status,
					ObservedGeneration: tt.observedGeneration,
				})
			}

			if got := a.CanHandle(context.TODO(), &instance); !reflect.DeepEqual(got, tt.canHandle) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.canHandle)
			}
		})
	}
}

func TestServerConfig_Handle(t *testing.T) {
	g := NewWithT(t)
	labels := constants.LabelsFor(ComponentName, DeploymentName, "ctlog")
	labels[constants.LabelResource] = serverConfigResourceName

	type env struct {
		spec    rhtasv1alpha1.CTlogSpec
		status  rhtasv1alpha1.CTlogStatus
		objects []client.Object
	}
	type want struct {
		result *action.Result
		verify func(Gomega, *rhtasv1alpha1.CTlog, client.WithWatch)
	}
	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "use spec.serverConfigRef",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					ServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "config"},
				},
				status: rhtasv1alpha1.CTlogStatus{
					ServerConfigRef: nil,
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, instance *rhtasv1alpha1.CTlog, cli client.WithWatch) {
					g.Expect(instance.Status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(instance.Status.ServerConfigRef.Name).Should(Equal("config"))
				},
			},
		},
		{
			name: "create a new config",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					ServerConfigRef: nil,
					Trillian:        rhtasv1alpha1.TrillianService{Port: ptr.To(int32(80))},
				},
				status: rhtasv1alpha1.CTlogStatus{
					ServerConfigRef: nil,
					TreeID:          ptr.To(int64(123456)),
					RootCertificates: []rhtasv1alpha1.SecretKeySelector{
						{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "cert"},
					},
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
					PublicKeyRef:  &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "public"},
				},
				objects: []client.Object{
					kubernetes.CreateSecret("secret", "default", map[string][]byte{
						"cert":    cert,
						"private": privateKey,
						"public":  publicKey,
					}, map[string]string{}),
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, instance *rhtasv1alpha1.CTlog, cli client.WithWatch) {
					g.Expect(instance.Status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(instance.Status.ServerConfigRef.Name).Should(ContainSubstring("ctlog-config-"))
				},
			},
		},
		{
			name: "replace config from spec",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					ServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "new_config"},
				},
				status: rhtasv1alpha1.CTlogStatus{
					ServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "old_config"},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, instance *rhtasv1alpha1.CTlog, cli client.WithWatch) {
					g.Expect(instance.Status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(instance.Status.ServerConfigRef.Name).Should(Equal("new_config"))
				},
			},
		},
		{
			name: "Waiting for Fulcio root certificate",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					ServerConfigRef: nil,
					Trillian:        rhtasv1alpha1.TrillianService{Port: ptr.To(int32(80))},
				},
				status: rhtasv1alpha1.CTlogStatus{
					ServerConfigRef: nil,
					TreeID:          ptr.To(int64(123456)),
					RootCertificates: []rhtasv1alpha1.SecretKeySelector{
						{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "not-existing"}, Key: "cert"},
					},
					PrivateKeyRef:         &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
					PublicKeyRef:          &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "public"},
					PrivateKeyPasswordRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "password"},
				},
				objects: []client.Object{
					kubernetes.CreateSecret("secret", "default", map[string][]byte{
						"cert":    cert,
						"private": privateKey,
						"public":  publicKey,
					}, map[string]string{}),
				},
			},
			want: want{
				result: testAction.Requeue(),
				verify: func(g Gomega, instance *rhtasv1alpha1.CTlog, cli client.WithWatch) {
					g.Expect(instance.Status.ServerConfigRef).Should(BeNil())
					g.Expect(instance.Status.Conditions).To(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Message": ContainSubstring("Waiting for Fulcio root certificate: not-existing/cert"),
						"Status":  Equal(metav1.ConditionFalse),
						"Reason":  Equal(FulcioReason),
					})))
				},
			},
		},
		{
			name: "Waiting for Ctlog private key secret",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					ServerConfigRef: nil,
					Trillian:        rhtasv1alpha1.TrillianService{Port: ptr.To(int32(80))},
				},
				status: rhtasv1alpha1.CTlogStatus{
					ServerConfigRef: nil,
					TreeID:          ptr.To(int64(123456)),
					RootCertificates: []rhtasv1alpha1.SecretKeySelector{
						{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "cert"},
					},
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "not-existing"}, Key: "private"},
					PublicKeyRef:  &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "public"},
				},
				objects: []client.Object{
					kubernetes.CreateSecret("secret", "default", map[string][]byte{
						"cert":    cert,
						"private": privateKey,
						"public":  publicKey,
					}, map[string]string{}),
				},
			},
			want: want{
				result: testAction.Requeue(),
				verify: func(g Gomega, instance *rhtasv1alpha1.CTlog, cli client.WithWatch) {
					g.Expect(instance.Status.ServerConfigRef).Should(BeNil())
					g.Expect(instance.Status.Conditions).To(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Message": ContainSubstring("Waiting for Ctlog private key secret"),
						"Status":  Equal(metav1.ConditionFalse),
						"Reason":  Equal(SignerKeyReason),
					})))
				},
			},
		},
		{
			name: "Delete existing Ctlog configuration",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					ServerConfigRef: nil,
					Trillian:        rhtasv1alpha1.TrillianService{Port: ptr.To(int32(80))},
				},
				status: rhtasv1alpha1.CTlogStatus{
					ServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "config"},
					TreeID:          ptr.To(int64(123456)),
					RootCertificates: []rhtasv1alpha1.SecretKeySelector{
						{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "cert"},
					},
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
					PublicKeyRef:  &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "public"},
				},
				objects: []client.Object{
					kubernetes.CreateSecret("secret", "default", map[string][]byte{
						"cert":    cert,
						"private": privateKey,
						"public":  publicKey,
					}, map[string]string{}),

					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "config",
							Namespace: "default",
							Labels:    labels,
						},
						Data: map[string][]byte{},
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, instance *rhtasv1alpha1.CTlog, cli client.WithWatch) {
					g.Expect(instance.Status.ServerConfigRef).Should(Not(BeNil()))

					g.Expect(k8sErrors.IsNotFound(cli.Get(context.TODO(), client.ObjectKey{Name: "config", Namespace: "default"}, &v1.Secret{}))).To(BeTrue())

					secret, err := kubernetes.GetSecret(cli, "default", instance.Status.ServerConfigRef.Name)
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(secret.Data).To(HaveKey("config"))
				},
			},
		},
		{
			name: "Update config on cert change",
			env: env{
				spec: rhtasv1alpha1.CTlogSpec{
					ServerConfigRef: nil,
					Trillian:        rhtasv1alpha1.TrillianService{Port: ptr.To(int32(80))},
				},
				status: rhtasv1alpha1.CTlogStatus{
					ServerConfigRef: nil,
					TreeID:          ptr.To(int64(123456)),
					RootCertificates: []rhtasv1alpha1.SecretKeySelector{
						{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "cert"},
						{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "new"}, Key: "cert"},
					},
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
					PublicKeyRef:  &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "public"},
				},
				objects: []client.Object{
					kubernetes.CreateSecret("secret", "default", map[string][]byte{
						"cert":    cert,
						"private": privateKey,
						"public":  publicKey,
					}, map[string]string{}),
					kubernetes.CreateSecret("new", "default", map[string][]byte{
						"cert": cert,
					}, map[string]string{}),

					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "config",
							Namespace: "default",
							Labels:    labels,
						},
						Data: map[string][]byte{},
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, instance *rhtasv1alpha1.CTlog, cli client.WithWatch) {
					g.Expect(instance.Status.ServerConfigRef).Should(Not(BeNil()))
					g.Expect(instance.Status.ServerConfigRef.Name).Should(Not(Equal("config")))

					_, err := kubernetes.GetSecret(cli, "default", "config")
					g.Expect(k8sErrors.IsNotFound(err)).To(BeTrue())

					secret, err := kubernetes.GetSecret(cli, "default", instance.Status.ServerConfigRef.Name)
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(secret.Data).To(HaveKey("config"))
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()
			instance := &rhtasv1alpha1.CTlog{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "ctlog",
					Namespace:  "default",
					Generation: int64(1),
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

			a := testAction.PrepareAction(c, NewServerConfigAction())

			if got := a.Handle(ctx, instance); !reflect.DeepEqual(got, tt.want.result) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want.result)
			}
			if tt.want.verify != nil {
				tt.want.verify(g, instance, c)
			}
		})
	}
}

func TestServerConfig_Update(t *testing.T) {
	g := NewWithT(t)
	type env struct {
		instance rhtasv1alpha1.CTlog
		objects  []client.Object
	}
	type want struct {
		result *action.Result
		verify func(Gomega, client.Client, *rhtasv1alpha1.CTlog)
	}
	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "new secret with config created",
			env: env{
				instance: rhtasv1alpha1.CTlog{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test",
						Namespace:  "default",
						Generation: 2,
					},
					Spec: rhtasv1alpha1.CTlogSpec{
						Trillian: rhtasv1alpha1.TrillianService{Port: ptr.To(int32(443))},
						TreeID:   ptr.To(int64(123456)),
					},
					Status: rhtasv1alpha1.CTlogStatus{
						ServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "old_secret"},
						TreeID:          ptr.To(int64(123456)),
						RootCertificates: []rhtasv1alpha1.SecretKeySelector{
							{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "cert"},
						},
						PrivateKeyRef:         &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
						PrivateKeyPasswordRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "password"},
						PublicKeyRef:          &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "public"},
						Conditions: []metav1.Condition{
							{
								Type:               constants.Ready,
								Reason:             constants.Ready,
								ObservedGeneration: 1,
							},
						},
					},
				},
				objects: []client.Object{
					kubernetes.CreateSecret("secret", "default", map[string][]byte{
						"cert":     cert,
						"private":  privateKey,
						"public":   publicKey,
						"password": []byte("secure"),
					}, map[string]string{}),
					kubernetes.CreateSecret("old_secret", "default",
						errors.IgnoreError(ctlogUtils.CreateCtlogConfig(
							"trillian-logserver.default.svc:80",
							654321,
							[]ctlogUtils.RootCertificate{cert},
							&ctlogUtils.KeyConfig{
								PrivateKey:     privateKey,
								PublicKey:      publicKey,
								PrivateKeyPass: []byte("secure"),
							})),
						map[string]string{}),
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, cli client.Client, current *rhtasv1alpha1.CTlog) {
					g.Expect(current.Status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(current.Status.ServerConfigRef.Name).Should(ContainSubstring("ctlog-config-"))

					data, err := kubernetes.GetSecretData(cli, "default", &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: *current.Status.ServerConfigRef, Key: "config"})
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(data).To(And(ContainSubstring("trillian-logserver.default.svc:443"), ContainSubstring("123456")))

					_, err = kubernetes.GetSecret(cli, "default", "old_config")
					g.Expect(err).To(WithTransform(k8sErrors.IsNotFound, BeTrue()), "old_config should be deleted")
				},
			},
		},
		{
			name: "use custom config and ignore other changes",
			env: env{
				instance: rhtasv1alpha1.CTlog{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test",
						Namespace:  "default",
						Generation: 2,
					},
					Spec: rhtasv1alpha1.CTlogSpec{
						ServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "custom_config"},
					},
					Status: rhtasv1alpha1.CTlogStatus{
						ServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "old_secret"},
						TreeID:          ptr.To(int64(123456)),
						RootCertificates: []rhtasv1alpha1.SecretKeySelector{
							{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "cert"},
						},
						PrivateKeyRef:         &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
						PrivateKeyPasswordRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "password"},
						PublicKeyRef:          &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "public"},
						Conditions: []metav1.Condition{
							{
								Type:               constants.Ready,
								Reason:             constants.Ready,
								ObservedGeneration: 1,
							},
						},
					},
				},
				objects: []client.Object{
					kubernetes.CreateSecret("custom_config", "default",
						errors.IgnoreError(ctlogUtils.CreateCtlogConfig(
							"trillian-logserver.custom.svc:80",
							9999999,
							[]ctlogUtils.RootCertificate{cert},
							&ctlogUtils.KeyConfig{
								PrivateKey:     privateKey,
								PublicKey:      publicKey,
								PrivateKeyPass: []byte("secure"),
							})),
						map[string]string{}),
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, cli client.Client, current *rhtasv1alpha1.CTlog) {
					g.Expect(current.Status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(current.Status.ServerConfigRef.Name).Should(Equal("custom_config"))

					data, err := kubernetes.GetSecretData(cli, "default", &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: *current.Status.ServerConfigRef, Key: "config"})
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(data).To(And(ContainSubstring("trillian-logserver.custom.svc:80"), ContainSubstring("9999999")))
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()
			c := testAction.FakeClientBuilder().
				WithObjects(&tt.env.instance).
				WithStatusSubresource(&tt.env.instance).
				WithObjects(tt.env.objects...).
				Build()

			a := testAction.PrepareAction(c, NewServerConfigAction())

			if got := a.Handle(ctx, &tt.env.instance); !reflect.DeepEqual(got, tt.want.result) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want.result)
			}
			if tt.want.verify != nil {
				tt.want.verify(g, c, &tt.env.instance)
			}
		})
	}
}
