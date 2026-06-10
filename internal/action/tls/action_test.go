package tls

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/config"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const testCondition = "TestCondition"
const testSecretFormat = "%s-test-tls"
const testComponent = "test component"

func testWrapper() func(*rhtasv1.Trillian) *wrapper[*rhtasv1.Trillian] {
	return Wrapper(
		func(t *rhtasv1.Trillian) rhtasv1.TLS { return t.Spec.LogServer.TLS },
		func(t *rhtasv1.Trillian) rhtasv1.TLS { return t.Status.LogServer.TLS },
		func(t *rhtasv1.Trillian, tls rhtasv1.TLS) { t.Status.LogServer.TLS = tls },
		nil,
	)
}

func TestCanHandle(t *testing.T) {
	type env struct {
		conditions  []metav1.Condition
		specTLS     rhtasv1.TLS
		statusTLS   rhtasv1.TLS
		isOpenShift bool
		isEnabled   func(*rhtasv1.Trillian) bool
	}
	tests := []struct {
		name      string
		env       env
		canHandle bool
	}{
		{
			name: "no condition — cannot handle",
			env: env{
				conditions: []metav1.Condition{},
			},
			canHandle: false,
		},
		{
			name: "condition Pending — can handle",
			env: env{
				conditions: []metav1.Condition{
					{
						Type:   testCondition,
						Status: metav1.ConditionFalse,
						Reason: state.Pending.String(),
					},
				},
			},
			canHandle: true,
		},
		{
			name: "spec and status TLS differ — can handle",
			env: env{
				conditions: []metav1.Condition{
					{
						Type:   testCondition,
						Status: metav1.ConditionTrue,
						Reason: state.Ready.String(),
					},
				},
				specTLS: rhtasv1.TLS{
					CertRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "new-cert"},
						Key:                  "tls.crt",
					},
				},
				statusTLS: rhtasv1.TLS{},
			},
			canHandle: true,
		},
		{
			name: "spec and status TLS identical — cannot handle",
			env: env{
				conditions: []metav1.Condition{
					{
						Type:   testCondition,
						Status: metav1.ConditionTrue,
						Reason: state.Ready.String(),
					},
				},
				specTLS: rhtasv1.TLS{
					CertRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "same-cert"},
						Key:                  "tls.crt",
					},
				},
				statusTLS: rhtasv1.TLS{
					CertRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "same-cert"},
						Key:                  "tls.crt",
					},
				},
			},
			canHandle: false,
		},
		{
			name: "OpenShift, Ready, no cert in status — can handle (auto TLS)",
			env: env{
				conditions: []metav1.Condition{
					{
						Type:   testCondition,
						Status: metav1.ConditionTrue,
						Reason: state.Ready.String(),
					},
				},
				isOpenShift: true,
			},
			canHandle: true,
		},
		{
			name: "OpenShift, Ready, cert exists in status — cannot handle",
			env: env{
				conditions: []metav1.Condition{
					{
						Type:   testCondition,
						Status: metav1.ConditionTrue,
						Reason: state.Ready.String(),
					},
				},
				statusTLS: rhtasv1.TLS{
					CertRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "existing-cert"},
						Key:                  "tls.crt",
					},
				},
				isOpenShift: true,
			},
			canHandle: false,
		},
		{
			name: "non-OpenShift, Ready, no cert — cannot handle",
			env: env{
				conditions: []metav1.Condition{
					{
						Type:   testCondition,
						Status: metav1.ConditionTrue,
						Reason: state.Ready.String(),
					},
				},
				isOpenShift: false,
			},
			canHandle: false,
		},
		{
			name: "OpenShift, Creating, no cert in status — can handle (auto TLS regardless of reason)",
			env: env{
				conditions: []metav1.Condition{
					{
						Type:   testCondition,
						Status: metav1.ConditionFalse,
						Reason: state.Creating.String(),
					},
				},
				isOpenShift: true,
			},
			canHandle: true,
		},
		{
			name: "disabled — cannot handle",
			env: env{
				conditions: []metav1.Condition{
					{
						Type:   testCondition,
						Status: metav1.ConditionFalse,
						Reason: state.Pending.String(),
					},
				},
				isEnabled: func(_ *rhtasv1.Trillian) bool { return false },
			},
			canHandle: false,
		},
		{
			name: "nil isEnabled defaults to enabled — can handle",
			env: env{
				conditions: []metav1.Condition{
					{
						Type:   testCondition,
						Status: metav1.ConditionFalse,
						Reason: state.Pending.String(),
					},
				},
			},
			canHandle: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			instance := &rhtasv1.Trillian{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-instance",
					Namespace: "default",
				},
				Spec: rhtasv1.TrillianSpec{
					LogServer: rhtasv1.TrillianLogServer{
						TLS: tt.env.specTLS,
					},
				},
				Status: rhtasv1.TrillianStatus{
					LogServer: rhtasv1.TrillianLogServer{
						TLS: tt.env.statusTLS,
					},
					Conditions: tt.env.conditions,
				},
			}

			w := Wrapper(
				func(t *rhtasv1.Trillian) rhtasv1.TLS { return t.Spec.LogServer.TLS },
				func(t *rhtasv1.Trillian) rhtasv1.TLS { return t.Status.LogServer.TLS },
				func(t *rhtasv1.Trillian, tls rhtasv1.TLS) { t.Status.LogServer.TLS = tls },
				tt.env.isEnabled,
			)

			c := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).
				Build()

			a := testAction.PrepareAction(c, NewAction(
				testCondition, metav1.ConditionFalse, testSecretFormat, testComponent, w,
			))
			config.Openshift = tt.env.isOpenShift

			g.Expect(a.CanHandle(t.Context(), instance)).To(Equal(tt.canHandle))
		})
	}
}

