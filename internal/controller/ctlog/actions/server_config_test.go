package actions

import (
	"context"
	_ "embed"
	"reflect"
	"testing"
	"time"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	v1 "k8s.io/api/core/v1"

	ctlogUtils "github.com/securesign/operator/internal/controller/ctlog/utils"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/utils/ptr"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	_ "github.com/securesign/operator/internal/controller/trillian/serviceresolver"
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
	t.Parallel()
	tests := []struct {
		name               string
		status             metav1.ConditionStatus
		canHandle          bool
		observedGeneration int64
		generation         int64
	}{
		{
			name:      "ConditionTrue: ready to handle",
			status:    metav1.ConditionTrue,
			canHandle: true,
		},
		{
			name:      "empty condition",
			status:    "",
			canHandle: false,
		},
		{
			name:      "ConditionUnknown",
			status:    metav1.ConditionUnknown,
			canHandle: true,
		},
		{
			name:      "ConditionFalse",
			status:    metav1.ConditionFalse,
			canHandle: true,
		},
		{
			name:               "generation mismatch triggers rehandle",
			status:             metav1.ConditionTrue,
			canHandle:          true,
			observedGeneration: 1,
			generation:         2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := testAction.FakeClientBuilder().Build()
			a := testAction.PrepareAction(c, NewServerConfigAction())
			instance := rhtasv1.CTlog{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "default",
					Generation: tt.generation,
				},
				Spec:   rhtasv1.CTlogSpec{},
				Status: rhtasv1.CTlogStatus{},
			}
			if tt.status != "" {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:               ConfigCondition,
					Status:             tt.status,
					ObservedGeneration: tt.observedGeneration,
				})
			}

			if got := a.CanHandle(t.Context(), &instance); !reflect.DeepEqual(got, tt.canHandle) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.canHandle)
			}
		})
	}
}

