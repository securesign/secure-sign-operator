package serviceresolver

import (
	"testing"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/serviceresolver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestCtlogResolver(t *testing.T) {
	tlsResolved := metav1.Condition{
		Type:   "ServerTLS",
		Status: metav1.ConditionTrue,
		Reason: "Resolved",
	}

	tests := []struct {
		name    string
		obj     *rhtasv1.CTlog
		want    string
		wantErr bool
	}{
		{
			name: "TLS condition not set returns error",
			obj: &rhtasv1.CTlog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ctlog",
					Namespace: "rhtas",
				},
				Spec: rhtasv1.CTlogSpec{
					Logs: []rhtasv1.CTLogConfig{
						{
							Prefix: "trusted-artifact-signer",
							Active: ptr.To(true),
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "http with prefix",
			obj: &rhtasv1.CTlog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ctlog",
					Namespace: "rhtas",
				},
				Spec: rhtasv1.CTlogSpec{
					Logs: []rhtasv1.CTLogConfig{{Prefix: "spec-prefix", Active: ptr.To(true)}},
				},
				Status: rhtasv1.CTlogStatus{
					Logs:       []rhtasv1.CTlogLogStatus{{Prefix: "trusted-artifact-signer", Active: true}},
					Conditions: []metav1.Condition{tlsResolved},
				},
			},
			want: "http://ctlog.rhtas.svc/trusted-artifact-signer",
		},
		{
			name: "TLS resolved without active status log returns error",
			obj: &rhtasv1.CTlog{
				ObjectMeta: metav1.ObjectMeta{Namespace: "rhtas"},
				Status:     rhtasv1.CTlogStatus{Conditions: []metav1.Condition{tlsResolved}},
			},
			wantErr: true,
		},
		{
			name: "https with TLS and prefix",
			obj: &rhtasv1.CTlog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ctlog",
					Namespace: "test-ns",
				},
				Spec: rhtasv1.CTlogSpec{
					Logs: []rhtasv1.CTLogConfig{{Prefix: "spec-prefix", Active: ptr.To(true)}},
				},
				Status: rhtasv1.CTlogStatus{
					Logs: []rhtasv1.CTlogLogStatus{{Prefix: "logs/sigstore", Active: true}},
					TLS: rhtasv1.TLS{
						CertRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{Name: "ctlog-tls"},
							Key:                  "tls.crt",
						},
					},
					Conditions: []metav1.Condition{tlsResolved},
				},
			},
			want: "https://ctlog.test-ns.svc/logs/sigstore",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := serviceresolver.Resolve(tt.obj)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("URL = %q, want %q", got, tt.want)
			}
		})
	}
}