func TestHandle(t *testing.T) {
	tests := []struct {
		name              string
		specTLS           rhtasv1.TLS
		isOpenShift       bool
		conditionStatus   metav1.ConditionStatus
		expectedStatusTLS rhtasv1.TLS
	}{
		{
			name: "user cert specified — copies spec to status",
			specTLS: rhtasv1.TLS{
				CertRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-cert"},
					Key:                  "tls.crt",
				},
				PrivateKeyRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-key"},
					Key:                  "tls.key",
				},
			},
			conditionStatus: metav1.ConditionFalse,
			expectedStatusTLS: rhtasv1.TLS{
				CertRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-cert"},
					Key:                  "tls.crt",
				},
				PrivateKeyRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-key"},
					Key:                  "tls.key",
				},
			},
		},
		{
			name:            "OpenShift, no user cert — auto-provisions",
			isOpenShift:     true,
			conditionStatus: metav1.ConditionFalse,
			expectedStatusTLS: rhtasv1.TLS{
				CertRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{Name: "test-instance-test-tls"},
					Key:                  "tls.crt",
				},
				PrivateKeyRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{Name: "test-instance-test-tls"},
					Key:                  "tls.key",
				},
			},
		},
		{
			name:              "vanilla K8s, no user cert — insecure",
			isOpenShift:       false,
			conditionStatus:   metav1.ConditionFalse,
			expectedStatusTLS: rhtasv1.TLS{},
		},
		{
			name:            "condition status matches configured resolved status (ConditionTrue)",
			conditionStatus: metav1.ConditionTrue,
			specTLS: rhtasv1.TLS{
				CertRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{Name: "cert"},
					Key:                  "tls.crt",
				},
			},
			expectedStatusTLS: rhtasv1.TLS{
				CertRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{Name: "cert"},
					Key:                  "tls.crt",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			instance := &rhtasv1.Trillian{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-instance",
					Namespace: "default",
				},
				Spec: rhtasv1.TrillianSpec{
					LogServer: rhtasv1.TrillianLogServer{
						TLS: tt.specTLS,
					},
				},
			}

			c := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).
				Build()

			a := testAction.PrepareAction(c, NewAction(
				testCondition, tt.conditionStatus, testSecretFormat, testComponent, testWrapper(),
			))
			config.Openshift = tt.isOpenShift

			result := a.Handle(t.Context(), instance)

			g.Expect(result).To(Equal(testAction.Return()))
			g.Expect(instance.Status.LogServer.TLS).To(Equal(tt.expectedStatusTLS))

			con := meta.FindStatusCondition(instance.GetConditions(), testCondition)
			g.Expect(con).To(gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Type":   Equal(testCondition),
				"Status": Equal(tt.conditionStatus),
				"Reason": Equal(ReasonResolved),
			})))
		})
	}
}
