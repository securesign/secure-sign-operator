package utils

import (
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/annotations"
	corev1 "k8s.io/api/core/v1"
	"reflect"
	"testing"
)

func TestSetTrustedCA(t *testing.T) {
	g := NewWithT(t)
	deployment := func() *corev1.PodTemplateSpec {
		return &corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "empty",
					},
					{
						Name: "env",
						Env: []corev1.EnvVar{
							{
								Name:  "NAME",
								Value: "VALUE",
							},
						},
					},
					{
						Name: "volume",
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "mount",
								MountPath: "/mount/path/",
							},
						},
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: "mount",
					},
				},
			},
		}
	}

	type asserts func(*corev1.PodTemplateSpec, error)
	type args struct {
		dep *corev1.PodTemplateSpec
		lor *v1alpha1.LocalObjectReference
	}
	tests := []struct {
		name  string
		args  args
		error bool
		want  asserts
	}{
		{
			name: "nil LocalObjectReference",
			args: args{
				dep: deployment(),
				lor: nil,
			},
			want: func(spec *corev1.PodTemplateSpec, _ error) {

				g.Expect(spec.Spec.Containers).ShouldNot(BeNil())
				g.Expect(spec.Spec.Containers).Should(HaveLen(3))
				g.Expect(spec.Spec.Containers[0].Name).Should(BeEquivalentTo("empty"))
				g.Expect(spec.Spec.Containers[0].Env).Should(HaveLen(1))
				g.Expect(spec.Spec.Containers[0].Env).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Name": Equal("SSL_CERT_DIR"),
				})))
				g.Expect(spec.Spec.Containers[0].VolumeMounts).Should(HaveLen(1))
				g.Expect(spec.Spec.Containers[0].VolumeMounts).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Name": Equal("ca-trust"),
				})))

				g.Expect(spec.Spec.Containers[1].Name).Should(BeEquivalentTo("env"))
				g.Expect(spec.Spec.Containers[1].Env).Should(HaveLen(2))
				g.Expect(spec.Spec.Containers[1].Env).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Name": Equal("NAME"),
				})))
				g.Expect(spec.Spec.Containers[1].Env).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Name": Equal("SSL_CERT_DIR"),
				})))
				g.Expect(spec.Spec.Containers[1].VolumeMounts).Should(HaveLen(1))
				g.Expect(spec.Spec.Containers[1].VolumeMounts).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Name": Equal("ca-trust"),
				})))

				g.Expect(spec.Spec.Containers[2].Name).Should(BeEquivalentTo("volume"))
				g.Expect(spec.Spec.Containers[2].Env).Should(HaveLen(1))
				g.Expect(spec.Spec.Containers[2].Env).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Name": Equal("SSL_CERT_DIR"),
				})))
				g.Expect(spec.Spec.Containers[2].VolumeMounts).Should(HaveLen(2))
				g.Expect(spec.Spec.Containers[2].VolumeMounts).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Name": Equal("mount"),
				})))
				g.Expect(spec.Spec.Containers[2].VolumeMounts).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Name": Equal("ca-trust"),
				})))

				g.Expect(spec.Spec.Volumes).Should(HaveLen(2))
				g.Expect(spec.Spec.Volumes).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Name": Equal("mount"),
				})))
				g.Expect(spec.Spec.Volumes).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Name": Equal("ca-trust"),
				})))
				g.Expect(spec.Spec.Volumes[1].VolumeSource.Projected.Sources).Should(BeEmpty())
			},
		},
		{
			name: "mount config map",
			args: args{
				dep: deployment(),
				lor: &v1alpha1.LocalObjectReference{Name: "trusted"},
			},
			want: func(spec *corev1.PodTemplateSpec, _ error) {

				g.Expect(spec.Spec.Containers).ShouldNot(BeNil())
				g.Expect(spec.Spec.Containers).Should(HaveLen(3))
				g.Expect(spec.Spec.Containers[0].Name).Should(BeEquivalentTo("empty"))
				g.Expect(spec.Spec.Containers[0].Env).Should(HaveLen(1))
				g.Expect(spec.Spec.Containers[0].Env).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Name": Equal("SSL_CERT_DIR"),
				})))
				g.Expect(spec.Spec.Containers[0].VolumeMounts).Should(HaveLen(1))
				g.Expect(spec.Spec.Containers[0].VolumeMounts).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Name": Equal("ca-trust"),
				})))

				g.Expect(spec.Spec.Containers[1].Name).Should(BeEquivalentTo("env"))
				g.Expect(spec.Spec.Containers[1].Env).Should(HaveLen(2))
				g.Expect(spec.Spec.Containers[1].Env).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Name": Equal("NAME"),
				})))
				g.Expect(spec.Spec.Containers[1].Env).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Name": Equal("SSL_CERT_DIR"),
				})))
				g.Expect(spec.Spec.Containers[1].VolumeMounts).Should(HaveLen(1))
				g.Expect(spec.Spec.Containers[1].VolumeMounts).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Name": Equal("ca-trust"),
				})))

				g.Expect(spec.Spec.Containers[2].Name).Should(BeEquivalentTo("volume"))
				g.Expect(spec.Spec.Containers[2].Env).Should(HaveLen(1))
				g.Expect(spec.Spec.Containers[2].Env).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Name": Equal("SSL_CERT_DIR"),
				})))
				g.Expect(spec.Spec.Containers[2].VolumeMounts).Should(HaveLen(2))
				g.Expect(spec.Spec.Containers[2].VolumeMounts).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Name": Equal("mount"),
				})))
				g.Expect(spec.Spec.Containers[2].VolumeMounts).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Name": Equal("ca-trust"),
				})))

				g.Expect(spec.Spec.Volumes).Should(HaveLen(2))
				g.Expect(spec.Spec.Volumes).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Name": Equal("mount"),
				})))
				g.Expect(spec.Spec.Volumes).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Name": Equal("ca-trust"),
				})))
				g.Expect(spec.Spec.Volumes[1].VolumeSource.Projected.Sources).Should(HaveLen(1))
				g.Expect(spec.Spec.Volumes[1].VolumeSource.Projected.Sources[0].ConfigMap.LocalObjectReference.Name).Should(Equal("trusted"))
			},
		},
		{
			name: "nil Deployment",
			args: args{
				dep: nil,
				lor: nil,
			},
			error: true,
			want: func(d *corev1.PodTemplateSpec, err error) {
				g.Expect(d).Should(BeNil())
				g.Expect(err).Should(MatchError(ContainSubstring("PodTemplateSpec is not set")))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SetTrustedCA(tt.args.dep, tt.args.lor)
			if tt.error {
				g.Expect(err).Should(HaveOccurred())
			} else {
				g.Expect(err).ShouldNot(HaveOccurred())
			}
			tt.want(tt.args.dep, err)
		})
	}
}

func TestTrustedCAAnnotationToReference(t *testing.T) {
	type args struct {
		anns map[string]string
	}
	tests := []struct {
		name string
		args args
		want *v1alpha1.LocalObjectReference
	}{
		{
			name: "nil",
			args: args{
				anns: nil,
			},
			want: nil,
		},
		{
			name: "empty",
			args: args{
				anns: make(map[string]string, 0),
			},
			want: nil,
		},
		{
			name: "not existing",
			args: args{
				anns: map[string]string{
					"annotation": "value",
				},
			},
			want: nil,
		},
		{
			name: "existing",
			args: args{map[string]string{
				"annotation":          "value",
				annotations.TrustedCA: "trusted",
			}},
			want: &v1alpha1.LocalObjectReference{Name: "trusted"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TrustedCAAnnotationToReference(tt.args.anns); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("TrustedCAAnnotationToReference() = %v, want %v", got, tt.want)
			}
		})
	}
}
