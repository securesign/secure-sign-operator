package actions

import (
	"maps"
	"slices"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	ctlogActions "github.com/securesign/operator/internal/controller/ctlog/actions"
	_ "github.com/securesign/operator/internal/controller/ctlog/serviceresolver"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	"github.com/securesign/operator/internal/utils/fips"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure/deployment"
	v13 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	componentName  = "component"
	deploymentName = "instance"

	rbacName = "fulcio"
)

func TestSimpleDeploymen(t *testing.T) {
	g := NewWithT(t)
	instance := createInstance()
	labels := labels.For(componentName, DeploymentName, instance.Name)
	deployment, err := createDeployment(instance, labels)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(deployment).ShouldNot(BeNil())
	g.Expect(deployment.Labels).Should(Equal(labels))
	g.Expect(deployment.Name).Should(Equal(DeploymentName))
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
	instance.Status.Certificate.PrivateKeyPasswordRef = &rhtasv1.SecretKeySelector{
		LocalObjectReference: rhtasv1.LocalObjectReference{
			Name: "secret",
		},
		Key: "key",
	}
	labels := labels.For(componentName, deploymentName, instance.Name)
	deployment, err := createDeployment(instance, labels)
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
	instance.Spec.TrustedCA = &rhtasv1.LocalObjectReference{Name: "trusted"}
	labels := labels.For(componentName, deploymentName, instance.Name)
	deployment, err := createDeployment(instance, labels)
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
	labels := labels.For(componentName, deploymentName, instance.Name)
	deployment, err := createDeployment(instance, labels)
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
	labels := labels.For(componentName, deploymentName, instance.Name)
	deployment, err := createDeployment(instance, labels)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(deployment).Should(BeNil())
}

func TestFIPSClientSigningAlgorithms(t *testing.T) {
	g := NewWithT(t)

	original := fips.Enabled
	fips.Enabled = func() bool { return true }
	t.Cleanup(func() { fips.Enabled = original })

	instance := createInstance()
	labels := labels.For(componentName, deploymentName, instance.Name)
	dp, err := createDeployment(instance, labels)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(dp.Spec.Template.Spec.Containers[0].Args).Should(
		ContainElement(Equal("--client-signing-algorithms")))
	g.Expect(dp.Spec.Template.Spec.Containers[0].Args).Should(
		ContainElement(Equal(fips.ClientSigningAlgorithms)))
}

func TestNonFIPSNoClientSigningAlgorithms(t *testing.T) {
	g := NewWithT(t)

	original := fips.Enabled
	fips.Enabled = func() bool { return false }
	t.Cleanup(func() { fips.Enabled = original })

	instance := createInstance()
	labels := labels.For(componentName, deploymentName, instance.Name)
	dp, err := createDeployment(instance, labels)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(dp.Spec.Template.Spec.Containers[0].Args).ShouldNot(
		ContainElement(Equal("--client-signing-algorithms")))
}

func TestCtlogUrlInDeployment(t *testing.T) {
	g := NewWithT(t)
	instance := createInstance()
	dp, err := createDeploymentWithCtlogUrl(instance, map[string]string{}, "http://ctlog.default.svc/prefix")
	g.Expect(err).Should(Succeed())
	g.Expect(dp.Spec.Template.Spec.Containers[0].Args).Should(ContainElement(Equal("--ct-log-url=http://ctlog.default.svc/prefix")))
}

func findVolume(name string, volumes []v12.Volume) *v12.Volume {
	for _, v := range volumes {
		if v.Name == name {
			return &v
		}
	}
	return nil
}

func createInstance() *rhtasv1.Fulcio {
	return &rhtasv1.Fulcio{
		ObjectMeta: v1.ObjectMeta{
			Name:      "name",
			Namespace: "default",
		},
		Status: rhtasv1.FulcioStatus{
			ServerConfigRef: &rhtasv1.LocalObjectReference{Name: "config"},
			Certificate: &rhtasv1.FulcioCertStatus{
				PrivateKeyRef: &rhtasv1.SecretKeySelector{
					Key:                  "private",
					LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"},
				},
				CARef: &rhtasv1.SecretKeySelector{
					Key:                  "cert",
					LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"},
				},
			},
		},
	}
}

