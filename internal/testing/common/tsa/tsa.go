package common

import (
	"testing"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GenerateTSAInstance() *rhtasv1.TimestampAuthority {
	return &rhtasv1.TimestampAuthority{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "timestampAuthority",
			Namespace: "default",
		},
		Status: rhtasv1.TimestampAuthorityStatus{
			Conditions: []metav1.Condition{
				{
					Type:   constants.ReadyCondition,
					Reason: state.Ready.String(),
				},
			},
			NtpConfigRef: nil,
			Signer:       nil,
		},
		Spec: rhtasv1.TimestampAuthoritySpec{
			Signer: rhtasv1.TimestampAuthoritySigner{
				CertificateChain: rhtasv1.CertificateChain{
					RootCA: &rhtasv1.TsaCertificateAuthority{
						OrganizationName: "Red Hat",
					},
					IntermediateCA: []*rhtasv1.TsaCertificateAuthority{
						{
							OrganizationName: "Red Hat",
						},
					},
					LeafCA: &rhtasv1.TsaCertificateAuthority{
						OrganizationName: "Red Hat",
					},
				},
			},
			NTPMonitoring: rhtasv1.NTPMonitoring{
				Enabled: ptr.To(true),
				Config: &rhtasv1.NtpMonitoringConfig{
					RequestAttempts: 3,
					RequestTimeout:  5,
					NumServers:      4,
					ServerThreshold: 3,
					MaxTimeDelta:    6,
					Period:          60,
					Servers:         []string{"time.apple.com", "time.google.com", "time-a-b.nist.gov", "time-b-b.nist.gov", "gbg1.ntp.se"},
				},
			},
		},
	}
}

func TsaTestSetup(instance *rhtasv1.TimestampAuthority, t *testing.T, client client.WithWatch, action action.Action[*rhtasv1.TimestampAuthority], initObjs ...client.Object) (client.WithWatch, action.Action[*rhtasv1.TimestampAuthority]) {
	if client == nil {
		client = testAction.FakeClientBuilder().WithObjects(instance).WithStatusSubresource(instance).Build()
	}
	if err := client.Get(t.Context(), types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}, instance); err != nil {
		t.Error(err)
		return nil, nil
	}

	for _, obj := range initObjs {
		if err := client.Create(t.Context(), obj); err != nil {
			t.Error(err)
			return nil, nil
		}
	}

	a := testAction.PrepareAction(client, action)
	if !a.CanHandle(t.Context(), instance) {
		return nil, nil
	}

	_ = a.Handle(t.Context(), instance)
	return client, a
}
