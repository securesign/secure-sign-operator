package olm

import (
	"context"
	"slices"
	"testing"

	coreV1 "k8s.io/api/core/v1"
	rbacV1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestOlmV1InstallerTracksChannel(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := coreV1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rbacV1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()
	ctx := context.Background()
	extension, _, err := OlmV1Installer(ctx, cli, "example.com/catalog:base", "test", "rhtas-operator", "stable")
	if err != nil {
		t.Fatal(err)
	}
	obj := extension.Unwrap().(*unstructured.Unstructured)
	if err := cli.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		t.Fatal(err)
	}
	channels, _, err := unstructured.NestedStringSlice(obj.Object, "spec", "source", "catalog", "channels")
	if err != nil || !slices.Equal(channels, []string{"stable"}) {
		t.Fatalf("channels = %v, error = %v; want [stable]", channels, err)
	}
	if version, found, err := unstructured.NestedString(obj.Object, "spec", "source", "catalog", "version"); err != nil || found {
		t.Fatalf("channel upgrades must not pin a version: version = %q, error = %v", version, err)
	}
	enforcement, _, err := unstructured.NestedString(obj.Object, "spec", "install", "preflight", "crdUpgradeSafety", "enforcement")
	if err != nil || (enforcement != "" && enforcement != "Strict") {
		t.Fatalf("CRD upgrade safety must remain enabled: enforcement = %q, error = %v", enforcement, err)
	}
	binding := &rbacV1.ClusterRoleBinding{}
	if err := cli.Get(ctx, client.ObjectKey{Name: "rhtas-operator-installer-test"}, binding); err != nil {
		t.Fatal(err)
	}
	if binding.RoleRef.APIGroup != rbacV1.GroupName {
		t.Fatalf("invalid installer role API group: %q", binding.RoleRef.APIGroup)
	}
}
