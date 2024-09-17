package actions

import (
	"context"
	_ "embed"
	"reflect"
	"testing"

	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
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
						"config.yaml": string(configYaml),
					}),
				},
			},
			canHandle: false,
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
						"config.yaml": string(configYaml),
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
