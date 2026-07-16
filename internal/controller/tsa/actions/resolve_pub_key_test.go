package actions

import (
	"context"
	_ "embed"
	"net/http"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	httpmock "github.com/securesign/operator/internal/testing/http"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

//go:embed testdata/cert_chain.pem
var testCertChainPEMRaw string

// testCertChainPEM has no trailing newline, matching the shape of a manually
// concatenated PEM chain as served by a real TSA endpoint.
var testCertChainPEM = strings.TrimSpace(testCertChainPEMRaw)

func TestTSAResolvePubKey_CanHandle(t *testing.T) {
	a := NewResolvePubKeyAction()
	t.Run("not ready", func(t *testing.T) {
		instance := &rhtasv1.TimestampAuthority{}
		if a.CanHandle(context.TODO(), instance) {
			t.Error("expected false when no condition set")
		}
	})
	t.Run("initialize phase", func(t *testing.T) {
		instance := &rhtasv1.TimestampAuthority{}
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Initialize.String(),
		})
		if !a.CanHandle(context.TODO(), instance) {
			t.Error("expected true in Initialize phase")
		}
	})
}

func TestTSAResolvePubKey_Handle(t *testing.T) {
	g := NewWithT(t)
	type want struct {
		result           *action.Result
		certificateChain string
	}
	tests := []struct {
		name             string
		certificateChain string
		httpStatus       int
		httpBody         string
		want             want
	}{
		{
			name:       "resolve cert chain from API",
			httpStatus: http.StatusOK,
			httpBody:   testCertChainPEM,
			want: want{
				result:           testAction.Continue(),
				certificateChain: testCertChainPEM,
			},
		},
		{
			name:             "unchanged — no status update",
			certificateChain: testCertChainPEM,
			httpStatus:       http.StatusOK,
			httpBody:         testCertChainPEM,
			want: want{
				result:           testAction.Continue(),
				certificateChain: testCertChainPEM,
			},
		},
		{
			name:       "API error — requeue",
			httpStatus: http.StatusInternalServerError,
			httpBody:   "error",
			want: want{
				result:           &action.Result{Result: reconcile.Result{RequeueAfter: 5 * time.Second}},
				certificateChain: "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			const baseURL = "http://tsa-server.default.svc"

			httpmock.StubClientBuilder(t, baseURL+"/api/v1/timestamp/certchain", tt.httpStatus, tt.httpBody)

			instance := &rhtasv1.TimestampAuthority{
				ObjectMeta: metav1.ObjectMeta{Name: "tsa", Namespace: "default"},
				Status: rhtasv1.TimestampAuthorityStatus{
					Url:              baseURL,
					CertificateChain: tt.certificateChain,
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Initialize.String()},
					},
				},
			}

			c := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).Build()

			a := testAction.PrepareAction(c, NewResolvePubKeyAction())
			got := a.Handle(ctx, instance)

			g.Expect(got).To(Equal(tt.want.result))
			g.Expect(instance.Status.CertificateChain).To(Equal(tt.want.certificateChain))
		})
	}
}
