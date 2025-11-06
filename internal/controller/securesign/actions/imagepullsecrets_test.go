package actions

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	testAction "github.com/securesign/operator/internal/testing/action"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SecuresignOption is a functional option for building test Securesign resources.
type SecuresignOption func(*v1alpha1.Securesign)

// toSecretRefs converts a slice of secret names to LocalObjectReference slice.
func toSecretRefs(secrets ...string) []corev1.LocalObjectReference {
	refs := make([]corev1.LocalObjectReference, len(secrets))
	for i, secret := range secrets {
		refs[i] = corev1.LocalObjectReference{Name: secret}
	}
	return refs
}

// withServiceAccountSecrets creates ServiceAccountRequirements with ImagePullSecrets.
func withServiceAccountSecrets(secrets ...string) v1alpha1.ServiceAccountRequirements {
	return v1alpha1.ServiceAccountRequirements{
		ImagePullSecrets: toSecretRefs(secrets...),
	}
}

// newSecuresign creates a new Securesign resource for testing with the given options.
//
//nolint:unparam
func newSecuresign(name, namespace string, opts ...SecuresignOption) *v1alpha1.Securesign {
	s := &v1alpha1.Securesign{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// withParentSecrets adds parent-level ImagePullSecrets to the Securesign resource.
func withParentSecrets(secrets ...string) SecuresignOption {
	return func(s *v1alpha1.Securesign) {
		s.Spec.ServiceAccountRequirements = withServiceAccountSecrets(secrets...)
	}
}

// withCtlogSecrets adds a Ctlog spec with ImagePullSecrets to the Securesign resource.
func withCtlogSecrets(secrets ...string) SecuresignOption {
	return func(s *v1alpha1.Securesign) {
		s.Spec.Ctlog = v1alpha1.CTlogSpec{
			ServiceAccountRequirements: withServiceAccountSecrets(secrets...),
		}
	}
}

// withFulcioSecrets adds a Fulcio spec with ImagePullSecrets to the Securesign resource.
func withFulcioSecrets(secrets ...string) SecuresignOption {
	return func(s *v1alpha1.Securesign) {
		s.Spec.Fulcio = v1alpha1.FulcioSpec{
			ServiceAccountRequirements: withServiceAccountSecrets(secrets...),
		}
	}
}

// withRekorSecrets adds a Rekor spec with ImagePullSecrets to the Securesign resource.
func withRekorSecrets(secrets ...string) SecuresignOption {
	return func(s *v1alpha1.Securesign) {
		s.Spec.Rekor = v1alpha1.RekorSpec{
			ServiceAccountRequirements: withServiceAccountSecrets(secrets...),
		}
	}
}

// withTsaSecrets adds a TimestampAuthority spec with ImagePullSecrets to the Securesign resource.
func withTsaSecrets(secrets ...string) SecuresignOption {
	return func(s *v1alpha1.Securesign) {
		tsa := v1alpha1.TimestampAuthoritySpec{
			ServiceAccountRequirements: withServiceAccountSecrets(secrets...),
		}
		s.Spec.TimestampAuthority = &tsa
	}
}

// withTufSecrets adds a Tuf spec with ImagePullSecrets to the Securesign resource.
func withTufSecrets(secrets ...string) SecuresignOption {
	return func(s *v1alpha1.Securesign) {
		s.Spec.Tuf = v1alpha1.TufSpec{
			ServiceAccountRequirements: withServiceAccountSecrets(secrets...),
		}
	}
}

// testComponent is a generic helper function that tests the propagation
// of ImagePullSecrets from a Securesign parent to a component resource.
func testComponent[T client.Object](
	g Gomega,
	ctx context.Context,
	c client.WithWatch,
	securesign *v1alpha1.Securesign,
	newAction func() action.Action[*v1alpha1.Securesign],
	verify func(Gomega, client.WithWatch, T),
	resource T,
	name, namespace string,
) {
	if verify == nil {
		return
	}

	a := testAction.PrepareAction(c, newAction())
	_ = a.Handle(ctx, securesign)

	g.Expect(c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, resource)).To(Succeed())
	verify(g, c, resource)
}

func TestSecuresignImagePullSecrets(t *testing.T) {
	const (
		namespace = "default"
		name      = "test-securesign"
	)

	tests := []struct {
		name         string
		securesign   *v1alpha1.Securesign
		verifyCtlog  func(Gomega, client.WithWatch, *v1alpha1.CTlog)
		verifyFulcio func(Gomega, client.WithWatch, *v1alpha1.Fulcio)
		verifyRekor  func(Gomega, client.WithWatch, *v1alpha1.Rekor)
		verifyTSA    func(Gomega, client.WithWatch, *v1alpha1.TimestampAuthority)
		verifyTUF    func(Gomega, client.WithWatch, *v1alpha1.Tuf)
	}{
		{
			name:       "propagate ImagePullSecrets to Ctlog",
			securesign: newSecuresign(name, namespace, withParentSecrets("parent-secret"), withCtlogSecrets()),
			verifyCtlog: func(g Gomega, c client.WithWatch, ctlog *v1alpha1.CTlog) {
				g.Expect(ctlog.Spec.ImagePullSecrets).To(HaveLen(1))
				g.Expect(ctlog.Spec.ImagePullSecrets).To(ContainElement(corev1.LocalObjectReference{Name: "parent-secret"}))
			},
		},
		{
			name:       "merge parent and component ImagePullSecrets for Ctlog",
			securesign: newSecuresign(name, namespace, withParentSecrets("parent-secret"), withCtlogSecrets("ctlog-secret")),
			verifyCtlog: func(g Gomega, c client.WithWatch, ctlog *v1alpha1.CTlog) {
				g.Expect(ctlog.Spec.ImagePullSecrets).To(HaveLen(2))
				g.Expect(ctlog.Spec.ImagePullSecrets).To(ContainElement(corev1.LocalObjectReference{Name: "parent-secret"}))
				g.Expect(ctlog.Spec.ImagePullSecrets).To(ContainElement(corev1.LocalObjectReference{Name: "ctlog-secret"}))
			},
		},
		{
			name:       "propagate ImagePullSecrets to Fulcio",
			securesign: newSecuresign(name, namespace, withParentSecrets("parent-secret"), withFulcioSecrets()),
			verifyFulcio: func(g Gomega, c client.WithWatch, fulcio *v1alpha1.Fulcio) {
				g.Expect(fulcio.Spec.ImagePullSecrets).To(HaveLen(1))
				g.Expect(fulcio.Spec.ImagePullSecrets).To(ContainElement(corev1.LocalObjectReference{Name: "parent-secret"}))
			},
		},
		{
			name:       "merge parent and component ImagePullSecrets for Fulcio",
			securesign: newSecuresign(name, namespace, withParentSecrets("parent-secret"), withFulcioSecrets("fulcio-secret")),
			verifyFulcio: func(g Gomega, c client.WithWatch, fulcio *v1alpha1.Fulcio) {
				g.Expect(fulcio.Spec.ImagePullSecrets).To(HaveLen(2))
				g.Expect(fulcio.Spec.ImagePullSecrets).To(ContainElement(corev1.LocalObjectReference{Name: "parent-secret"}))
				g.Expect(fulcio.Spec.ImagePullSecrets).To(ContainElement(corev1.LocalObjectReference{Name: "fulcio-secret"}))
			},
		},
		{
			name:       "propagate ImagePullSecrets to Rekor",
			securesign: newSecuresign(name, namespace, withParentSecrets("parent-secret"), withRekorSecrets()),
			verifyRekor: func(g Gomega, c client.WithWatch, rekor *v1alpha1.Rekor) {
				g.Expect(rekor.Spec.ImagePullSecrets).To(HaveLen(1))
				g.Expect(rekor.Spec.ImagePullSecrets).To(ContainElement(corev1.LocalObjectReference{Name: "parent-secret"}))
			},
		},
		{
			name:       "merge parent and component ImagePullSecrets for Rekor",
			securesign: newSecuresign(name, namespace, withParentSecrets("parent-secret"), withRekorSecrets("rekor-secret")),
			verifyRekor: func(g Gomega, c client.WithWatch, rekor *v1alpha1.Rekor) {
				g.Expect(rekor.Spec.ImagePullSecrets).To(HaveLen(2))
				g.Expect(rekor.Spec.ImagePullSecrets).To(ContainElement(corev1.LocalObjectReference{Name: "parent-secret"}))
				g.Expect(rekor.Spec.ImagePullSecrets).To(ContainElement(corev1.LocalObjectReference{Name: "rekor-secret"}))
			},
		},
		{
			name:       "propagate ImagePullSecrets to TSA",
			securesign: newSecuresign(name, namespace, withParentSecrets("parent-secret"), withTsaSecrets()),
			verifyTSA: func(g Gomega, c client.WithWatch, tsa *v1alpha1.TimestampAuthority) {
				g.Expect(tsa.Spec.ImagePullSecrets).To(HaveLen(1))
				g.Expect(tsa.Spec.ImagePullSecrets).To(ContainElement(corev1.LocalObjectReference{Name: "parent-secret"}))
			},
		},
		{
			name:       "merge parent and component ImagePullSecrets for TSA",
			securesign: newSecuresign(name, namespace, withParentSecrets("parent-secret"), withTsaSecrets("tsa-secret")),
			verifyTSA: func(g Gomega, c client.WithWatch, tsa *v1alpha1.TimestampAuthority) {
				g.Expect(tsa.Spec.ImagePullSecrets).To(HaveLen(2))
				g.Expect(tsa.Spec.ImagePullSecrets).To(ContainElement(corev1.LocalObjectReference{Name: "parent-secret"}))
				g.Expect(tsa.Spec.ImagePullSecrets).To(ContainElement(corev1.LocalObjectReference{Name: "tsa-secret"}))
			},
		},
		{
			name:       "propagate ImagePullSecrets to TUF",
			securesign: newSecuresign(name, namespace, withParentSecrets("parent-secret"), withTufSecrets()),
			verifyTUF: func(g Gomega, c client.WithWatch, tuf *v1alpha1.Tuf) {
				g.Expect(tuf.Spec.ImagePullSecrets).To(HaveLen(1))
				g.Expect(tuf.Spec.ImagePullSecrets).To(ContainElement(corev1.LocalObjectReference{Name: "parent-secret"}))
			},
		},
		{
			name:       "merge parent and component ImagePullSecrets for TUF",
			securesign: newSecuresign(name, namespace, withParentSecrets("parent-secret"), withTufSecrets("tuf-secret")),
			verifyTUF: func(g Gomega, c client.WithWatch, tuf *v1alpha1.Tuf) {
				g.Expect(tuf.Spec.ImagePullSecrets).To(HaveLen(2))
				g.Expect(tuf.Spec.ImagePullSecrets).To(ContainElement(corev1.LocalObjectReference{Name: "parent-secret"}))
				g.Expect(tuf.Spec.ImagePullSecrets).To(ContainElement(corev1.LocalObjectReference{Name: "tuf-secret"}))
			},
		},
		{
			name:       "deduplicate ImagePullSecrets when same secret in parent and component",
			securesign: newSecuresign(name, namespace, withParentSecrets("shared-secret"), withCtlogSecrets("shared-secret")),
			verifyCtlog: func(g Gomega, c client.WithWatch, ctlog *v1alpha1.CTlog) {
				g.Expect(ctlog.Spec.ImagePullSecrets).To(HaveLen(1))
				g.Expect(ctlog.Spec.ImagePullSecrets).To(ContainElement(corev1.LocalObjectReference{Name: "shared-secret"}))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := context.Background()

			c := testAction.FakeClientBuilder().
				WithObjects(tt.securesign).
				WithStatusSubresource(tt.securesign).
				Build()

			testComponent(g, ctx, c, tt.securesign, NewCtlogAction, tt.verifyCtlog, &v1alpha1.CTlog{}, name, namespace)
			testComponent(g, ctx, c, tt.securesign, NewFulcioAction, tt.verifyFulcio, &v1alpha1.Fulcio{}, name, namespace)
			testComponent(g, ctx, c, tt.securesign, NewRekorAction, tt.verifyRekor, &v1alpha1.Rekor{}, name, namespace)
			testComponent(g, ctx, c, tt.securesign, NewTsaAction, tt.verifyTSA, &v1alpha1.TimestampAuthority{}, name, namespace)
			testComponent(g, ctx, c, tt.securesign, NewTufAction, tt.verifyTUF, &v1alpha1.Tuf{}, name, namespace)
		})
	}
}
