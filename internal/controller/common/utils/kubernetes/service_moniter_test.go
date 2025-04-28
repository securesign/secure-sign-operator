package kubernetes

import (
	"reflect"
	"testing"

	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestCreateServiceMonitor(t *testing.T) {
	type args struct {
		namespace string
		name      string
	}
	tests := []struct {
		name string
		args args
		want *unstructured.Unstructured
	}{
		{
			name: "simple",
			args: args{
				"default",
				"simple",
			},
			want: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "monitoring.coreos.com/v1",
					"kind":       "ServiceMonitor",
					"metadata": map[string]interface{}{
						"name":      "simple",
						"namespace": "default",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CreateServiceMonitor(tt.args.namespace, tt.args.name); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CreateServiceMonitor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnsureServiceMonitorSpec(t *testing.T) {
	type args struct {
		selectorLabels map[string]string
		endpoints      []serviceMonitorEndpoint
	}
	tests := []struct {
		name string
		args args
		want func(gomega.Gomega, *unstructured.Unstructured)
	}{
		{
			name: "empty",
			args: args{},
			want: func(g gomega.Gomega, u *unstructured.Unstructured) {
				endpoints, found, err := unstructured.NestedSlice(u.Object, "spec", "endpoints")
				g.Expect(err).ShouldNot(gomega.HaveOccurred())
				g.Expect(found).Should(gomega.BeTrue())
				g.Expect(endpoints).Should(gomega.BeEmpty())

				selector, found, err := unstructured.NestedStringMap(u.Object, "spec", "selector", "matchLabels")
				g.Expect(err).ShouldNot(gomega.HaveOccurred())
				g.Expect(found).Should(gomega.BeTrue())
				g.Expect(selector).Should(gomega.BeEmpty())
				g.Expect(selector).Should(gomega.BeEmpty())
			},
		},
		{
			name: "single endpoint",
			args: args{
				endpoints: []serviceMonitorEndpoint{
					ServiceMonitorEndpoint("http"),
				},
				selectorLabels: map[string]string{
					"label": "value",
				},
			},
			want: func(g gomega.Gomega, u *unstructured.Unstructured) {
				endpoints, found, err := unstructured.NestedSlice(u.Object, "spec", "endpoints")
				g.Expect(err).ShouldNot(gomega.HaveOccurred())
				g.Expect(found).Should(gomega.BeTrue())
				g.Expect(endpoints).ShouldNot(gomega.BeEmpty())
				g.Expect(endpoints).To(gomega.HaveLen(1))
				g.Expect(endpoints[0]).To(gomega.HaveKeyWithValue("port", "http"))

				selector, found, err := unstructured.NestedStringMap(u.Object, "spec", "selector", "matchLabels")
				g.Expect(err).ShouldNot(gomega.HaveOccurred())
				g.Expect(found).Should(gomega.BeTrue())
				g.Expect(selector).ShouldNot(gomega.BeEmpty())
				g.Expect(selector).Should(gomega.HaveKeyWithValue("label", "value"))
			},
		},
		{
			name: "multiple endpoints",
			args: args{
				endpoints: []serviceMonitorEndpoint{
					ServiceMonitorEndpoint("http"),
					ServiceMonitorEndpoint("https"),
					ServiceMonitorEndpoint("grpc"),
				},
				selectorLabels: map[string]string{
					"label":  "value",
					"label2": "value2",
				},
			},
			want: func(g gomega.Gomega, u *unstructured.Unstructured) {
				endpoints, found, err := unstructured.NestedSlice(u.Object, "spec", "endpoints")
				g.Expect(err).ShouldNot(gomega.HaveOccurred())
				g.Expect(found).Should(gomega.BeTrue())
				g.Expect(endpoints).ShouldNot(gomega.BeEmpty())
				g.Expect(endpoints).To(gomega.HaveLen(3))
				g.Expect(endpoints[0]).To(gomega.HaveKeyWithValue("port", "http"))
				g.Expect(endpoints[1]).To(gomega.HaveKeyWithValue("port", "https"))
				g.Expect(endpoints[2]).To(gomega.HaveKeyWithValue("port", "grpc"))

				selector, found, err := unstructured.NestedStringMap(u.Object, "spec", "selector", "matchLabels")
				g.Expect(err).ShouldNot(gomega.HaveOccurred())
				g.Expect(found).Should(gomega.BeTrue())
				g.Expect(selector).ShouldNot(gomega.BeEmpty())
				g.Expect(selector).Should(gomega.HaveKeyWithValue("label", "value"))
				g.Expect(selector).Should(gomega.HaveKeyWithValue("label2", "value2"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			sm := CreateServiceMonitor("default", "monitor")
			got := EnsureServiceMonitorSpec(tt.args.selectorLabels, tt.args.endpoints...)
			err := got(sm)
			g.Expect(err).ShouldNot(gomega.HaveOccurred())
			tt.want(g, sm)
		})
	}
}
