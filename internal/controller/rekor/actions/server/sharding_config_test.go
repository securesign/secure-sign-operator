package server

import (
	"context"
	"reflect"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	testAction "github.com/securesign/operator/internal/testing/action"
	"github.com/securesign/operator/internal/testing/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

func TestShardingConfig_CanHandle(t *testing.T) {
	tests := []struct {
		name      string
		phase     string
		canHandle bool
	}{
		{
			name:      "no phase condition",
			phase:     "",
			canHandle: false,
		},
		{
			name:      constants.Ready,
			phase:     constants.Ready,
			canHandle: true,
		},
		{
			name:      constants.Pending,
			phase:     constants.Pending,
			canHandle: false,
		},
		{
			name:      constants.Creating,
			phase:     constants.Creating,
			canHandle: true,
		},
		{
			name:      constants.Initialize,
			phase:     constants.Initialize,
			canHandle: false,
		},
		{
			name:      constants.Failure,
			phase:     constants.Failure,
			canHandle: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testAction.FakeClientBuilder().Build()
			a := testAction.PrepareAction(c, NewShardingConfigAction())
			instance := rhtasv1alpha1.Rekor{
				Spec: rhtasv1alpha1.RekorSpec{
					Sharding: []rhtasv1alpha1.RekorLogRange{
						{
							TreeID:     123456,
							TreeLength: 1,
						},
					},
				},
			}
			if tt.phase != "" {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:   actions.ServerCondition,
					Reason: tt.phase,
				})
			}

			if got := a.CanHandle(context.TODO(), &instance); !reflect.DeepEqual(got, tt.canHandle) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.canHandle)
			}
		})
	}
}

