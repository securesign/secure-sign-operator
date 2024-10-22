package actions

import (
	"context"
	_ "embed"
	"reflect"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/constants"
	testAction "github.com/securesign/operator/internal/testing/action"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	//go:embed testdata/config.yaml
	configYaml []byte
	//go:embed testdata/config.json
	configJson []byte
)

func TestServerConfig_CanHandle(t *testing.T) {
	type env struct {
		objects []client.Object
	}
	tests := []struct {
		name            string
		phase           string
		canHandle       bool
		config          rhtasv1alpha1.FulcioConfig
		statusConfigRef *rhtasv1alpha1.LocalObjectReference
		env             env
	}{
		{
			name: "config.json",
			config: rhtasv1alpha1.FulcioConfig{
				OIDCIssuers: []rhtasv1alpha1.OIDCIssuer{
					{
						Issuer:    "https://example.com",
						IssuerURL: "https://example.com",
						ClientID:  "client-id",
						Type:      "email",
					},
				},
			},
			statusConfigRef: &rhtasv1alpha1.LocalObjectReference{
				Name: "config",
			},
			env: env{
				objects: []client.Object{
					kubernetes.CreateConfigmap("default", "config", map[string]string{}, map[string]string{
						"config.json": string(configJson),
					}),
				},
			},
			canHandle: true,
			phase:     constants.Ready,
		},
		{
			name: "same config.yaml",
			config: rhtasv1alpha1.FulcioConfig{
				OIDCIssuers: []rhtasv1alpha1.OIDCIssuer{
					{
						Issuer:    "https://example.com",
						IssuerURL: "https://example.com",
						ClientID:  "client-id",
						Type:      "email",
					},
				},
			},
			statusConfigRef: &rhtasv1alpha1.LocalObjectReference{
				Name: "config",
			},
			env: env{
				objects: []client.Object{
					kubernetes.CreateConfigmap("default", "config", map[string]string{}, map[string]string{
						serverConfigName: string(configYaml),
					}),
				},
			},
			canHandle: true,
			phase:     constants.Ready,
		},
		{
			name: "different config.yaml",
			config: rhtasv1alpha1.FulcioConfig{
				OIDCIssuers: []rhtasv1alpha1.OIDCIssuer{
					{
						Issuer:    "https://new.com",
						IssuerURL: "https://new.com",
						ClientID:  "client-id",
						Type:      "email",
					},
				},
			},
			statusConfigRef: &rhtasv1alpha1.LocalObjectReference{
				Name: "config",
			},
			env: env{
				objects: []client.Object{
					kubernetes.CreateConfigmap("default", "config", map[string]string{}, map[string]string{
						serverConfigName: string(configYaml),
					}),
				},
			},
			canHandle: true,
			phase:     constants.Ready,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			c := testAction.FakeClientBuilder().
				WithObjects(tt.env.objects...).
				Build()
			a := testAction.PrepareAction(c, NewServerConfigAction())

			instance := rhtasv1alpha1.Fulcio{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "instance",
					Namespace: "default",
				},
				Spec: rhtasv1alpha1.FulcioSpec{
					Config: tt.config,
				},
				Status: rhtasv1alpha1.FulcioStatus{
					ServerConfigRef: tt.statusConfigRef,
				},
			}
			if tt.phase != "" {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:   constants.Ready,
					Reason: tt.phase,
				})
			}

			if got := a.CanHandle(context.TODO(), &instance); !reflect.DeepEqual(got, tt.canHandle) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.canHandle)
			}
		})
	}
}

