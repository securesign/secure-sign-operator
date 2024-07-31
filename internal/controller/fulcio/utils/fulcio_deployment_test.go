package utils

import (
	"github.com/securesign/operator/internal/controller/annotations"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/constants"
	v12 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	componentName  = "component"
	deploymentName = "instance"

	rbacName = "fulcio"
)

func TestSimpleDeploymen(t *testing.T) {
	g := NewWithT(t)

	instance := createInstance()
	labels := constants.LabelsFor(componentName, deploymentName, instance.Name)
	deployment, err := CreateDeployment(instance, deploymentName, rbacName, labels)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(deployment).ShouldNot(BeNil())
	g.Expect(deployment.Labels).Should(Equal(labels))
	g.Expect(deployment.Name).Should(Equal(deploymentName))
	g.Expect(deployment.Spec.Template.Spec.ServiceAccountName).Should(Equal(rbacName))

	// private key password
	g.Expect(deployment.Spec.Template.Spec.Containers[0].Env).ShouldNot(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Name": Equal("PASSWORD"),
	})), "PASSWORD env should not be set")

	// oidc-info volume
	oidcVolume := findVolume("ca-trust", deployment.Spec.Template.Spec.Volumes)
	g.Expect(oidcVolume).ShouldNot(BeNil())
	g.Expect(oidcVolume.VolumeSource.Projected.Sources).Should(BeEmpty())
}

func TestPrivateKeyPassword(t *testing.T) {
	g := NewWithT(t)

	instance := createInstance()
	instance.Status.Certificate.PrivateKeyPasswordRef = &v1alpha1.SecretKeySelector{
		LocalObjectReference: v1alpha1.LocalObjectReference{
			Name: "secret",
		},
		Key: "key",
	}
	labels := constants.LabelsFor(componentName, deploymentName, instance.Name)
	deployment, err := CreateDeployment(instance, deploymentName, rbacName, labels)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(deployment).ShouldNot(BeNil())

	g.Expect(deployment.Spec.Template.Spec.Containers[0].Env).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Name": Equal("PASSWORD"),
	})), "PASSWORD env should be set")
	g.Expect(deployment.Spec.Template.Spec.Containers[0].Env).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Name": Equal("SSL_CERT_DIR"),
	})))
}

func TestTrustedCA(t *testing.T) {
	g := NewWithT(t)

	instance := createInstance()
	instance.Spec.TrustedCA = &v1alpha1.LocalObjectReference{Name: "trusted"}
	labels := constants.LabelsFor(componentName, deploymentName, instance.Name)
	deployment, err := CreateDeployment(instance, deploymentName, rbacName, labels)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(deployment).ShouldNot(BeNil())

	g.Expect(deployment.Spec.Template.Spec.Containers[0].Env).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Name": Equal("SSL_CERT_DIR"),
	})))

	oidcVolume := findVolume("ca-trust", deployment.Spec.Template.Spec.Volumes)
	g.Expect(oidcVolume).ShouldNot(BeNil())
	g.Expect(oidcVolume.VolumeSource.Projected.Sources).Should(HaveLen(1))
	g.Expect(oidcVolume.VolumeSource.Projected.Sources[0].ConfigMap.Name).Should(Equal("trusted"))
}

func TestTrustedCAByAnnotation(t *testing.T) {
	g := NewWithT(t)

	instance := createInstance()
	instance.Annotations = make(map[string]string)
	instance.Annotations[annotations.TrustedCA] = "trusted-annotation"
	labels := constants.LabelsFor(componentName, deploymentName, instance.Name)
	deployment, err := CreateDeployment(instance, deploymentName, rbacName, labels)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(deployment).ShouldNot(BeNil())

	g.Expect(deployment.Spec.Template.Spec.Containers[0].Env).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Name": Equal("SSL_CERT_DIR"),
	})))

	oidcVolume := findVolume("ca-trust", deployment.Spec.Template.Spec.Volumes)
	g.Expect(oidcVolume).ShouldNot(BeNil())
	g.Expect(oidcVolume.VolumeSource.Projected.Sources).Should(HaveLen(1))
	g.Expect(oidcVolume.VolumeSource.Projected.Sources[0].ConfigMap.Name).Should(Equal("trusted-annotation"))
}

func TestMissingPrivateKey(t *testing.T) {
	g := NewWithT(t)

	instance := createInstance()
	instance.Status.Certificate.PrivateKeyRef = nil
	labels := constants.LabelsFor(componentName, deploymentName, instance.Name)
	deployment, err := CreateDeployment(instance, deploymentName, rbacName, labels)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(deployment).Should(BeNil())
}

func findVolume(name string, volumes []v12.Volume) *v12.Volume {
	for _, v := range volumes {
		if v.Name == name {
			return &v
		}
	}
	return nil
}

func createInstance() *v1alpha1.Fulcio {
	port := int32(80)
	return &v1alpha1.Fulcio{
		ObjectMeta: v1.ObjectMeta{
			Name:      "name",
			Namespace: "default",
		},
		Spec: v1alpha1.FulcioSpec{
			Ctlog: v1alpha1.CtlogService{
				Address: "http://ctlog.default.svc",
				Port:    &port,
			},
		},
		Status: v1alpha1.FulcioStatus{
			ServerConfigRef: &v1alpha1.LocalObjectReference{Name: "config"},
			Certificate: &v1alpha1.FulcioCert{
				PrivateKeyRef: &v1alpha1.SecretKeySelector{
					Key:                  "private",
					LocalObjectReference: v1alpha1.LocalObjectReference{Name: "secret"},
				},
				CARef: &v1alpha1.SecretKeySelector{
					Key:                  "cert",
					LocalObjectReference: v1alpha1.LocalObjectReference{Name: "secret"},
				},
			},
		},
	}
}
