/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func tlsTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := configv1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add configv1 to scheme: %v", err)
	}
	return s
}

func apiServerWith(profile *configv1.TLSSecurityProfile, adherence configv1.TLSAdherencePolicy) *configv1.APIServer {
	return &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: profile,
			TLSAdherence:       adherence,
		},
	}
}

// resolveClusterTLSProfile: non-OpenShift and disabled paths return Intermediate defaults
// without touching the client.
func TestResolveClusterTLSProfile_Defaults(t *testing.T) {
	t.Parallel()
	g := gomega.NewWithT(t)
	intermediate := *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]

	tests := []struct {
		name      string
		openshift bool
		disabled  bool
	}{
		{"vanilla kubernetes", false, false},
		{"openshift with resolution disabled", true, true},
		{"non-openshift with disable flag set", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A nil client must never be dereferenced on these paths.
			profile, adherence, err := resolveClusterTLSProfile(
				context.Background(), nil, tt.openshift, tt.disabled, logr.Discard())

			g.Expect(err).ToNot(gomega.HaveOccurred())
			g.Expect(profile.MinTLSVersion).To(gomega.Equal(intermediate.MinTLSVersion))
			g.Expect(adherence).To(gomega.Equal(configv1.TLSAdherencePolicyNoOpinion))
		})
	}
}

// resolveClusterTLSProfile: on OpenShift the configured cluster profile and adherence policy
// are returned.
func TestResolveClusterTLSProfile_FetchesConfiguredProfile(t *testing.T) {
	t.Parallel()
	g := gomega.NewWithT(t)
	modern := *configv1.TLSProfiles[configv1.TLSProfileModernType]

	cli := fake.NewClientBuilder().
		WithScheme(tlsTestScheme(t)).
		WithObjects(apiServerWith(
			&configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType},
			configv1.TLSAdherencePolicyStrictAllComponents,
		)).
		Build()

	profile, adherence, err := resolveClusterTLSProfile(
		context.Background(), cli, true, false, logr.Discard())

	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(profile.MinTLSVersion).To(gomega.Equal(modern.MinTLSVersion))
	g.Expect(adherence).To(gomega.Equal(configv1.TLSAdherencePolicyStrictAllComponents))
}

// resolveClusterTLSProfile: when the APIServer resource is absent the resolver falls back to
// Intermediate defaults and NoOpinion adherence without erroring.
func TestResolveClusterTLSProfile_APIServerNotFound(t *testing.T) {
	t.Parallel()
	g := gomega.NewWithT(t)
	intermediate := *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]

	cli := fake.NewClientBuilder().WithScheme(tlsTestScheme(t)).Build()

	profile, adherence, err := resolveClusterTLSProfile(
		context.Background(), cli, true, false, logr.Discard())

	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(profile.MinTLSVersion).To(gomega.Equal(intermediate.MinTLSVersion))
	g.Expect(adherence).To(gomega.Equal(configv1.TLSAdherencePolicyNoOpinion))
}

// resolveClusterTLSProfile: an unexpected error fetching the profile aborts startup.
func TestResolveClusterTLSProfile_ProfileFetchError(t *testing.T) {
	t.Parallel()
	g := gomega.NewWithT(t)

	boom := errors.New("connection refused")
	cli := fake.NewClientBuilder().
		WithScheme(tlsTestScheme(t)).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return boom
			},
		}).
		Build()

	_, _, err := resolveClusterTLSProfile(
		context.Background(), cli, true, false, logr.Discard())

	g.Expect(err).To(gomega.HaveOccurred())
	g.Expect(err.Error()).To(gomega.ContainSubstring("unable to fetch cluster TLS security profile"))
}

// resolveClusterTLSProfile: an unexpected error fetching only the adherence policy is tolerated;
// the resolver keeps the profile and defaults adherence to NoOpinion.
func TestResolveClusterTLSProfile_AdherenceFetchError(t *testing.T) {
	t.Parallel()
	g := gomega.NewWithT(t)
	modern := *configv1.TLSProfiles[configv1.TLSProfileModernType]

	apiServer := apiServerWith(
		&configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType},
		configv1.TLSAdherencePolicyStrictAllComponents,
	)

	// Succeed on the first Get (profile) and fail on the second (adherence).
	// NOTE: this assumes controller-runtime-common's FetchAPIServerTLSProfile and
	// FetchAPIServerTLSAdherencePolicy each issue exactly one Get, in that order. That holds for the
	// pinned version; if the dependency changes its internal call count/order, revisit this counter.
	var calls int
	cli := fake.NewClientBuilder().
		WithScheme(tlsTestScheme(t)).
		WithObjects(apiServer).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				calls++
				if calls >= 2 {
					return errors.New("connection refused")
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}).
		Build()

	profile, adherence, err := resolveClusterTLSProfile(
		context.Background(), cli, true, false, logr.Discard())

	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(profile.MinTLSVersion).To(gomega.Equal(modern.MinTLSVersion))
	g.Expect(adherence).To(gomega.Equal(configv1.TLSAdherencePolicyNoOpinion))
}

// Guard: the NotFound classification the resolver relies on must hold for the fake client so the
// fallback path stays correct if dependencies change.
func TestResolveClusterTLSProfile_NotFoundClassification(t *testing.T) {
	t.Parallel()
	g := gomega.NewWithT(t)

	cli := fake.NewClientBuilder().WithScheme(tlsTestScheme(t)).Build()
	err := cli.Get(context.Background(), client.ObjectKey{Name: "cluster"}, &configv1.APIServer{})

	g.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())
}
