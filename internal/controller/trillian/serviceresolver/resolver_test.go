package serviceresolver

import (
	"testing"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/serviceresolver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTrillianResolver(t *testing.T) {
	obj := &rhtasv1.Trillian{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "trillian",
			Namespace: "rhtas",
		},
	}

	u, err := serviceresolver.Resolve(obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "dns:///trillian-logserver.rhtas.svc:8091"
	if got := u; got != want {
		t.Errorf("URL = %q, want %q", got, want)
	}

}
