package common

import (
	"context"
	"testing"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	testAction "github.com/securesign/operator/internal/testing/action"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GenerateTSAInstance() *rhtasv1alpha1.TimestampAuthority {
	return &rhtasv1alpha1.TimestampAuthority{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "timestampAuthority",
			Namespace: "default",
		},
		Status: rhtasv1alpha1.TimestampAuthorityStatus{
			Conditions: []metav1.Condition{
				{
					Type:   constants.Ready,
					Reason: constants.Ready,
				},
			},
			NTPMonitoring: nil,
			Signer:        nil,
		},
		Spec: rhtasv1alpha1.TimestampAuthoritySpec{
			Signer: rhtasv1alpha1.TimestampAuthoritySigner{
				CertificateChain: rhtasv1alpha1.CertificateChain{
					RootCA: rhtasv1alpha1.TsaCertificateAuthority{
						OrganizationName: "Red Hat",
					},
					IntermediateCA: []rhtasv1alpha1.TsaCertificateAuthority{
						{
							OrganizationName: "Red Hat",
						},
					},
					LeafCA: rhtasv1alpha1.TsaCertificateAuthority{
						OrganizationName: "Red Hat",
					},
				},
			},
			NTPMonitoring: rhtasv1alpha1.NTPMonitoring{
				Enabled: true,
				Config: &rhtasv1alpha1.NtpMonitoringConfig{
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

func TsaTestSetup(instance *rhtasv1alpha1.TimestampAuthority, t *testing.T, client client.WithWatch, action action.Action[*rhtasv1alpha1.TimestampAuthority], initObjs ...client.Object) (client.WithWatch, action.Action[*rhtasv1alpha1.TimestampAuthority]) {
	if client == nil {
		client = testAction.FakeClientBuilder().WithObjects(instance).Build()
	}
	if err := client.Get(context.TODO(), types.NamespacedName{Name: instance.GetName(), Namespace: instance.GetNamespace()}, instance); err != nil {
		t.Error(err)
		return nil, nil
	}

	for _, obj := range initObjs {
		if err := client.Create(context.TODO(), obj); err != nil {
			t.Error(err)
			return nil, nil
		}
	}

	a := testAction.PrepareAction(client, action)
	if !a.CanHandle(context.TODO(), instance) {
		return nil, nil
	}

	_ = a.Handle(context.TODO(), instance)
	return client, a
}
