package ui

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/migration"
	"github.com/securesign/operator/internal/testing/action"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestMigrationAction_CanHandle(t *testing.T) {
	tests := []struct {
		name     string
		instance *rhtasv1.Rekor
		want     bool
	}{
		{
			name: "no annotation",
			instance: &rhtasv1.Rekor{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			},
			want: false,
		},
		{
			name: "has migration annotation",
			instance: &rhtasv1.Rekor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
					Annotations: map[string]string{
						rhtasv1alpha1.MigrationSearchUIData: `{"enabled":true}`,
					},
				},
			},
			want: true,
		},
		{
			name: "has empty annotation",
			instance: &rhtasv1.Rekor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
					Annotations: map[string]string{
						rhtasv1alpha1.MigrationSearchUIData: "",
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			a := NewMigrationAction()
			g.Expect(a.CanHandle(t.Context(), tt.instance)).To(Equal(tt.want))
		})
	}
}

func TestMigrationAction_Handle(t *testing.T) {
	tests := []struct {
		name            string
		searchUI        *rhtasv1alpha1.RekorSearchUI
		existingConsole *rhtasv1.Console
		wantConsole     bool
		wantAnnotation  bool
	}{
		{
			name: "creates Console CR when enabled",
			searchUI: &rhtasv1alpha1.RekorSearchUI{
				Enabled: ptr.To(true),
				Host:    "rekor-ui.example.com",
				RouteSelectorLabels: map[string]string{
					"app": "rekor-ui",
				},
			},
			wantConsole:    true,
			wantAnnotation: false,
		},
		{
			name: "updates existing Console CR preserving non-UI fields",
			searchUI: &rhtasv1alpha1.RekorSearchUI{
				Enabled: ptr.To(true),
				Host:    "updated-host.example.com",
			},
			existingConsole: &rhtasv1.Console{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-console",
					Namespace: "default",
				},
				Spec: rhtasv1.ConsoleSpec{
					Api: rhtasv1.ConsoleAPI{
						Tuf: rhtasv1.ServiceReference{URL: "https://tuf.example.com"},
					},
					TrustedCA: &rhtasv1.LocalObjectReference{Name: "my-ca"},
				},
			},
			wantConsole:    true,
			wantAnnotation: false,
		},
		{
			name:           "removes annotation when data is empty",
			searchUI:       nil,
			wantConsole:    false,
			wantAnnotation: false,
		},
		{
			name: "skips Console when enabled is false",
			searchUI: &rhtasv1alpha1.RekorSearchUI{
				Enabled: ptr.To(false),
				Host:    "rekor-ui.example.com",
			},
			wantConsole:    false,
			wantAnnotation: false,
		},
		{
			name: "skips Console when enabled is nil",
			searchUI: &rhtasv1alpha1.RekorSearchUI{
				Host: "rekor-ui.example.com",
			},
			wantConsole:    false,
			wantAnnotation: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			instance := &rhtasv1.Rekor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
			}

			if tt.searchUI != nil {
				g.Expect(migration.Set(instance, rhtasv1alpha1.MigrationSearchUIData, tt.searchUI)).To(Succeed())
			} else {
				if instance.Annotations == nil {
					instance.Annotations = map[string]string{}
				}
				instance.Annotations[rhtasv1alpha1.MigrationSearchUIData] = ""
			}

			objects := []client.Object{instance}
			if tt.existingConsole != nil {
				objects = append(objects, tt.existingConsole)
			}

			cli := action.FakeClientBuilder().WithObjects(objects...).WithStatusSubresource(&rhtasv1.Rekor{}).Build()
			a := action.PrepareAction(cli, NewMigrationAction())

			result := a.Handle(t.Context(), instance)
			g.Expect(result).To(Equal(action.Continue()))

			g.Expect(migration.Has(instance, rhtasv1alpha1.MigrationSearchUIData)).To(Equal(tt.wantAnnotation))

			consoleList := &rhtasv1.ConsoleList{}
			g.Expect(cli.List(t.Context(), consoleList, client.InNamespace("default"))).To(Succeed())

			if tt.wantConsole {
				g.Expect(consoleList.Items).To(HaveLen(1))
				console := &consoleList.Items[0]

				if tt.searchUI != nil {
					g.Expect(console.Name).To(Equal("test-console"))
					g.Expect(console.Spec.UI.Ingress.Enabled).To(Equal(tt.searchUI.Enabled))
					g.Expect(console.Spec.UI.Ingress.Host).To(Equal(tt.searchUI.Host))
					g.Expect(console.Spec.UI.Ingress.Labels).To(Equal(tt.searchUI.RouteSelectorLabels))
					g.Expect(console.Spec.UI.Rekor.Ref).ToNot(BeNil())
					g.Expect(console.Spec.UI.Rekor.Ref.Name).To(Equal(instance.Name))
					g.Expect(console.Spec.UI.Rekor.Ref.Namespace).To(Equal(instance.Namespace))
				}
				if tt.existingConsole != nil {
					g.Expect(console.Spec.Api).To(Equal(tt.existingConsole.Spec.Api))
					g.Expect(console.Spec.TrustedCA).To(Equal(tt.existingConsole.Spec.TrustedCA))
				}
			} else {
				g.Expect(consoleList.Items).To(BeEmpty())
			}
		})
	}
}
