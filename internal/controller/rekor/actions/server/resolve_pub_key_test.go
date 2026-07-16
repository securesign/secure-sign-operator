package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"reflect"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	httpmock "github.com/securesign/operator/internal/testing/http"
	httputils "github.com/securesign/operator/internal/utils/http"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var testPublicKey = "-----BEGIN PUBLIC KEY-----\nMHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEy5wMSNagtqLsSF+zf8gBVHm2VThGP69D\ngWyhhIm/BkemPBoD/BNq+/yvD2IjsV4unLp5Lcpv4UAGAPJHL/wm+tHD1nS4QKo/\nsXJ8Ezy1K+bM5DUEilcu4hGgQ7+RCG/H\n-----END PUBLIC KEY-----"

func TestResolvePubKey_CanHandle(t *testing.T) {
	tests := []struct {
		name      string
		reason    string
		canHandle bool
		publicKey string
	}{
		{
			name:      "no server condition",
			canHandle: false,
		},
		{
			name:      "Initialize phase",
			reason:    state.Initialize.String(),
			canHandle: true,
		},
		{
			name:      "Ready phase — still handles for rotation detection",
			reason:    state.Ready.String(),
			publicKey: testPublicKey,
			canHandle: true,
		},
		{
			name:      "Creating phase — not yet ready",
			reason:    state.Creating.String(),
			canHandle: false,
		},
		{
			name:      "Pending phase",
			reason:    state.Pending.String(),
			canHandle: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewResolvePubKeyAction()
			instance := rhtasv1.Rekor{
				Status: rhtasv1.RekorStatus{PublicKey: tt.publicKey},
			}
			if tt.reason != "" {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:   actions.ServerCondition,
					Status: metav1.ConditionFalse,
					Reason: tt.reason,
				})
			}
			if got := a.CanHandle(context.TODO(), &instance); got != tt.canHandle {
				t.Errorf("CanHandle() = %v, want %v", got, tt.canHandle)
			}
		})
	}
}

func TestResolvePubKey_Handle(t *testing.T) {
	g := NewWithT(t)
	type want struct {
		result    *action.Result
		publicKey string
	}
	tests := []struct {
		name       string
		publicKey  string
		httpStatus int
		httpBody   string
		want       want
	}{
		{
			name:       "resolve public key from API",
			httpStatus: http.StatusOK,
			httpBody:   testPublicKey,
			want: want{
				result:    testAction.Continue(),
				publicKey: testPublicKey,
			},
		},
		{
			name:       "unchanged public key — no status update",
			publicKey:  testPublicKey,
			httpStatus: http.StatusOK,
			httpBody:   testPublicKey,
			want: want{
				result:    testAction.Continue(),
				publicKey: testPublicKey,
			},
		},
		{
			name:       "key rotation detected — flagged immediately, does not update status without acknowledgement",
			publicKey:  "-----BEGIN PUBLIC KEY-----\nold\n-----END PUBLIC KEY-----",
			httpStatus: http.StatusOK,
			httpBody:   testPublicKey,
			want: want{
				result:    &action.Result{Result: reconcile.Result{}},
				publicKey: "-----BEGIN PUBLIC KEY-----\nold\n-----END PUBLIC KEY-----",
			},
		},
		{
			name:       "API error — requeue",
			httpStatus: http.StatusInternalServerError,
			httpBody:   "error",
			want: want{
				result:    &action.Result{Result: reconcile.Result{RequeueAfter: 5 * time.Second}},
				publicKey: "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()

			httpmock.StubClientBuilder(t, "http://rekor-server.default.svc/api/v1/log/publicKey", tt.httpStatus, tt.httpBody)

			instance := &rhtasv1.Rekor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rekor",
					Namespace: "default",
				},
				Status: rhtasv1.RekorStatus{
					Url:       "http://rekor-server.default.svc",
					PublicKey: tt.publicKey,
					Conditions: []metav1.Condition{
						{
							Type:   actions.ServerCondition,
							Status: metav1.ConditionTrue,
						},
					},
				},
			}

			c := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).Build()

			a := testAction.PrepareAction(c, NewResolvePubKeyAction())
			got := a.Handle(ctx, instance)

			if !reflect.DeepEqual(got, tt.want.result) {
				t.Errorf("Handle() = %v, want %v", got, tt.want.result)
			}
			g.Expect(instance.Status.PublicKey).To(Equal(tt.want.publicKey))
		})
	}
}

func TestResolvePubKey_Handle_TrustedCA(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	const baseURL = "http://rekor-server.default.svc"

	var receivedCAs [][]byte
	orig := httputils.GetClientBuilder()
	httputils.SetClientBuilder(func(cas ...[]byte) *http.Client {
		receivedCAs = cas
		mockClient := &http.Client{}
		httpmock.SetMockTransport(mockClient, map[string]httpmock.RoundTripFunc{
			baseURL + "/api/v1/log/publicKey": func(_ *http.Request) *http.Response {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte(testPublicKey))),
					Header:     make(http.Header),
				}
			},
		})
		return mockClient
	})
	defer func() { httputils.SetClientBuilder(orig) }()

	caConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "custom-ca", Namespace: "default"},
		Data:       map[string]string{"ca-bundle.crt": "-----BEGIN CERTIFICATE-----\nfakeCA\n-----END CERTIFICATE-----"},
	}

	instance := &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{Name: "rekor", Namespace: "default"},
		Spec: rhtasv1.RekorSpec{
			TrustedCA: &rhtasv1.LocalObjectReference{Name: "custom-ca"},
		},
		Status: rhtasv1.RekorStatus{
			Url: baseURL,
			Conditions: []metav1.Condition{
				{Type: actions.ServerCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
			},
		},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance, caConfigMap).
		WithStatusSubresource(instance).Build()

	a := testAction.PrepareAction(c, NewResolvePubKeyAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).To(Equal(testAction.Continue()))
	g.Expect(instance.Status.PublicKey).To(Equal(testPublicKey))
	g.Expect(receivedCAs).To(HaveLen(1))
	g.Expect(string(receivedCAs[0])).To(ContainSubstring("fakeCA"))
}