func TestServerConfig_Handle_Sharding(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	type env struct {
		spec    rhtasv1.CTlogSpec
		status  rhtasv1.CTlogStatus
		objects []client.Object
	}
	type want struct {
		result *action.Result
		verify func(context.Context, Gomega, *rhtasv1.CTlog, client.WithWatch)
	}
	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "create config with shards",
			env: env{
				spec: rhtasv1.CTlogSpec{
					Trillian: rhtasv1.ServiceReference{URL: "trillian.default.svc:8091"},
					Logs: []rhtasv1.CTLogConfig{
						{
							LogId:    ptr.To(int64(111111)),
							Prefix:   "shard-111111",
							Readonly: ptr.To(true),
							Signer: &rhtasv1.CTlogSigner{
								Type: "file",
								File: &rhtasv1.CTlogFile{
									PublicKeyRef: &rhtasv1.SecretKeySelector{
										LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-keys"},
										Key:                  "public",
									},
								},
							},
							RootCerts: []rhtasv1.SecretKeySelector{
								{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"}, Key: "cert"},
							},
						},
					},
				},
				status: rhtasv1.CTlogStatus{
					Logs: []rhtasv1.CTlogLogStatus{
						{
							LogId:  ptr.To(int64(111111)),
							Prefix: "shard-111111",
							PublicKeyRef: &rhtasv1.SecretKeySelector{
								LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-keys"},
								Key:                  "public",
							},
							PrivateKeyRef: &rhtasv1.SecretKeySelector{
								LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-keys"},
								Key:                  "private",
							},
							RootCertificates: []rhtasv1.SecretKeySelector{
								{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"}, Key: "cert"},
							},
						},
					},
				},
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: "secret", Namespace: "default"},
						Data:       map[string][]byte{"cert": cert, "private": privateKey, "public": publicKey},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: "shard-keys", Namespace: "default"},
						Data:       map[string][]byte{"public": publicKey, "private": privateKey},
					},
				},
			},
			want: want{
				result: testAction.Return(),
				verify: func(ctx context.Context, g Gomega, instance *rhtasv1.CTlog, cli client.WithWatch) {
					g.Expect(instance.Status.ServerConfigRef).ShouldNot(BeNil())

					secret, err := kubernetes.GetSecret(ctx, cli, "default", instance.Status.ServerConfigRef.Name)
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(secret.Data).To(HaveKey("config"))
					g.Expect(string(secret.Data["config"])).To(ContainSubstring("111111"))
					g.Expect(string(secret.Data["config"])).To(ContainSubstring("is_readonly:true"))
				},
			},
		},
		{
			name: "create config with readonly shard",
			env: env{
				spec: rhtasv1.CTlogSpec{
					Trillian: rhtasv1.ServiceReference{URL: "trillian.default.svc:8091"},
					Logs: []rhtasv1.CTLogConfig{
						{
							LogId:    ptr.To(int64(222222)),
							Prefix:   "shard-222222",
							Readonly: ptr.To(true),
							Signer: &rhtasv1.CTlogSigner{
								Type: "file",
								File: &rhtasv1.CTlogFile{
									PublicKeyRef: &rhtasv1.SecretKeySelector{
										LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-keys"},
										Key:                  "public",
									},
								},
							},
							RootCerts: []rhtasv1.SecretKeySelector{
								{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"}, Key: "cert"},
							},
						},
					},
				},
				status: rhtasv1.CTlogStatus{
					Logs: []rhtasv1.CTlogLogStatus{
						{
							LogId:  ptr.To(int64(222222)),
							Prefix: "shard-222222",
							PublicKeyRef: &rhtasv1.SecretKeySelector{
								LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-keys"},
								Key:                  "public",
							},
							RootCertificates: []rhtasv1.SecretKeySelector{
								{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"}, Key: "cert"},
							},
						},
					},
				},
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: "secret", Namespace: "default"},
						Data:       map[string][]byte{"cert": cert, "private": privateKey, "public": publicKey},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: "shard-keys", Namespace: "default"},
						Data:       map[string][]byte{"public": publicKey},
					},
				},
			},
			want: want{
				result: testAction.Return(),
				verify: func(ctx context.Context, g Gomega, instance *rhtasv1.CTlog, cli client.WithWatch) {
					g.Expect(instance.Status.ServerConfigRef).ShouldNot(BeNil())

					secret, err := kubernetes.GetSecret(ctx, cli, "default", instance.Status.ServerConfigRef.Name)
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(secret.Data).To(HaveKey("config"))
					g.Expect(string(secret.Data["config"])).To(ContainSubstring("222222"))
					g.Expect(string(secret.Data["config"])).To(ContainSubstring("is_readonly:true"))
				},
			},
		},
		{
			name: "create config with shard validity timestamps",
			env: env{
				spec: rhtasv1.CTlogSpec{
					Trillian: rhtasv1.ServiceReference{URL: "trillian.default.svc:8091"},
					Logs: []rhtasv1.CTLogConfig{
						{
							LogId:    ptr.To(int64(333333)),
							Prefix:   "shard-333333",
							Readonly: ptr.To(true),
							Signer: &rhtasv1.CTlogSigner{
								Type: "file",
								File: &rhtasv1.CTlogFile{
									PublicKeyRef: &rhtasv1.SecretKeySelector{
										LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-keys"},
										Key:                  "public",
									},
								},
							},
							RootCerts: []rhtasv1.SecretKeySelector{
								{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"}, Key: "cert"},
							},
							NotAfterStart: &metav1.Time{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
							NotAfterLimit: &metav1.Time{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
						},
					},
				},
				status: rhtasv1.CTlogStatus{
					Logs: []rhtasv1.CTlogLogStatus{
						{
							LogId:  ptr.To(int64(333333)),
							Prefix: "shard-333333",
							PublicKeyRef: &rhtasv1.SecretKeySelector{
								LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-keys"},
								Key:                  "public",
							},
							PrivateKeyRef: &rhtasv1.SecretKeySelector{
								LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-keys"},
								Key:                  "private",
							},
							RootCertificates: []rhtasv1.SecretKeySelector{
								{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"}, Key: "cert"},
							},
						},
					},
				},
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: "secret", Namespace: "default"},
						Data:       map[string][]byte{"cert": cert, "private": privateKey, "public": publicKey},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: "shard-keys", Namespace: "default"},
						Data:       map[string][]byte{"public": publicKey, "private": privateKey},
					},
				},
			},
			want: want{
				result: testAction.Return(),
				verify: func(ctx context.Context, g Gomega, instance *rhtasv1.CTlog, cli client.WithWatch) {
					g.Expect(instance.Status.ServerConfigRef).ShouldNot(BeNil())

					secret, err := kubernetes.GetSecret(ctx, cli, "default", instance.Status.ServerConfigRef.Name)
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(secret.Data).To(HaveKey("config"))
					g.Expect(string(secret.Data["config"])).To(ContainSubstring("333333"))
					g.Expect(string(secret.Data["config"])).To(ContainSubstring("1704067200"))
					g.Expect(string(secret.Data["config"])).To(ContainSubstring("1735689600"))
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			instance := &rhtasv1.CTlog{
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
				t.Errorf("Handle() = %v, want %v", got, tt.want.result)
			}
			if tt.want.verify != nil {
				tt.want.verify(ctx, g, instance, c)
			}
		})
	}
}

