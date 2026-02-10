package actions

import (
	"context"
	_ "embed"
	"reflect"
	"testing"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	v1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"

	ctlogUtils "github.com/securesign/operator/internal/controller/ctlog/utils"
	"github.com/securesign/operator/internal/testing/errors"

	"github.com/onsi/gomega/gstruct"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/utils/ptr"

	. "github.com/onsi/gomega"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
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
			canHandle:             true,
			serverConfigRef:       nil,
			statusServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "config"},
			observedGeneration:    1,
			generation:            1,
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
			canHandle:             true, // Always true for periodic validation
			serverConfigRef:       &rhtasv1alpha1.LocalObjectReference{Name: "config"},
			statusServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "config"},
		},
		{
			name:                  "ConditionTrue: observedGeneration == generation",
			status:                metav1.ConditionTrue,
			canHandle:             true,
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
	labels := labels.ForResource(ComponentName, DeploymentName, "ctlog", serverConfigResourceName)

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
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "config",
							Namespace: "default",
						},
						Data: map[string][]byte{
							ctlogUtils.ConfigKey: []byte("test-config"),
						},
					},
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
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "secret",
							Namespace: "default",
						},
						Data: map[string][]byte{
							"cert":    cert,
							"private": privateKey,
							"public":  publicKey,
						},
					},
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
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "new_config",
							Namespace: "default",
						},
						Data: map[string][]byte{
							ctlogUtils.ConfigKey: []byte("new-test-config"),
						},
					},
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
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "secret",
							Namespace: "default",
						},
						Data: map[string][]byte{
							"cert":    cert,
							"private": privateKey,
							"public":  publicKey,
						},
					},
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
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "secret",
							Namespace: "default",
						},
						Data: map[string][]byte{
							"cert":    cert,
							"private": privateKey,
							"public":  publicKey,
						},
					},
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
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "secret",
							Namespace: "default",
						},
						Data: map[string][]byte{
							"cert":    cert,
							"private": privateKey,
							"public":  publicKey,
						},
					},

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
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "secret",
							Namespace: "default",
						},
						Data: map[string][]byte{
							"cert":    cert,
							"private": privateKey,
							"public":  publicKey,
						},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "new",
							Namespace: "default",
						},
						Data: map[string][]byte{
							"cert": cert,
						},
					},

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
				Type:   constants.ReadyCondition,
				Reason: state.Creating.String(),
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

	// -- local helpers scoped to this test function --

	newBaseInstance := func() rhtasv1alpha1.CTlog {
		return rhtasv1alpha1.CTlog{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test",
				Namespace:  "default",
				Generation: 1,
			},
			Spec: rhtasv1alpha1.CTlogSpec{
				Trillian: rhtasv1alpha1.TrillianService{Port: ptr.To(int32(80))},
			},
			Status: rhtasv1alpha1.CTlogStatus{
				ServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "existing-config"},
				TreeID:          ptr.To(int64(123456)),
				RootCertificates: []rhtasv1alpha1.SecretKeySelector{
					{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "cert"},
				},
				PrivateKeyRef:         &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
				PrivateKeyPasswordRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "password"},
				PublicKeyRef:          &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "public"},
				Conditions: []metav1.Condition{
					{
						Type:               constants.ReadyCondition,
						Reason:             state.Ready.String(),
						ObservedGeneration: 1,
					},
					{
						Type:               ConfigCondition,
						Status:             metav1.ConditionTrue,
						Reason:             state.Ready.String(),
						Message:            "Server config created",
						ObservedGeneration: 1,
					},
				},
			},
		}
	}

	newKeySecret := func(namespace string) *v1.Secret {
		return &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "secret", Namespace: namespace},
			Data: map[string][]byte{
				"cert": cert, "private": privateKey, "public": publicKey, "password": []byte("secure"),
			},
		}
	}

	defaultAnnotations := func() map[string]string {
		return map[string]string{
			"rhtas.redhat.com/treeID":           "123456",
			"rhtas.redhat.com/trillianUrl":      "trillian-logserver.default.svc:80",
			"rhtas.redhat.com/rootCertificates": "secret/cert",
			"rhtas.redhat.com/privateKeyRef":    "secret/private",
		}
	}

	newConfigSecret := func(name, namespace string, annotations map[string]string) *v1.Secret {
		return &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Annotations: annotations},
			Data: errors.IgnoreError(ctlogUtils.CreateCtlogConfig(
				"trillian-logserver.default.svc:80", 123456,
				[]ctlogUtils.RootCertificate{cert},
				&ctlogUtils.KeyConfig{PrivateKey: privateKey, PublicKey: publicKey, PrivateKeyPass: []byte("secure")},
			)),
		}
	}

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
			env: func() env {
				inst := newBaseInstance()
				inst.Generation = 2
				inst.Spec.Trillian.Port = ptr.To(int32(443))
				inst.Spec.TreeID = ptr.To(int64(123456))
				inst.Status.ServerConfigRef = &rhtasv1alpha1.LocalObjectReference{Name: "old_secret"}
				// Only ReadyCondition, no ConfigCondition
				inst.Status.Conditions = []metav1.Condition{{
					Type:               constants.ReadyCondition,
					Reason:             state.Ready.String(),
					ObservedGeneration: 1,
				}}
				return env{
					instance: inst,
					objects: []client.Object{
						newKeySecret("default"),
						// Old secret without annotations - forces recreation
						&v1.Secret{
							ObjectMeta: metav1.ObjectMeta{Name: "old_secret", Namespace: "default"},
							Data: errors.IgnoreError(ctlogUtils.CreateCtlogConfig(
								"trillian-logserver.default.svc:80", 654321,
								[]ctlogUtils.RootCertificate{cert},
								&ctlogUtils.KeyConfig{PrivateKey: privateKey, PublicKey: publicKey, PrivateKeyPass: []byte("secure")},
							)),
						},
					},
				}
			}(),
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
			name: "replica-only change should not recreate config",
			env: func() env {
				inst := newBaseInstance()
				inst.Generation = 2 // Generation bumped (e.g. replicas changed), but config inputs unchanged
				return env{
					instance: inst,
					objects: []client.Object{
						newKeySecret("default"),
						newConfigSecret("existing-config", "default", defaultAnnotations()),
					},
				}
			}(),
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, cli client.Client, current *rhtasv1alpha1.CTlog) {
					g.Expect(current.Status.ServerConfigRef.Name).Should(Equal("existing-config"))

					c := meta.FindStatusCondition(current.Status.Conditions, ConfigCondition)
					g.Expect(c).ShouldNot(BeNil())
					g.Expect(c.ObservedGeneration).Should(Equal(int64(2)))
					g.Expect(c.Status).Should(Equal(metav1.ConditionTrue))
				},
			},
		},
		{
			name: "steady state - valid config and no generation change returns Continue",
			env: func() env {
				inst := newBaseInstance() // Generation=1, observedGeneration=1 -> no change
				return env{
					instance: inst,
					objects: []client.Object{
						newKeySecret("default"),
						newConfigSecret("existing-config", "default", defaultAnnotations()),
					},
				}
			}(),
			want: want{
				result: testAction.Continue(),
				verify: func(g Gomega, cli client.Client, current *rhtasv1alpha1.CTlog) {
					g.Expect(current.Status.ServerConfigRef.Name).Should(Equal("existing-config"))

					c := meta.FindStatusCondition(current.Status.Conditions, ConfigCondition)
					g.Expect(c).ShouldNot(BeNil())
					g.Expect(c.ObservedGeneration).Should(Equal(int64(1)))
					g.Expect(c.Status).Should(Equal(metav1.ConditionTrue))
				},
			},
		},
		{
			name: "secret deleted externally should trigger recreation",
			env: func() env {
				inst := newBaseInstance()
				inst.Status.ServerConfigRef = &rhtasv1alpha1.LocalObjectReference{Name: "deleted-config"}
				return env{
					instance: inst,
					// Note: "deleted-config" secret is intentionally NOT created
					objects: []client.Object{newKeySecret("default")},
				}
			}(),
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, cli client.Client, current *rhtasv1alpha1.CTlog) {
					g.Expect(current.Status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(current.Status.ServerConfigRef.Name).Should(ContainSubstring("ctlog-config-"))
					g.Expect(current.Status.ServerConfigRef.Name).ShouldNot(Equal("deleted-config"))

					secret, err := kubernetes.GetSecret(cli, "default", current.Status.ServerConfigRef.Name)
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(secret.Data).To(HaveKey("config"))
				},
			},
		},
		{
			name: "treeID change detected via annotations triggers recreation",
			env: func() env {
				inst := newBaseInstance()
				inst.Generation = 2
				inst.Status.ServerConfigRef = &rhtasv1alpha1.LocalObjectReference{Name: "old-config"}
				inst.Status.TreeID = ptr.To(int64(999999)) // Changed from 123456
				return env{
					instance: inst,
					objects: []client.Object{
						newKeySecret("default"),
						// Config secret still has OLD treeID in annotations
						newConfigSecret("old-config", "default", defaultAnnotations()),
					},
				}
			}(),
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, cli client.Client, current *rhtasv1alpha1.CTlog) {
					g.Expect(current.Status.ServerConfigRef.Name).Should(ContainSubstring("ctlog-config-"))
					g.Expect(current.Status.ServerConfigRef.Name).ShouldNot(Equal("old-config"))

					data, err := kubernetes.GetSecretData(cli, "default", &rhtasv1alpha1.SecretKeySelector{
						LocalObjectReference: *current.Status.ServerConfigRef, Key: "config",
					})
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(data).To(ContainSubstring("999999"))

					c := meta.FindStatusCondition(current.Status.Conditions, ConfigCondition)
					g.Expect(c).ShouldNot(BeNil())
					g.Expect(c.Status).Should(Equal(metav1.ConditionTrue))
					g.Expect(c.ObservedGeneration).Should(Equal(int64(2)))
				},
			},
		},
		{
			name: "root certificate change detected via annotations triggers recreation",
			env: func() env {
				inst := newBaseInstance()
				inst.Generation = 2
				inst.Status.ServerConfigRef = &rhtasv1alpha1.LocalObjectReference{Name: "old-config"}
				// Add a second root certificate
				inst.Status.RootCertificates = append(inst.Status.RootCertificates,
					rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "new-fulcio"}, Key: "cert"},
				)
				return env{
					instance: inst,
					objects: []client.Object{
						newKeySecret("default"),
						&v1.Secret{
							ObjectMeta: metav1.ObjectMeta{Name: "new-fulcio", Namespace: "default"},
							Data:       map[string][]byte{"cert": cert},
						},
						// Config secret has only the original single root cert annotation
						newConfigSecret("old-config", "default", defaultAnnotations()),
					},
				}
			}(),
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, cli client.Client, current *rhtasv1alpha1.CTlog) {
					g.Expect(current.Status.ServerConfigRef.Name).ShouldNot(Equal("old-config"))

					secret, err := kubernetes.GetSecret(cli, "default", current.Status.ServerConfigRef.Name)
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(secret.Data).To(HaveKey("config"))
					g.Expect(secret.Annotations).To(HaveKeyWithValue(
						"rhtas.redhat.com/rootCertificates", "secret/cert,new-fulcio/cert",
					))
				},
			},
		},
		{
			name: "trillian address defaults when empty",
			env: func() env {
				inst := newBaseInstance()
				inst.Namespace = "mynamespace"
				inst.Spec.Trillian.Port = ptr.To(int32(8091))
				inst.Spec.Trillian.Address = "" // Should default to trillian-logserver.mynamespace.svc
				inst.Status.ServerConfigRef = nil
				return env{
					instance: inst,
					objects:  []client.Object{newKeySecret("mynamespace")},
				}
			}(),
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, cli client.Client, current *rhtasv1alpha1.CTlog) {
					g.Expect(current.Status.ServerConfigRef).ShouldNot(BeNil())

					data, err := kubernetes.GetSecretData(cli, "mynamespace", &rhtasv1alpha1.SecretKeySelector{
						LocalObjectReference: *current.Status.ServerConfigRef, Key: "config",
					})
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(data).To(ContainSubstring("trillian-logserver.mynamespace.svc:8091"))

					secret, err := kubernetes.GetSecret(cli, "mynamespace", current.Status.ServerConfigRef.Name)
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(secret.Annotations).To(HaveKeyWithValue(
						"rhtas.redhat.com/trillianUrl", "trillian-logserver.mynamespace.svc:8091",
					))
				},
			},
		},
		{
			name: "use custom config and ignore other changes",
			env: func() env {
				inst := newBaseInstance()
				inst.Generation = 2
				inst.Spec.ServerConfigRef = &rhtasv1alpha1.LocalObjectReference{Name: "custom_config"}
				inst.Status.ServerConfigRef = &rhtasv1alpha1.LocalObjectReference{Name: "old_secret"}
				// Only ReadyCondition, no ConfigCondition
				inst.Status.Conditions = []metav1.Condition{{
					Type:               constants.ReadyCondition,
					Reason:             state.Ready.String(),
					ObservedGeneration: 1,
				}}
				return env{
					instance: inst,
					objects: []client.Object{
						&v1.Secret{
							ObjectMeta: metav1.ObjectMeta{Name: "custom_config", Namespace: "default"},
							Data: errors.IgnoreError(ctlogUtils.CreateCtlogConfig(
								"trillian-logserver.custom.svc:80", 9999999,
								[]ctlogUtils.RootCertificate{cert},
								&ctlogUtils.KeyConfig{PrivateKey: privateKey, PublicKey: publicKey, PrivateKeyPass: []byte("secure")},
							)),
						},
					},
				}
			}(),
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, cli client.Client, current *rhtasv1alpha1.CTlog) {
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

func TestServerConfig_Prerequisites(t *testing.T) {
	newBaseInstance := func() rhtasv1alpha1.CTlog {
		return rhtasv1alpha1.CTlog{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test",
				Namespace:  "default",
				Generation: 1,
			},
			Spec: rhtasv1alpha1.CTlogSpec{
				Trillian: rhtasv1alpha1.TrillianService{Port: ptr.To(int32(80))},
			},
			Status: rhtasv1alpha1.CTlogStatus{
				ServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: "existing-config"},
				TreeID:          ptr.To(int64(123456)),
				RootCertificates: []rhtasv1alpha1.SecretKeySelector{
					{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "cert"},
				},
				PrivateKeyRef:         &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "private"},
				PrivateKeyPasswordRef: &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "password"},
				PublicKeyRef:          &rhtasv1alpha1.SecretKeySelector{LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "secret"}, Key: "public"},
				Conditions: []metav1.Condition{
					{
						Type:               constants.ReadyCondition,
						Reason:             state.Ready.String(),
						ObservedGeneration: 1,
					},
					{
						Type:               ConfigCondition,
						Status:             metav1.ConditionTrue,
						Reason:             state.Ready.String(),
						Message:            "Server config created",
						ObservedGeneration: 1,
					},
				},
			},
		}
	}

	type testCase struct {
		name    string
		setup   func() *rhtasv1alpha1.CTlog // Build instance from base with modifications
		objects []client.Object
		verify  func(Gomega, *action.Result, *rhtasv1alpha1.CTlog)
	}

	tests := []testCase{
		{
			name: "custom server config secret not found",
			setup: func() *rhtasv1alpha1.CTlog {
				inst := newBaseInstance()
				inst.Spec.ServerConfigRef = &rhtasv1alpha1.LocalObjectReference{Name: "missing-config"}
				inst.Status = rhtasv1alpha1.CTlogStatus{}
				return &inst
			},
			verify: func(g Gomega, result *action.Result, instance *rhtasv1alpha1.CTlog) {
				g.Expect(action.IsError(result)).To(BeTrue(), "expected error result")
				g.Expect(result.Err.Error()).To(ContainSubstring("error accessing custom server config secret"))

				c := meta.FindStatusCondition(instance.Status.Conditions, ConfigCondition)
				g.Expect(c).ShouldNot(BeNil())
				g.Expect(c.Status).Should(Equal(metav1.ConditionFalse))
				g.Expect(c.Reason).Should(Equal(state.Failure.String()))
				g.Expect(c.Message).Should(ContainSubstring("missing-config"))
			},
		},
		{
			name: "custom server config secret missing config key",
			setup: func() *rhtasv1alpha1.CTlog {
				inst := newBaseInstance()
				inst.Spec.ServerConfigRef = &rhtasv1alpha1.LocalObjectReference{Name: "bad-config"}
				inst.Status = rhtasv1alpha1.CTlogStatus{}
				return &inst
			},
			objects: []client.Object{
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "bad-config", Namespace: "default"},
					Data:       map[string][]byte{"wrong-key": []byte("data")},
				},
			},
			verify: func(g Gomega, result *action.Result, instance *rhtasv1alpha1.CTlog) {
				g.Expect(action.IsError(result)).To(BeTrue(), "expected error result")
				g.Expect(result.Err.Error()).To(ContainSubstring("custom server config secret is invalid"))

				c := meta.FindStatusCondition(instance.Status.Conditions, ConfigCondition)
				g.Expect(c).ShouldNot(BeNil())
				g.Expect(c.Status).Should(Equal(metav1.ConditionFalse))
				g.Expect(c.Message).Should(ContainSubstring("missing '" + ctlogUtils.ConfigKey + "' key"))
			},
		},
		{
			name: "error when TreeID is nil",
			setup: func() *rhtasv1alpha1.CTlog {
				inst := newBaseInstance()
				inst.Status.TreeID = nil
				return &inst
			},
			verify: func(g Gomega, result *action.Result, _ *rhtasv1alpha1.CTlog) {
				g.Expect(action.IsError(result)).To(BeTrue(), "expected error result")
				g.Expect(result.Err.Error()).To(ContainSubstring("tree not specified"))
			},
		},
		{
			name: "error when PrivateKeyRef is nil",
			setup: func() *rhtasv1alpha1.CTlog {
				inst := newBaseInstance()
				inst.Status.PrivateKeyRef = nil
				return &inst
			},
			verify: func(g Gomega, result *action.Result, _ *rhtasv1alpha1.CTlog) {
				g.Expect(action.IsError(result)).To(BeTrue(), "expected error result")
				g.Expect(result.Err.Error()).To(ContainSubstring("private key not specified"))
			},
		},
		{
			name: "terminal error when Trillian.Port is nil",
			setup: func() *rhtasv1alpha1.CTlog {
				inst := newBaseInstance()
				inst.Spec.Trillian.Port = nil
				return &inst
			},
			verify: func(g Gomega, result *action.Result, instance *rhtasv1alpha1.CTlog) {
				g.Expect(action.IsError(result)).To(BeTrue(), "expected error result")
				g.Expect(result.Err.Error()).To(ContainSubstring("trillian port not specified"))

				// TerminalError should set ReadyCondition to False
				c := meta.FindStatusCondition(instance.Status.Conditions, constants.ReadyCondition)
				g.Expect(c).ShouldNot(BeNil())
				g.Expect(c.Status).Should(Equal(metav1.ConditionFalse))
				g.Expect(c.Reason).Should(Equal(state.Failure.String()))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := context.TODO()
			instance := tt.setup()

			// Ensure ConfigCondition exists (required by CanHandle)
			if meta.FindStatusCondition(instance.Status.Conditions, ConfigCondition) == nil {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:               ConfigCondition,
					Status:             metav1.ConditionTrue,
					Reason:             state.Ready.String(),
					ObservedGeneration: 1,
				})
			}

			builder := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance)
			if len(tt.objects) > 0 {
				builder = builder.WithObjects(tt.objects...)
			}
			c := builder.Build()

			a := testAction.PrepareAction(c, NewServerConfigAction())
			result := a.Handle(ctx, instance)
			tt.verify(g, result, instance)
		})
	}
}
