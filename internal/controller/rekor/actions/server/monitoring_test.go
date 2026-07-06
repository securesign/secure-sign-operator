package server

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	testaction "github.com/securesign/operator/internal/testing/action"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestRekorServerMonitoringHandle_NoServiceMonitorCRD(t *testing.T) {
	g := NewWithT(t)

	cl := testaction.FakeClientBuilder().
		WithStatusSubresource(&rhtasv1.Rekor{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if obj.GetObjectKind().GroupVersionKind().Kind == "ServiceMonitor" {
					return &apimeta.NoKindMatchError{
						GroupKind:        schema.GroupKind{Group: "monitoring.coreos.com", Kind: "ServiceMonitor"},
						SearchedVersions: []string{"v1"},
					}
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}).
		Build()

	a := testaction.PrepareAction(cl, NewCreateMonitorAction())

	instance := &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: rhtasv1.RekorSpec{
			Monitoring: rhtasv1.MonitoringWithTLogConfig{
				MonitoringConfig: rhtasv1.MonitoringConfig{
					Enabled: ptr.To(true),
				},
			},
		},
		Status: rhtasv1.RekorStatus{
			Conditions: []metav1.Condition{
				{
					Type:   constants.ReadyCondition,
					Reason: state.Creating.String(),
					Status: metav1.ConditionFalse,
				},
			},
		},
	}

	result := a.Handle(context.Background(), instance)

	g.Expect(result.Err).To(MatchError(ContainSubstring("ServiceMonitor CRD is not installed")))
}