func TestServerConfig_Handle_Update_Sharding(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	newBaseInstance := func() rhtasv1.CTlog {
		return rhtasv1.CTlog{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test",
				Namespace:  "default",
				Generation: 1,
			},
			Spec: rhtasv1.CTlogSpec{
				Trillian: rhtasv1.ServiceReference{URL: "trillian-logserver.default.svc:80"},
				Logs: []rhtasv1.CTLogConfig{
					{
						LogId:  ptr.To(int64(123456)),
						Prefix: "trusted-artifact-signer",
						Active: ptr.To(true),
						Signer: &rhtasv1.CTlogSigner{Type: "file"},
						RootCerts: []rhtasv1.SecretKeySelector{
							{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"}, Key: "cert"},
						},
					},
				},
			},
			Status: rhtasv1.CTlogStatus{
				Logs: []rhtasv1.CTlogLogStatus{
					{
						Active: true,
						LogId:  ptr.To(int64(123456)),
						Prefix: "trusted-artifact-signer",
						PrivateKeyRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"}, Key: "private",
						},
						PublicKeyRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"}, Key: "public",
						},
						RootCertificates: []rhtasv1.SecretKeySelector{
							{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"}, Key: "cert"},
						},
					},
				},
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

	newConfigSecret := func(name, namespace string) *v1.Secret {
		cfg, hash, _ := ctlogUtils.CreateConfig(
			"trillian-logserver.default.svc:80",
			[]ctlogUtils.ShardConfig{
				{
					TreeID:     123456,
					Prefix:     "trusted-artifact-signer",
					PublicKey:  publicKey,
					PrivateKey: privateKey,
					RootCerts:  []ctlogUtils.RootCertificate{cert},
				},
			},
		)

		a := serverConfig{}
		annotations := a.configMatchingAnnotations("trillian-logserver.default.svc:80", hash)
		return &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Annotations: annotations},
			Data:       cfg,
		}
	}

	type env struct {
		instance rhtasv1.CTlog
		objects  []client.Object
	}
	type want struct {
		result *action.Result
		verify func(context.Context, Gomega, client.Client, *rhtasv1.CTlog)
	}
	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "sharding change triggers config recreation",
			env: func() env {
				inst := newBaseInstance()
				inst.Generation = 2
				inst.Status.ServerConfigRef = &rhtasv1.LocalObjectReference{Name: "old-config"}
				inst.Spec.Logs = append(inst.Spec.Logs, rhtasv1.CTLogConfig{
					LogId:    ptr.To(int64(444444)),
					Prefix:   "shard-444444",
					Readonly: ptr.To(true),
					Signer: &rhtasv1.CTlogSigner{
						Type: "file",
						File: &rhtasv1.CTlogFile{
							PublicKeyRef: &rhtasv1.SecretKeySelector{
								LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-keys"},
								Key:                  "public",
							},
						},
					},
					RootCerts: []rhtasv1.SecretKeySelector{
						{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"}, Key: "cert"},
					},
				})
				inst.Status.Logs = append(inst.Status.Logs, rhtasv1.CTlogLogStatus{
					LogId:  ptr.To(int64(444444)),
					Prefix: "shard-444444",
					PublicKeyRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-keys"},
						Key:                  "public",
					},
					PrivateKeyRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-keys"},
						Key:                  "private",
					},
					RootCertificates: []rhtasv1.SecretKeySelector{
						{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"}, Key: "cert"},
					},
				})
				return env{
					instance: inst,
					objects: []client.Object{
						newKeySecret("default"),
						&v1.Secret{
							ObjectMeta: metav1.ObjectMeta{Name: "shard-keys", Namespace: "default"},
							Data:       map[string][]byte{"public": publicKey, "private": privateKey},
						},
						newConfigSecret("old-config", "default"),
					},
				}
			}(),
			want: want{
				result: testAction.Return(),
				verify: func(ctx context.Context, g Gomega, cli client.Client, current *rhtasv1.CTlog) {
					g.Expect(current.Status.ServerConfigRef.Name).ShouldNot(Equal("old-config"))
					g.Expect(current.Status.ServerConfigRef.Name).Should(ContainSubstring("ctlog-config-"))

					secret, err := kubernetes.GetSecret(ctx, cli, "default", current.Status.ServerConfigRef.Name)
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(string(secret.Data["config"])).To(ContainSubstring("444444"))
					g.Expect(string(secret.Data["config"])).To(ContainSubstring("is_readonly:true"))
					g.Expect(secret.Annotations).To(HaveKey("rhtas.redhat.com/logsHash"))
				},
			},
		},
		{
			name: "fulcio cert rotation forces recreation even if annotations match",
			env: func() env {
				inst := newBaseInstance()
				inst.Status.ServerConfigRef = &rhtasv1.LocalObjectReference{Name: "old-config"}
				meta.SetStatusCondition(&inst.Status.Conditions, metav1.Condition{
					Type:    ConfigCondition,
					Status:  metav1.ConditionFalse,
					Reason:  FulcioReason,
					Message: "Fulcio certificate changed",
				})
				return env{
					instance: inst,
					objects: []client.Object{
						newKeySecret("default"),
						newConfigSecret("old-config", "default"),
					},
				}
			}(),
			want: want{
				result: testAction.Return(),
				verify: func(ctx context.Context, g Gomega, cli client.Client, current *rhtasv1.CTlog) {
					g.Expect(current.Status.ServerConfigRef.Name).ShouldNot(Equal("old-config"))
					g.Expect(current.Status.ServerConfigRef.Name).Should(ContainSubstring("ctlog-config-"))

					c := meta.FindStatusCondition(current.Status.Conditions, ConfigCondition)
					g.Expect(c).ShouldNot(BeNil())
					g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
				},
			},
		},
		{
			name: "sharding with multiple shards and private keys",
			env: func() env {
				inst := newBaseInstance()
				inst.Generation = 2
				inst.Status.ServerConfigRef = &rhtasv1.LocalObjectReference{Name: "old-config"}
				inst.Spec.Logs = append(inst.Spec.Logs,
					rhtasv1.CTLogConfig{
						LogId:    ptr.To(int64(555555)),
						Prefix:   "shard-555555",
						Readonly: ptr.To(true),
						Signer: &rhtasv1.CTlogSigner{
							Type: "file",
							File: &rhtasv1.CTlogFile{
								PublicKeyRef: &rhtasv1.SecretKeySelector{
									LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard1-keys"},
									Key:                  "public",
								},
							},
						},
						RootCerts: []rhtasv1.SecretKeySelector{
							{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"}, Key: "cert"},
						},
					},
					rhtasv1.CTLogConfig{
						LogId:    ptr.To(int64(666666)),
						Prefix:   "shard-666666",
						Readonly: ptr.To(true),
						Signer: &rhtasv1.CTlogSigner{
							Type: "file",
							File: &rhtasv1.CTlogFile{
								PublicKeyRef: &rhtasv1.SecretKeySelector{
									LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard2-keys"},
									Key:                  "public",
								},
							},
						},
						RootCerts: []rhtasv1.SecretKeySelector{
							{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"}, Key: "cert"},
						},
					},
				)
				inst.Status.Logs = append(inst.Status.Logs,
					rhtasv1.CTlogLogStatus{
						LogId:  ptr.To(int64(555555)),
						Prefix: "shard-555555",
						PublicKeyRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard1-keys"},
							Key:                  "public",
						},
						RootCertificates: []rhtasv1.SecretKeySelector{
							{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"}, Key: "cert"},
						},
					},
					rhtasv1.CTlogLogStatus{
						LogId:  ptr.To(int64(666666)),
						Prefix: "shard-666666",
						PublicKeyRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard2-keys"},
							Key:                  "public",
						},
						RootCertificates: []rhtasv1.SecretKeySelector{
							{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"}, Key: "cert"},
						},
					},
				)
				return env{
					instance: inst,
					objects: []client.Object{
						newKeySecret("default"),
						&v1.Secret{
							ObjectMeta: metav1.ObjectMeta{Name: "shard1-keys", Namespace: "default"},
							Data:       map[string][]byte{"public": publicKey, "private": privateKey},
						},
						&v1.Secret{
							ObjectMeta: metav1.ObjectMeta{Name: "shard2-keys", Namespace: "default"},
							Data:       map[string][]byte{"public": publicKey, "private": privateKey},
						},
						newConfigSecret("old-config", "default"),
					},
				}
			}(),
			want: want{
				result: testAction.Return(),
				verify: func(ctx context.Context, g Gomega, cli client.Client, current *rhtasv1.CTlog) {
					g.Expect(current.Status.ServerConfigRef.Name).ShouldNot(Equal("old-config"))

					secret, err := kubernetes.GetSecret(ctx, cli, "default", current.Status.ServerConfigRef.Name)
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(string(secret.Data["config"])).To(ContainSubstring("555555"))
					g.Expect(string(secret.Data["config"])).To(ContainSubstring("666666"))
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			c := testAction.FakeClientBuilder().
				WithObjects(&tt.env.instance).
				WithStatusSubresource(&tt.env.instance).
				WithObjects(tt.env.objects...).
				Build()

			a := testAction.PrepareAction(c, NewServerConfigAction())

			if got := a.Handle(ctx, &tt.env.instance); !reflect.DeepEqual(got, tt.want.result) {
				t.Errorf("Handle() = %v, want %v", got, tt.want.result)
			}
			if tt.want.verify != nil {
				tt.want.verify(ctx, g, c, &tt.env.instance)
			}
		})
	}
}

func TestServerConfig_PKCS11(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	type env struct {
		instance rhtasv1.CTlog
		objects  []client.Object
	}
	type want struct {
		result *action.Result
		verify func(context.Context, Gomega, client.Client, *rhtasv1.CTlog)
	}
	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "PKCS#11 mode creates config successfully",
			env: func() env {
				inst := rhtasv1.CTlog{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-pkcs11",
						Namespace:  "default",
						Generation: 1,
					},
					Spec: rhtasv1.CTlogSpec{
						Trillian: rhtasv1.ServiceReference{URL: "trillian-logserver.default.svc:8091"},
						Logs: []rhtasv1.CTLogConfig{
							{
								LogId:  ptr.To(int64(123456)),
								Prefix: "trusted-artifact-signer",
								Active: ptr.To(true),
								Signer: &rhtasv1.CTlogSigner{
									Type: rhtasv1.SignerTypePKCS11,
									PKCS11: &rhtasv1.CTlogPKCS11Config{
										PinSecretRef: &rhtasv1.SecretKeySelector{
											LocalObjectReference: rhtasv1.LocalObjectReference{Name: "pin-secret"},
											Key:                  "pin",
										},
										TokenLabel: "test-token",
										ModulePath: "/usr/lib64/pkcs11/libsofthsm2.so",
										PublicKeyRef: &rhtasv1.SecretKeySelector{
											LocalObjectReference: rhtasv1.LocalObjectReference{Name: "pubkey-secret"},
											Key:                  "public",
										},
									},
								},
								RootCerts: []rhtasv1.SecretKeySelector{
									{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "fulcio-secret"}, Key: "cert"},
								},
							},
						},
					},
					Status: rhtasv1.CTlogStatus{
						Logs: []rhtasv1.CTlogLogStatus{
							{
								Active: true,
								LogId:  ptr.To(int64(123456)),
								Prefix: "trusted-artifact-signer",
								PublicKeyRef: &rhtasv1.SecretKeySelector{
									LocalObjectReference: rhtasv1.LocalObjectReference{Name: "pubkey-secret"},
									Key:                  "public",
								},
								RootCertificates: []rhtasv1.SecretKeySelector{
									{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "fulcio-secret"}, Key: "cert"},
								},
							},
						},
						Conditions: []metav1.Condition{
							{
								Type:               constants.ReadyCondition,
								Reason:             state.Creating.String(),
								ObservedGeneration: 1,
							},
						},
					},
				}
				return env{
					instance: inst,
					objects: []client.Object{
						&v1.Secret{
							ObjectMeta: metav1.ObjectMeta{Name: "pin-secret", Namespace: "default"},
							Data:       map[string][]byte{"pin": []byte("1234")},
						},
						&v1.Secret{
							ObjectMeta: metav1.ObjectMeta{Name: "pubkey-secret", Namespace: "default"},
							Data:       map[string][]byte{"public": publicKey},
						},
						&v1.Secret{
							ObjectMeta: metav1.ObjectMeta{Name: "fulcio-secret", Namespace: "default"},
							Data:       map[string][]byte{"cert": cert},
						},
					},
				}
			}(),
			want: want{
				result: testAction.Return(),
				verify: func(ctx context.Context, g Gomega, cli client.Client, current *rhtasv1.CTlog) {
					g.Expect(current.Status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(current.Status.ServerConfigRef.Name).Should(ContainSubstring("ctlog-config-"))

					data, err := kubernetes.GetSecretData(ctx, cli, "default", &rhtasv1.SecretKeySelector{
						LocalObjectReference: *current.Status.ServerConfigRef, Key: "config",
					})
					g.Expect(err).ShouldNot(HaveOccurred())
					g.Expect(data).To(ContainSubstring("test-token"))
					g.Expect(data).To(ContainSubstring("1234"))
				},
			},
		},
		{
			name: "PKCS#11 mode with nil PinSecretRef returns error",
			env: func() env {
				inst := rhtasv1.CTlog{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-nil-pin",
						Namespace:  "default",
						Generation: 1,
					},
					Spec: rhtasv1.CTlogSpec{
						Trillian: rhtasv1.ServiceReference{URL: "trillian-logserver.default.svc:8091"},
					},
					Status: rhtasv1.CTlogStatus{
						Logs: []rhtasv1.CTlogLogStatus{
							{
								Active: true,
								LogId:  ptr.To(int64(123456)),
								Prefix: "trusted-artifact-signer",
								PublicKeyRef: &rhtasv1.SecretKeySelector{
									LocalObjectReference: rhtasv1.LocalObjectReference{Name: "pubkey-secret"},
									Key:                  "public",
								},
								RootCertificates: []rhtasv1.SecretKeySelector{
									{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "fulcio-secret"}, Key: "cert"},
								},
							},
						},
						Conditions: []metav1.Condition{
							{
								Type:               constants.ReadyCondition,
								Reason:             state.Creating.String(),
								ObservedGeneration: 1,
							},
						},
					},
				}
				return env{
					instance: inst,
					objects: []client.Object{
						&v1.Secret{
							ObjectMeta: metav1.ObjectMeta{Name: "pubkey-secret", Namespace: "default"},
							Data:       map[string][]byte{"public": publicKey},
						},
						&v1.Secret{
							ObjectMeta: metav1.ObjectMeta{Name: "fulcio-secret", Namespace: "default"},
							Data:       map[string][]byte{"cert": cert},
						},
					},
				}
			}(),
			want: want{
				result: nil, // error result
				verify: func(ctx context.Context, g Gomega, cli client.Client, current *rhtasv1.CTlog) {
					g.Expect(current.Status.ServerConfigRef).Should(BeNil())
				},
			},
		},
		{
			name: "PKCS#11 mode with nil PublicKeyRef returns error",
			env: func() env {
				inst := rhtasv1.CTlog{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-nil-pubkey",
						Namespace:  "default",
						Generation: 1,
					},
					Spec: rhtasv1.CTlogSpec{
						Trillian: rhtasv1.ServiceReference{URL: "trillian-logserver.default.svc:8091"},
					},
					Status: rhtasv1.CTlogStatus{
						Logs: []rhtasv1.CTlogLogStatus{
							{
								Active:       true,
								LogId:        ptr.To(int64(123456)),
								Prefix:       "trusted-artifact-signer",
								PublicKeyRef: nil,
								RootCertificates: []rhtasv1.SecretKeySelector{
									{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "fulcio-secret"}, Key: "cert"},
								},
							},
						},
						Conditions: []metav1.Condition{
							{
								Type:               constants.ReadyCondition,
								Reason:             state.Creating.String(),
								ObservedGeneration: 1,
							},
						},
					},
				}
				return env{
					instance: inst,
					objects: []client.Object{
						&v1.Secret{
							ObjectMeta: metav1.ObjectMeta{Name: "pin-secret", Namespace: "default"},
							Data:       map[string][]byte{"pin": []byte("1234")},
						},
						&v1.Secret{
							ObjectMeta: metav1.ObjectMeta{Name: "fulcio-secret", Namespace: "default"},
							Data:       map[string][]byte{"cert": cert},
						},
					},
				}
			}(),
			want: want{
				result: nil, // error result
				verify: func(ctx context.Context, g Gomega, cli client.Client, current *rhtasv1.CTlog) {
					g.Expect(current.Status.ServerConfigRef).Should(BeNil())
				},
			},
		},
		{
			name: "PKCS#11 mode works without PrivateKeyRef",
			env: func() env {
				inst := rhtasv1.CTlog{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-pkcs11-no-privkey",
						Namespace:  "default",
						Generation: 1,
					},
					Spec: rhtasv1.CTlogSpec{
						Trillian: rhtasv1.ServiceReference{URL: "trillian-logserver.default.svc:8091"},
					},
					Status: rhtasv1.CTlogStatus{
						Logs: []rhtasv1.CTlogLogStatus{
							{
								Active: true,
								LogId:  ptr.To(int64(123456)),
								Prefix: "trusted-artifact-signer",
								PublicKeyRef: &rhtasv1.SecretKeySelector{
									LocalObjectReference: rhtasv1.LocalObjectReference{Name: "pubkey-secret"},
									Key:                  "public",
								},
								RootCertificates: []rhtasv1.SecretKeySelector{
									{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "fulcio-secret"}, Key: "cert"},
								},
							},
						},
						Conditions: []metav1.Condition{
							{
								Type:               constants.ReadyCondition,
								Reason:             state.Creating.String(),
								ObservedGeneration: 1,
							},
						},
					},
				}
				return env{
					instance: inst,
					objects: []client.Object{
						&v1.Secret{
							ObjectMeta: metav1.ObjectMeta{Name: "pin-secret", Namespace: "default"},
							Data:       map[string][]byte{"pin": []byte("1234")},
						},
						&v1.Secret{
							ObjectMeta: metav1.ObjectMeta{Name: "pubkey-secret", Namespace: "default"},
							Data:       map[string][]byte{"public": publicKey},
						},
						&v1.Secret{
							ObjectMeta: metav1.ObjectMeta{Name: "fulcio-secret", Namespace: "default"},
							Data:       map[string][]byte{"cert": cert},
						},
					},
				}
			}(),
			want: want{
				result: testAction.Return(),
				verify: func(ctx context.Context, g Gomega, cli client.Client, current *rhtasv1.CTlog) {
					g.Expect(current.Status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(current.Status.ServerConfigRef.Name).Should(ContainSubstring("ctlog-config-"))
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()

			i := tt.env.instance.DeepCopy()
			i.Spec.SetDefaults()

			c := testAction.FakeClientBuilder().
				WithObjects(i).
				WithStatusSubresource(i).
				WithObjects(tt.env.objects...).
				Build()

			a := testAction.PrepareAction(c, NewServerConfigAction())
			result := a.Handle(ctx, i)

			if tt.want.result != nil {
				if !reflect.DeepEqual(result, tt.want.result) {
					t.Errorf("Handle() = %v, want %v", result, tt.want.result)
				}
			} else {
				g.Expect(action.IsError(result)).To(BeTrue(), "expected error result")
			}
			if tt.want.verify != nil {
				tt.want.verify(ctx, g, c, i)
			}
		})
	}
}

