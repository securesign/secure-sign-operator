package kubernetes

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const testServiceName = "openshift-apiserver"

// retriableClient embeds a fake client but injects failures for the first
// failTimes List calls, simulating transient API server errors.
type retriableClient struct {
	client.Client
	callCount int
	failTimes int
	failErr   error
}

func (r *retriableClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	r.callCount++
	if r.callCount <= r.failTimes {
		return r.failErr
	}
	return r.Client.List(ctx, list, opts...)
}

func apiService(namespace, name string) apiregistrationv1.APIService {
	return apiregistrationv1.APIService{
		ObjectMeta: metav1.ObjectMeta{Name: namespace + "." + name},
		Spec: apiregistrationv1.APIServiceSpec{
			Service: &apiregistrationv1.ServiceReference{
				Namespace: namespace,
				Name:      name,
			},
		},
	}
}

func TestDetectOpenShiftWithRetry_ServiceDiscovery(t *testing.T) {
	tests := []struct {
		name        string
		services    []apiregistrationv1.APIService
		searchName  string
		expectFound bool
	}{
		{
			name:        "no services",
			searchName:  testServiceName,
			expectFound: false,
		},
		{
			name:        "match by namespace",
			services:    []apiregistrationv1.APIService{apiService(testServiceName, "api")},
			searchName:  testServiceName,
			expectFound: true,
		},
		{
			name:        "match by name",
			services:    []apiregistrationv1.APIService{apiService("default", testServiceName)},
			searchName:  testServiceName,
			expectFound: true,
		},
		{
			name:        "no match",
			services:    []apiregistrationv1.APIService{apiService("some-namespace", "some-service")},
			searchName:  testServiceName,
			expectFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			found, err := detectOpenShiftWithRetry(
				context.Background(), logr.Discard(),
				buildFakeClient(tt.services...),
				tt.searchName, fastBackoff(),
			)

			g.Expect(err).NotTo(gomega.HaveOccurred())
			g.Expect(found).To(gomega.Equal(tt.expectFound))
		})
	}
}

func TestDetectOpenShiftWithRetry_RetriesOnTransientErrors(t *testing.T) {
	transientCases := []struct {
		name string
		err  error
	}{
		{"ServiceUnavailable", apierrors.NewServiceUnavailable("server busy")},
		{"EOF", io.EOF},
		{"NetError", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection reset")}},
		{"ServerTimeout", apierrors.NewServerTimeout(schema.GroupResource{}, "list", 0)},
		{"TooManyRequests", apierrors.NewTooManyRequests("rate limited", 1)},
	}

	for _, tc := range transientCases {
		t.Run(tc.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			inner := buildFakeClient(apiService(testServiceName, "api"))
			rc := &retriableClient{Client: inner, failTimes: 2, failErr: tc.err}

			found, err := detectOpenShiftWithRetry(
				context.Background(), logr.Discard(),
				rc, testServiceName,
				wait.Backoff{Duration: 1 * time.Millisecond, Factor: 1.0, Steps: 10},
			)

			g.Expect(err).NotTo(gomega.HaveOccurred())
			g.Expect(found).To(gomega.BeTrue())
			g.Expect(rc.callCount).To(gomega.Equal(3)) // 2 failures + 1 success
		})
	}
}

func TestDetectOpenShiftWithRetry_NonTransientErrorPropagated(t *testing.T) {
	g := gomega.NewWithT(t)

	unexpectedErr := errors.New("unexpected error")
	rc := &retriableClient{
		Client:    buildFakeClient(),
		failTimes: 999,
		failErr:   unexpectedErr,
	}

	_, err := detectOpenShiftWithRetry(
		context.Background(), logr.Discard(),
		rc, testServiceName, fastBackoff(),
	)

	g.Expect(err).To(gomega.MatchError(unexpectedErr))
}

func TestDetectOpenShiftWithRetry_ContextTimeout(t *testing.T) {
	g := gomega.NewWithT(t)

	rc := &retriableClient{
		Client:    buildFakeClient(),
		failTimes: 999,
		failErr:   io.EOF,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, err := detectOpenShiftWithRetry(
		ctx, logr.Discard(),
		rc, testServiceName,
		wait.Backoff{Duration: 5 * time.Millisecond, Factor: 1.0, Steps: 100},
	)

	g.Expect(err).To(gomega.HaveOccurred())
	g.Expect(err.Error()).To(gomega.ContainSubstring("timed out"))
}

func buildFakeClient(services ...apiregistrationv1.APIService) client.Client {
	scheme := runtime.NewScheme()
	if err := apiregistrationv1.SchemeBuilder.AddToScheme(scheme); err != nil {
		panic(err)
	}
	objs := make([]client.Object, len(services))
	for i := range services {
		objs[i] = &services[i]
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func fastBackoff() wait.Backoff {
	return wait.Backoff{
		Duration: 1 * time.Millisecond,
		Factor:   1.0,
		Steps:    10,
	}
}