func TestShardingConfig_Handle(t *testing.T) {
	rekorNN := types.NamespacedName{Name: "rekor", Namespace: "default"}

	type env struct {
		spec    rhtasv1alpha1.RekorSpec
		objects []client.Object
		status  rhtasv1alpha1.RekorStatus
	}
	type want struct {
		result          *action.Result
		serverCondition string
		verify          func(Gomega, client.WithWatch)
	}
	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "create empty sharding config",
			env: env{
				spec: rhtasv1alpha1.RekorSpec{
					Sharding: make([]rhtasv1alpha1.RekorLogRange, 0),
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, c client.WithWatch) {
					r := rhtasv1alpha1.Rekor{}
					g.Expect(c.Get(context.TODO(), rekorNN, &r)).To(Succeed())
					g.Expect(r.Status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(r.Status.ServerConfigRef.Name).Should(ContainSubstring(cmName))

					cm := v1.ConfigMap{}
					g.Expect(c.Get(context.TODO(), types.NamespacedName{Name: r.Status.ServerConfigRef.Name, Namespace: rekorNN.Namespace}, &cm)).To(Succeed())
					g.Expect(cm.Data).Should(HaveKeyWithValue(shardingConfigName, ""))
				},
			},
		},
		{
			name: "create sharding config with 2 shards",
			env: env{
				spec: rhtasv1alpha1.RekorSpec{
					Sharding: []rhtasv1alpha1.RekorLogRange{
						{
							TreeID:           222222,
							TreeLength:       10,
							EncodedPublicKey: "LS0tLS1CRUdJTiBQVUJMSUMgS0VZLS0tLS0=",
						},
						{
							TreeID:           333333,
							TreeLength:       20,
							EncodedPublicKey: "LS0tLS1CRUdJTiBQVUJMSUMgS0VZLS0tLS0=",
						},
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, c client.WithWatch) {
					r := rhtasv1alpha1.Rekor{}
					g.Expect(c.Get(context.TODO(), rekorNN, &r)).To(Succeed())
					g.Expect(r.Status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(r.Status.ServerConfigRef.Name).Should(ContainSubstring(cmName))

					cm := v1.ConfigMap{}
					g.Expect(c.Get(context.TODO(), types.NamespacedName{Name: r.Status.ServerConfigRef.Name, Namespace: rekorNN.Namespace}, &cm)).To(Succeed())
					g.Expect(cm.Data).Should(HaveKey(shardingConfigName))

					rlr := make([]rhtasv1alpha1.RekorLogRange, 0)
					g.Expect(yaml.Unmarshal([]byte(cm.Data[shardingConfigName]), &rlr)).To(Succeed())
					g.Expect(rlr).Should(Equal(r.Spec.Sharding))
				},
			},
		},
		{
			name: "update sharding config",
			env: env{
				spec: rhtasv1alpha1.RekorSpec{
					Sharding: []rhtasv1alpha1.RekorLogRange{
						{
							TreeID:     111111,
							TreeLength: 10,
						},
						{
							TreeID:     222222,
							TreeLength: 10,
						},
					},
				},
				status: rhtasv1alpha1.RekorStatus{
					ServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: cmName + "old"},
				},
				objects: []client.Object{
					kubernetes.CreateConfigmap(
						"default",
						cmName+"old",
						map[string]string{},
						errors.IgnoreError(createShardingConfigData([]rhtasv1alpha1.RekorLogRange{
							{
								TreeID:     111111,
								TreeLength: 10,
							},
						}))),
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, c client.WithWatch) {
					r := rhtasv1alpha1.Rekor{}
					g.Expect(c.Get(context.TODO(), rekorNN, &r)).To(Succeed())
					g.Expect(r.Status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(r.Status.ServerConfigRef.Name).Should(ContainSubstring(cmName))
					g.Expect(r.Status.ServerConfigRef.Name).ShouldNot(Equal(cmName + "old"))

					cm := v1.ConfigMap{}
					g.Expect(c.Get(context.TODO(), types.NamespacedName{Name: r.Status.ServerConfigRef.Name, Namespace: rekorNN.Namespace}, &cm)).To(Succeed())
					g.Expect(cm.Data).Should(HaveKey(shardingConfigName))

					rlr := make([]rhtasv1alpha1.RekorLogRange, 0)
					g.Expect(yaml.Unmarshal([]byte(cm.Data[shardingConfigName]), &rlr)).To(Succeed())
					g.Expect(rlr).Should(Equal(r.Spec.Sharding))
				},
			},
		},
		{
			name: "update empty sharding config",
			env: env{
				spec: rhtasv1alpha1.RekorSpec{
					Sharding: []rhtasv1alpha1.RekorLogRange{
						{
							TreeID:     123456,
							TreeLength: 10,
						},
					},
				},
				status: rhtasv1alpha1.RekorStatus{
					ServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: cmName + "old"},
				},
				objects: []client.Object{
					kubernetes.CreateConfigmap(
						"default",
						cmName+"old",
						map[string]string{},
						errors.IgnoreError(createShardingConfigData([]rhtasv1alpha1.RekorLogRange{}))),
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, c client.WithWatch) {
					r := rhtasv1alpha1.Rekor{}
					g.Expect(c.Get(context.TODO(), rekorNN, &r)).To(Succeed())
					g.Expect(r.Status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(r.Status.ServerConfigRef.Name).Should(ContainSubstring(cmName))
					g.Expect(r.Status.ServerConfigRef.Name).ShouldNot(Equal(cmName + "old"))

					cm := v1.ConfigMap{}
					g.Expect(c.Get(context.TODO(), types.NamespacedName{Name: r.Status.ServerConfigRef.Name, Namespace: rekorNN.Namespace}, &cm)).To(Succeed())
					g.Expect(cm.Data).Should(HaveKey(shardingConfigName))

					rlr := make([]rhtasv1alpha1.RekorLogRange, 0)
					g.Expect(yaml.Unmarshal([]byte(cm.Data[shardingConfigName]), &rlr)).To(Succeed())
					g.Expect(rlr).Should(Equal(r.Spec.Sharding))
				},
			},
		},
		{
			name: "spec.sharding == sharding ConfigMap (empty)",
			env: env{
				spec: rhtasv1alpha1.RekorSpec{},
				status: rhtasv1alpha1.RekorStatus{
					ServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: cmName + "old"},
				},
				objects: []client.Object{
					kubernetes.CreateConfigmap(
						"default",
						cmName+"old",
						map[string]string{},
						errors.IgnoreError(createShardingConfigData([]rhtasv1alpha1.RekorLogRange{}))),
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(g Gomega, c client.WithWatch) {
					r := rhtasv1alpha1.Rekor{}
					g.Expect(c.Get(context.TODO(), rekorNN, &r)).To(Succeed())
					g.Expect(r.Status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(r.Status.ServerConfigRef.Name).Should(Equal(cmName + "old"))

					cm := v1.ConfigMap{}
					g.Expect(c.Get(context.TODO(), types.NamespacedName{Name: r.Status.ServerConfigRef.Name, Namespace: rekorNN.Namespace}, &cm)).To(Succeed())
					g.Expect(cm.Data).Should(HaveKeyWithValue(shardingConfigName, ""))

					rlr := make([]rhtasv1alpha1.RekorLogRange, 0)
					g.Expect(yaml.Unmarshal([]byte(cm.Data[shardingConfigName]), &rlr)).To(Succeed())
					g.Expect(rlr).Should(BeEmpty())
				},
			},
		},
		{
			name: "spec.sharding == sharding ConfigMap",
			env: env{
				spec: rhtasv1alpha1.RekorSpec{
					Sharding: []rhtasv1alpha1.RekorLogRange{
						{
							TreeID:     111111,
							TreeLength: 10,
						},
					},
				},
				status: rhtasv1alpha1.RekorStatus{
					ServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: cmName + "old"},
				},
				objects: []client.Object{
					kubernetes.CreateConfigmap(
						"default",
						cmName+"old",
						map[string]string{},
						errors.IgnoreError(createShardingConfigData([]rhtasv1alpha1.RekorLogRange{
							{
								TreeID:     111111,
								TreeLength: 10,
							},
						}))),
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(g Gomega, c client.WithWatch) {
					r := rhtasv1alpha1.Rekor{}
					g.Expect(c.Get(context.TODO(), rekorNN, &r)).To(Succeed())
					g.Expect(r.Status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(r.Status.ServerConfigRef.Name).Should(Equal(cmName + "old"))

					cm := v1.ConfigMap{}
					g.Expect(c.Get(context.TODO(), types.NamespacedName{Name: r.Status.ServerConfigRef.Name, Namespace: rekorNN.Namespace}, &cm)).To(Succeed())
					g.Expect(cm.Data).Should(HaveKey(shardingConfigName))

					rlr := make([]rhtasv1alpha1.RekorLogRange, 0)
					g.Expect(yaml.Unmarshal([]byte(cm.Data[shardingConfigName]), &rlr)).To(Succeed())
					g.Expect(rlr).Should(Equal(r.Spec.Sharding))
				},
			},
		},
		{
			name: "status.serverConfigRef not found",
			env: env{
				spec: rhtasv1alpha1.RekorSpec{},
				status: rhtasv1alpha1.RekorStatus{
					ServerConfigRef: &rhtasv1alpha1.LocalObjectReference{Name: cmName + "deleted"},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, c client.WithWatch) {
					r := rhtasv1alpha1.Rekor{}
					g.Expect(c.Get(context.TODO(), rekorNN, &r)).To(Succeed())
					g.Expect(r.Status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(r.Status.ServerConfigRef.Name).ShouldNot(Equal(cmName + "deleted"))
					g.Expect(r.Status.ServerConfigRef.Name).Should(ContainSubstring(cmName))

					cm := v1.ConfigMap{}
					g.Expect(c.Get(context.TODO(), types.NamespacedName{Name: r.Status.ServerConfigRef.Name, Namespace: rekorNN.Namespace}, &cm)).To(Succeed())
					g.Expect(cm.Data).Should(HaveKeyWithValue(shardingConfigName, ""))
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := context.TODO()
			instance := &rhtasv1alpha1.Rekor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rekor",
					Namespace: "default",
				},
				Spec:   tt.env.spec,
				Status: tt.env.status,
			}

			meta.SetStatusCondition(&instance.Status.Conditions,
				metav1.Condition{Type: constants.Ready, Reason: constants.Creating},
			)

			c := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).
				WithObjects(tt.env.objects...).
				Build()

			a := testAction.PrepareAction(c, NewShardingConfigAction())

			if got := a.Handle(ctx, instance); !reflect.DeepEqual(got, tt.want.result) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want.result)
			}
			if tt.want.verify != nil {
				tt.want.verify(g, c)
			}
		})
	}
}