func TestServerConfig_Prerequisites(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name    string
		setup   func() *rhtasv1.CTlog
		objects []client.Object
		verify  func(Gomega, *action.Result, *rhtasv1.CTlog)
	}

	tests := []testCase{
		{
			name: "error when no logs in status",
			setup: func() *rhtasv1.CTlog {
				return &rhtasv1.CTlog{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test",
						Namespace:  "default",
						Generation: 1,
					},
					Spec: rhtasv1.CTlogSpec{
						Trillian: rhtasv1.ServiceReference{URL: "trillian.default.svc:8091"},
					},
					Status: rhtasv1.CTlogStatus{
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
								ObservedGeneration: 1,
							},
						},
					},
				}
			},
			verify: func(g Gomega, result *action.Result, _ *rhtasv1.CTlog) {
				g.Expect(action.IsError(result)).To(BeTrue(), "expected error result")
				g.Expect(result.Err.Error()).To(ContainSubstring("no logs in status"))
			},
		},
		{
			name: "error when log has no LogId",
			setup: func() *rhtasv1.CTlog {
				return &rhtasv1.CTlog{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test",
						Namespace:  "default",
						Generation: 1,
					},
					Spec: rhtasv1.CTlogSpec{
						Trillian: rhtasv1.ServiceReference{URL: "trillian.default.svc:8091"},
					},
					Status: rhtasv1.CTlogStatus{
						Logs: []rhtasv1.CTlogLogStatus{
							{
								Active: true,
								Prefix: "trusted-artifact-signer",
								LogId:  nil,
							},
						},
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
								ObservedGeneration: 1,
							},
						},
					},
				}
			},
			verify: func(g Gomega, result *action.Result, _ *rhtasv1.CTlog) {
				g.Expect(action.IsError(result)).To(BeTrue(), "expected error result")
				g.Expect(result.Err.Error()).To(ContainSubstring("has no LogId"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			ctx := t.Context()
			instance := tt.setup()

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