func TestConfig_Handle(t *testing.T) {
	labels := constants.LabelsFor(ComponentName, DeploymentName, "fulcio")
	labels[constants.LabelResource] = configResourceLabel

	type env struct {
		spec    rhtasv1alpha1.FulcioConfig
		objects []client.Object
		status  rhtasv1alpha1.FulcioStatus
	}
	type want struct {
		result *action.Result
		verify func(Gomega, rhtasv1alpha1.FulcioStatus, client.WithWatch, <-chan watch.Event)
	}
	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "create empty config",
			env: env{
				spec: rhtasv1alpha1.FulcioConfig{
					OIDCIssuers: []rhtasv1alpha1.OIDCIssuer{
						{
							Issuer:    "https://example.com",
							IssuerURL: "https://example.com",
							ClientID:  "client-id",
							Type:      "email",
						},
					},
				},
				status: rhtasv1alpha1.FulcioStatus{
					Conditions: []metav1.Condition{
						{Type: constants.Ready, Reason: constants.Creating},
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, status rhtasv1alpha1.FulcioStatus, cli client.WithWatch, events <-chan watch.Event) {

					g.Expect(status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(status.ServerConfigRef.Name).Should(ContainSubstring("fulcio-config-"))

					g.Expect(events).To(HaveLen(1))
					g.Expect(events).To(Receive(WithTransform(getEventType, Equal(watch.Added))))
				},
			},
		},
		{
			name: "update existing json config",
			env: env{
				spec: rhtasv1alpha1.FulcioConfig{
					OIDCIssuers: []rhtasv1alpha1.OIDCIssuer{
						{
							Issuer:    "https://example.com",
							IssuerURL: "https://example.com",
							ClientID:  "client-id",
							Type:      "email",
						},
					},
				},
				status: rhtasv1alpha1.FulcioStatus{
					ServerConfigRef: &rhtasv1alpha1.LocalObjectReference{
						Name: "config",
					},
					Conditions: []metav1.Condition{
						{Type: constants.Ready, Reason: constants.Creating},
					},
				},
				objects: []client.Object{
					kubernetes.CreateConfigmap("default", "config", map[string]string{}, map[string]string{
						"config.json": string(configJson),
					}),
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, status rhtasv1alpha1.FulcioStatus, cli client.WithWatch, events <-chan watch.Event) {
					g.Expect(status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(status.ServerConfigRef.Name).Should(Not(Equal("config")))

					g.Expect(events).To(HaveLen(2))
					for e := range events {
						g.Expect(e).To(Or(
							WithTransform(getEventType, Equal(watch.Added)),
							WithTransform(getEventType, Equal(watch.Deleted))),
						)
					}
					cm, err := kubernetes.GetConfigMap(context.TODO(), cli, "default", status.ServerConfigRef.Name)
					g.Expect(err).To(Not(HaveOccurred()))
					g.Expect(cm.Data[serverConfigName]).To(Equal(string(configYaml)))
				},
			},
		},
		{
			name: "no update on existing yaml config",
			env: env{
				spec: rhtasv1alpha1.FulcioConfig{
					OIDCIssuers: []rhtasv1alpha1.OIDCIssuer{
						{
							Issuer:    "https://example.com",
							IssuerURL: "https://example.com",
							ClientID:  "client-id",
							Type:      "email",
						},
					},
				},
				status: rhtasv1alpha1.FulcioStatus{
					ServerConfigRef: &rhtasv1alpha1.LocalObjectReference{
						Name: "config",
					},
					Conditions: []metav1.Condition{
						{Type: constants.Ready, Reason: constants.Creating},
					},
				},
				objects: []client.Object{
					kubernetes.CreateConfigmap("default", "config", labels, map[string]string{
						serverConfigName: string(configYaml),
					}),
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(g Gomega, status rhtasv1alpha1.FulcioStatus, cli client.WithWatch, events <-chan watch.Event) {
					g.Expect(status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(status.ServerConfigRef.Name).Should(Equal("config"))

					g.Expect(events).To(BeEmpty())
				},
			},
		},
		{
			name: "spec update",
			env: env{
				spec: rhtasv1alpha1.FulcioConfig{
					OIDCIssuers: []rhtasv1alpha1.OIDCIssuer{
						{
							Issuer:    "https://example.com",
							IssuerURL: "https://example.com",
							ClientID:  "clientIdUpdated",
							Type:      "email",
						},
					},
				},
				status: rhtasv1alpha1.FulcioStatus{
					ServerConfigRef: &rhtasv1alpha1.LocalObjectReference{
						Name: "config",
					},
					Conditions: []metav1.Condition{
						{Type: constants.Ready, Reason: constants.Creating},
					},
				},
				objects: []client.Object{
					kubernetes.CreateConfigmap("default", "config", labels, map[string]string{
						serverConfigName: string(configJson),
					}),
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, status rhtasv1alpha1.FulcioStatus, cli client.WithWatch, events <-chan watch.Event) {
					g.Expect(status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(status.ServerConfigRef.Name).Should(Not(Equal("config")))

					g.Expect(events).To(HaveLen(2))
					for e := range events {
						g.Expect(e).To(Or(
							WithTransform(getEventType, Equal(watch.Added)),
							WithTransform(getEventType, Equal(watch.Deleted)),
						))
					}
					cm, err := kubernetes.GetConfigMap(context.TODO(), cli, "default", status.ServerConfigRef.Name)
					g.Expect(err).To(Not(HaveOccurred()))
					g.Expect(cm.Data[serverConfigName]).To(ContainSubstring("clientIdUpdated"))
				},
			},
		},
		{
			name: "discover unlinked configmaps and delete them",
			env: env{
				spec: rhtasv1alpha1.FulcioConfig{
					OIDCIssuers: []rhtasv1alpha1.OIDCIssuer{
						{
							Issuer:    "https://example.com",
							IssuerURL: "https://example.com",
							ClientID:  "client-id",
							Type:      "email",
						},
					},
				},
				status: rhtasv1alpha1.FulcioStatus{
					Conditions: []metav1.Condition{
						{Type: constants.Ready, Reason: constants.Creating},
					},
				},
				objects: []client.Object{
					kubernetes.CreateConfigmap("default", "fake", labels, map[string]string{
						serverConfigName: "fake",
					}),
					kubernetes.CreateConfigmap("default", "config", labels, map[string]string{
						serverConfigName: string(configYaml),
					}),
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, status rhtasv1alpha1.FulcioStatus, cli client.WithWatch, events <-chan watch.Event) {
					g.Expect(status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(status.ServerConfigRef.Name).ShouldNot(Equal("config"))

					g.Expect(events).To(HaveLen(3))
					g.Expect(events).To(Receive(
						And(
							WithTransform(getEventType, Equal(watch.Added)),
							WithTransform(getEventObjectName, Equal(status.ServerConfigRef.Name)),
						)),
					)

					g.Expect(events).To(Receive(
						And(
							WithTransform(getEventType, Equal(watch.Deleted)),
							WithTransform(getEventObjectName, Equal("config")),
						)),
					)

					g.Expect(events).To(Receive(
						And(
							WithTransform(getEventType, Equal(watch.Deleted)),
							WithTransform(getEventObjectName, Equal("fake")),
						)),
					)
				},
			},
		},
		{
			name: "overwrite non-existing",
			env: env{
				spec: rhtasv1alpha1.FulcioConfig{
					OIDCIssuers: []rhtasv1alpha1.OIDCIssuer{
						{
							Issuer:    "https://example.com",
							IssuerURL: "https://example.com",
							ClientID:  "client-id",
							Type:      "email",
						},
					},
				},
				status: rhtasv1alpha1.FulcioStatus{
					ServerConfigRef: &rhtasv1alpha1.LocalObjectReference{
						Name: "config",
					},
					Conditions: []metav1.Condition{
						{Type: constants.Ready, Reason: constants.Creating},
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, status rhtasv1alpha1.FulcioStatus, cli client.WithWatch, events <-chan watch.Event) {
					g.Expect(status.ServerConfigRef).ShouldNot(BeNil())
					g.Expect(status.ServerConfigRef.Name).Should(Not(Equal("config")))

					g.Expect(events).To(HaveLen(1))
					g.Expect(events).To(Receive(
						WithTransform(getEventType, Equal(watch.Added)),
					),
					)
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := context.TODO()
			instance := &rhtasv1alpha1.Fulcio{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fulcio",
					Namespace: "default",
				},
				Spec: rhtasv1alpha1.FulcioSpec{
					Config: tt.env.spec,
				},
				Status: tt.env.status,
			}

			c := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).
				WithObjects(tt.env.objects...).
				Build()

			watchCm, err := c.Watch(ctx, &core.ConfigMapList{}, client.InNamespace("default"))
			g.Expect(err).To(Not(HaveOccurred()))

			a := testAction.PrepareAction(c, NewServerConfigAction())

			if got := a.Handle(ctx, instance); !reflect.DeepEqual(got, tt.want.result) {
				t.Errorf("Handle() = %v, want %v", got, tt.want.result)
			}
			watchCm.Stop()
			if tt.want.verify != nil {
				find := &rhtasv1alpha1.Fulcio{}
				g.Expect(c.Get(ctx, client.ObjectKeyFromObject(instance), find)).To(Succeed())
				tt.want.verify(g, find.Status, c, watchCm.ResultChan())
			}
		})
	}
}

func getEventType(e watch.Event) watch.EventType {
	return e.Type
}

func getEventObjectName(e watch.Event) string {
	return e.Object.(client.Object).GetName()
}
