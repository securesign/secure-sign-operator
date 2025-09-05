package logsigner

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/config"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	testAction "github.com/securesign/operator/internal/testing/action"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTlsAction_CanHandle(t *testing.T) {
	type env struct {
		conditions  []metav1.Condition
		specTLS     rhtasv1alpha1.TLS
		statusTLS   rhtasv1alpha1.TLS
		isOpenShift bool
	}
	type want struct {
		canHandle bool
	}
	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "can handle when SignerCondition has Pending reason",
			env: env{
				conditions: []metav1.Condition{
					{
						Type:   actions.SignerCondition,
						Status: metav1.ConditionFalse,
						Reason: constants.Pending,
					},
				},
			},
			want: want{
				canHandle: true,
			},
		},
		{
			name: "can handle when spec and status TLS differ",
			env: env{
				conditions: []metav1.Condition{
					{
						Type:   actions.SignerCondition,
						Status: metav1.ConditionTrue,
						Reason: constants.Ready,
					},
				},
				specTLS: rhtasv1alpha1.TLS{
					CertRef: &rhtasv1alpha1.SecretKeySelector{
						LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "new-cert"},
						Key:                  "tls.crt",
					},
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{
						LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "new-key"},
						Key:                  "tls.key",
					},
				},
				statusTLS: rhtasv1alpha1.TLS{},
			},
			want: want{
				canHandle: true,
			},
		},

		{
			name: "can handle when spec and status TLS differ - changed",
			env: env{
				conditions: []metav1.Condition{
					{
						Type:   actions.SignerCondition,
						Status: metav1.ConditionTrue,
						Reason: constants.Ready,
					},
				},
				specTLS: rhtasv1alpha1.TLS{
					CertRef: &rhtasv1alpha1.SecretKeySelector{
						LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "new-cert"},
						Key:                  "tls.crt",
					},
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{
						LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "key"},
						Key:                  "tls.key",
					},
				},
				statusTLS: rhtasv1alpha1.TLS{
					CertRef: &rhtasv1alpha1.SecretKeySelector{
						LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "old-cert"},
						Key:                  "tls.crt",
					},
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{
						LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "key"},
						Key:                  "tls.key",
					},
				},
			},
			want: want{
				canHandle: true,
			},
		},
		{
			name: "cannot handle when spec and status TLS are identical",
			env: env{
				conditions: []metav1.Condition{
					{
						Type:   actions.SignerCondition,
						Status: metav1.ConditionTrue,
						Reason: constants.Ready,
					},
				},
				specTLS: rhtasv1alpha1.TLS{
					CertRef: &rhtasv1alpha1.SecretKeySelector{
						LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "same-cert"},
						Key:                  "tls.crt",
					},
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{
						LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "key"},
						Key:                  "tls.key",
					},
				},
				statusTLS: rhtasv1alpha1.TLS{
					CertRef: &rhtasv1alpha1.SecretKeySelector{
						LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "same-cert"},
						Key:                  "tls.crt",
					},
					PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{
						LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "key"},
						Key:                  "tls.key",
					},
				},
				isOpenShift: false,
			},
			want: want{
				canHandle: false,
			},
		},
		{
			name: "can handle on OpenShift when Ready and no cert ref in status (enable TLS by default)",
			env: env{
				conditions: []metav1.Condition{
					{
						Type:   actions.SignerCondition,
						Status: metav1.ConditionTrue,
						Reason: constants.Ready,
					},
				},
				specTLS:     rhtasv1alpha1.TLS{},
				statusTLS:   rhtasv1alpha1.TLS{},
				isOpenShift: true,
			},
			want: want{
				canHandle: true,
			},
		},
		{
			name: "cannot handle on OpenShift when Ready and cert ref exists in status",
			env: env{
				conditions: []metav1.Condition{
					{
						Type:   actions.SignerCondition,
						Status: metav1.ConditionTrue,
						Reason: constants.Ready,
					},
				},
				specTLS: rhtasv1alpha1.TLS{},
				statusTLS: rhtasv1alpha1.TLS{
					CertRef: &rhtasv1alpha1.SecretKeySelector{
						LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "existing-cert"},
						Key:                  "tls.crt",
					},
				},
				isOpenShift: true,
			},
			want: want{
				canHandle: false,
			},
		},
		{
			name: "cannot handle on non-OpenShift when Ready - no TLS config",
			env: env{
				conditions: []metav1.Condition{
					{
						Type:   actions.SignerCondition,
						Status: metav1.ConditionTrue,
						Reason: constants.Ready,
					},
				},
				specTLS:     rhtasv1alpha1.TLS{},
				statusTLS:   rhtasv1alpha1.TLS{},
				isOpenShift: false,
			},
			want: want{
				canHandle: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			g := NewWithT(t)
			instance := &rhtasv1alpha1.Trillian{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-trillian",
					Namespace: "default",
				},
				Spec: rhtasv1alpha1.TrillianSpec{
					LogSigner: rhtasv1alpha1.TrillianLogSigner{
						TLS: tt.env.specTLS,
					},
				},
				Status: rhtasv1alpha1.TrillianStatus{
					LogSigner: rhtasv1alpha1.TrillianLogSigner{
						TLS: tt.env.statusTLS,
					},
					Conditions: tt.env.conditions,
				},
			}

			c := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).
				Build()

			action := testAction.PrepareAction(c, NewTlsAction())
			config.Openshift = tt.env.isOpenShift

			result := action.CanHandle(ctx, instance)

			g.Expect(result).To(Equal(tt.want.canHandle))

		})
	}
}
