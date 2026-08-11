package actions

import (
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
	v13 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestSimpleDeployment(t *testing.T) {
	g := NewWithT(t)
	instance := createInstance()
	expectedLabels := labels.For(ComponentName, DeploymentName, instance.Name)

	dp, err := handleDeployment(t, instance)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(dp).ShouldNot(BeNil())
	g.Expect(dp.Labels).Should(Equal(expectedLabels))
	g.Expect(dp.Name).Should(Equal(DeploymentName))
	g.Expect(dp.Spec.Template.Spec.ServiceAccountName).Should(Equal(RBACName))

	// private key password
	g.Expect(dp.Spec.Template.Spec.Containers[0].Env).ShouldNot(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Name": Equal("PASSWORD"),
	})), "PASSWORD env should not be set")

	// oidc-info volume
	oidcVolume := findVolume("ca-trust", dp.Spec.Template.Spec.Volumes)
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
	dp, err := handleDeployment(t, instance)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(dp).ShouldNot(BeNil())

	g.Expect(dp.Spec.Template.Spec.Containers[0].Env).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Name": Equal("PASSWORD"),
	})), "PASSWORD env should be set")
	g.Expect(dp.Spec.Template.Spec.Containers[0].Env).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Name": Equal("SSL_CERT_DIR"),
	})))
}

func TestTrustedCA(t *testing.T) {
	g := NewWithT(t)

	instance := createInstance()
	instance.Spec.TrustedCA = &rhtasv1.LocalObjectReference{Name: "trusted"}
	dp, err := handleDeployment(t, instance)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(dp).ShouldNot(BeNil())

	g.Expect(dp.Spec.Template.Spec.Containers[0].Env).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Name": Equal("SSL_CERT_DIR"),
	})))

	oidcVolume := findVolume("ca-trust", dp.Spec.Template.Spec.Volumes)
	g.Expect(oidcVolume).ShouldNot(BeNil())
	g.Expect(oidcVolume.VolumeSource.Projected.Sources).Should(HaveLen(1))
	g.Expect(oidcVolume.VolumeSource.Projected.Sources[0].ConfigMap.Name).Should(Equal("trusted"))
}

func TestTrustedCAByAnnotation(t *testing.T) {
	g := NewWithT(t)

	instance := createInstance()
	instance.Annotations = make(map[string]string)
	instance.Annotations[annotations.TrustedCA] = "trusted-annotation"
	dp, err := handleDeployment(t, instance)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(dp).ShouldNot(BeNil())

	g.Expect(dp.Spec.Template.Spec.Containers[0].Env).Should(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Name": Equal("SSL_CERT_DIR"),
	})))

	oidcVolume := findVolume("ca-trust", dp.Spec.Template.Spec.Volumes)
	g.Expect(oidcVolume).ShouldNot(BeNil())
	g.Expect(oidcVolume.VolumeSource.Projected.Sources).Should(HaveLen(1))
	g.Expect(oidcVolume.VolumeSource.Projected.Sources[0].ConfigMap.Name).Should(Equal("trusted-annotation"))
}

func TestMissingPrivateKey(t *testing.T) {
	g := NewWithT(t)

	instance := createInstance()
	instance.Status.Certificate.PrivateKeyRef = nil
	dp, err := handleDeployment(t, instance)
	g.Expect(err).Should(HaveOccurred())
	g.Expect(dp).Should(BeNil())
}

func TestFIPSClientSigningAlgorithms(t *testing.T) {
	g := NewWithT(t)

	original := fips.Enabled
	fips.Enabled = func() bool { return true }
	t.Cleanup(func() { fips.Enabled = original })

	instance := createInstance()
	dp, err := handleDeployment(t, instance)

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
	dp, err := handleDeployment(t, instance)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(dp.Spec.Template.Spec.Containers[0].Args).ShouldNot(
		ContainElement(Equal("--client-signing-algorithms")))
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
		Spec: rhtasv1.FulcioSpec{
			Ctlog: rhtasv1.ServiceReference{
				URL: "http://ctlog.default.svc/prefix",
			},
		},
		Status: rhtasv1.FulcioStatus{
			Conditions: []v1.Condition{
				{
					Type:   constants.ReadyCondition,
					Status: v1.ConditionTrue,
					Reason: state.Ready.String(),
				},
			},
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

func handleDeployment(t *testing.T, instance *rhtasv1.Fulcio, objects ...client.Object) (*v13.Deployment, error) {
	t.Helper()
	ctx := t.Context()

	allObjects := append([]client.Object{instance}, objects...)

	c := testAction.FakeClientBuilder().
		WithObjects(allObjects...).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewDeployAction())
	result := a.Handle(ctx, instance)
	if result.Err != nil {
		return nil, result.Err
	}

	dep := &v13.Deployment{}
	if err := c.Get(ctx, client.ObjectKey{
		Name:      DeploymentName,
		Namespace: instance.Namespace,
	}, dep); err != nil {
		return nil, err
	}
	return dep, nil
}

func TestDeployAction_Handle_RefCtlogAddress(t *testing.T) {
	g := NewWithT(t)

	instance := createInstance()
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
			Prefix: "test-prefix",
		},
		Status: rhtasv1.CTlogStatus{
			Conditions: []v1.Condition{
				{
					Type:   ctlogActions.TLSCondition,
					Status: v1.ConditionTrue,
					Reason: "Resolved",
				},
			},
		},
	}

	dp, err := handleDeployment(t, instance, ctlog)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(dp).ShouldNot(BeNil())

	expectedUrl := "http://" + ctlogActions.DeploymentName + ".default.svc/test-prefix"
	g.Expect(dp.Spec.Template.Spec.Containers[0].Args).To(ContainElement(Equal("--ct-log-url=" + expectedUrl)))
}

func TestDeployAction_Handle_AutodiscoveryCtlogAddress(t *testing.T) {
	g := NewWithT(t)

	instance := createInstance()
	instance.Spec.Ctlog = rhtasv1.ServiceReference{}

	ctlog := &rhtasv1.CTlog{
		ObjectMeta: v1.ObjectMeta{
			Name:      "my-ctlog",
			Namespace: "default",
		},
		Spec: rhtasv1.CTlogSpec{
			Prefix: "trusted-artifact-signer",
		},
		Status: rhtasv1.CTlogStatus{
			Conditions: []v1.Condition{
				{
					Type:   ctlogActions.TLSCondition,
					Status: v1.ConditionTrue,
					Reason: "Resolved",
				},
			},
		},
	}

	dp, err := handleDeployment(t, instance, ctlog)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(dp).ShouldNot(BeNil())

	expectedUrl := "http://" + ctlogActions.DeploymentName + ".default.svc/trusted-artifact-signer"
	g.Expect(dp.Spec.Template.Spec.Containers[0].Args).To(ContainElement(Equal("--ct-log-url=" + expectedUrl)))
}