func createDeployment(instance *rhtasv1.Fulcio, labels map[string]string) (*v13.Deployment, error) {
	return createDeploymentWithCtlogUrl(instance, labels, "http://ctlog.default.svc/prefix")
}

func createDeploymentWithCtlogUrl(instance *rhtasv1.Fulcio, labels map[string]string, ctlogUrl string) (*v13.Deployment, error) {
	testAction := deployAction{}
	d := &v13.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      DeploymentName,
			Namespace: instance.Namespace,
		},
	}

	ensures := []func(*v13.Deployment) error{
		testAction.ensureDeployment(instance, RBACName, labels, ctlogUrl),
		ensure.Labels[*v13.Deployment](slices.Collect(maps.Keys(labels)), labels),
		deployment.Proxy(),
		deployment.TrustedCA(instance.GetTrustedCA(), "fulcio-server"),
	}
	for _, en := range ensures {
		err := en(d)
		if err != nil {
			return nil, err
		}
	}
	return d, nil
}

func createHandleInstance() *rhtasv1.Fulcio {
	instance := createInstance()
	meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
		Type:   constants.ReadyCondition,
		Status: v1.ConditionFalse,
		Reason: state.Creating.String(),
	})
	return instance
}

func TestDeployAction_Handle_RefCtlogAddress(t *testing.T) {
	ctx := t.Context()
	g := NewWithT(t)

	instance := createHandleInstance()
	instance.Spec.Ctlog = rhtasv1.ServiceReference{
		Ref: &rhtasv1.ServiceReferenceRef{
			Namespace: "default",
			Name:      "test-ctlog",
		},
	}

	ctlog := &rhtasv1.CTlog{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-ctlog",
			Namespace: "default",
		},
		Spec: rhtasv1.CTlogSpec{
			Prefix: "trusted-artifact-signer",
		},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance, ctlog).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewDeployAction())
	result := a.Handle(ctx, instance)
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).ToNot(HaveOccurred())

	dep := &v13.Deployment{}
	g.Expect(c.Get(ctx, client.ObjectKey{
		Name:      DeploymentName,
		Namespace: "default",
	}, dep)).To(Succeed())

	expectedUrl := "http://" + ctlogActions.DeploymentName + ".default.svc/trusted-artifact-signer"
	g.Expect(dep.Spec.Template.Spec.Containers[0].Args).To(ContainElement(Equal("--ct-log-url=" + expectedUrl)))
}

func TestDeployAction_Handle_AutodiscoveryCtlogAddress(t *testing.T) {
	ctx := t.Context()
	g := NewWithT(t)

	instance := createHandleInstance()
	instance.Spec.Ctlog = rhtasv1.ServiceReference{}

	ctlog := &rhtasv1.CTlog{
		ObjectMeta: v1.ObjectMeta{
			Name:      "my-ctlog",
			Namespace: "default",
		},
		Spec: rhtasv1.CTlogSpec{
			Prefix: "trusted-artifact-signer",
		},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance, ctlog).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewDeployAction())
	result := a.Handle(ctx, instance)
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).ToNot(HaveOccurred())

	dep := &v13.Deployment{}
	g.Expect(c.Get(ctx, client.ObjectKey{
		Name:      DeploymentName,
		Namespace: "default",
	}, dep)).To(Succeed())

	expectedUrl := "http://" + ctlogActions.DeploymentName + ".default.svc/trusted-artifact-signer"
	g.Expect(dep.Spec.Template.Spec.Containers[0].Args).To(ContainElement(Equal("--ct-log-url=" + expectedUrl)))
}
