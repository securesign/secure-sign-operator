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
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	v13 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
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
	for i := range volumes {
		if volumes[i].Name == name {
			return &volumes[i]
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

// TestUserDefinedVolumesInFileMode verifies that user-specified Volumes and
// VolumeMounts on FulcioSpec are applied to the deployment even in file signer
// mode. This validates that user-defined volumes and mounts are applied regardless of signer type.
func TestUserDefinedVolumesInFileMode(t *testing.T) {
	g := NewWithT(t)

	instance := createInstance()
	// Add user-defined volumes and volume mounts
	instance.Spec.Volumes = []rhtasv1.AdditionalVolume{
		{
			Name: "custom-data",
			AdditionalVolumeSource: rhtasv1.AdditionalVolumeSource{
				PersistentVolumeClaim: &v12.PersistentVolumeClaimVolumeSource{
					ClaimName: "custom-data-pvc",
				},
			},
		},
		{
			Name: "custom-config",
			AdditionalVolumeSource: rhtasv1.AdditionalVolumeSource{
				ConfigMap: &v12.ConfigMapVolumeSource{
					LocalObjectReference: v12.LocalObjectReference{
						Name: "vendor-settings",
					},
				},
			},
		},
	}
	instance.Spec.VolumeMounts = []v12.VolumeMount{
		{
			Name:      "custom-data",
			MountPath: "/var/lib/custom/data",
		},
		{
			Name:      "custom-config",
			MountPath: "/etc/custom",
			ReadOnly:  true,
		},
	}

	dp, err := handleDeployment(t, instance)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(dp).ShouldNot(BeNil())

	// Verify user-defined volumes are present
	customDataVol := findVolume("custom-data", dp.Spec.Template.Spec.Volumes)
	g.Expect(customDataVol).ShouldNot(BeNil(), "custom-data volume should be present")
	g.Expect(customDataVol.PersistentVolumeClaim).ShouldNot(BeNil())
	g.Expect(customDataVol.PersistentVolumeClaim.ClaimName).Should(Equal("custom-data-pvc"))

	customConfigVol := findVolume("custom-config", dp.Spec.Template.Spec.Volumes)
	g.Expect(customConfigVol).ShouldNot(BeNil(), "custom-config volume should be present")
	g.Expect(customConfigVol.ConfigMap).ShouldNot(BeNil())
	g.Expect(customConfigVol.ConfigMap.Name).Should(Equal("vendor-settings"))
	g.Expect(customConfigVol.ConfigMap.Name).Should(Equal("vendor-settings"))

	// Verify user-defined volume mounts are present on the main container
	container := dp.Spec.Template.Spec.Containers[0]
	var customDataMount, customConfigMount *v12.VolumeMount
	for i := range container.VolumeMounts {
		switch container.VolumeMounts[i].Name {
		case "custom-data":
			customDataMount = &container.VolumeMounts[i]
		case "custom-config":
			customConfigMount = &container.VolumeMounts[i]
		}
	}
	g.Expect(customDataMount).ShouldNot(BeNil(), "custom-data mount should be present on main container")
	g.Expect(customDataMount.MountPath).Should(Equal("/var/lib/custom/data"))

	g.Expect(customConfigMount).ShouldNot(BeNil(), "custom-config mount should be present on main container")
	g.Expect(customConfigMount.MountPath).Should(Equal("/etc/custom"))
	g.Expect(customConfigMount.ReadOnly).Should(BeTrue())

	// Verify standard file-mode volumes still exist alongside user-defined ones
	g.Expect(findVolume("fulcio-cert", dp.Spec.Template.Spec.Volumes)).ShouldNot(BeNil(),
		"operator-managed fulcio-cert volume should still be present")
	g.Expect(findVolume("fulcio-config", dp.Spec.Template.Spec.Volumes)).ShouldNot(BeNil(),
		"operator-managed fulcio-config volume should still be present")
}

func findVolumeMount(name string, mounts []v12.VolumeMount) *v12.VolumeMount {
	for i := range mounts {
		if mounts[i].Name == name {
			return &mounts[i]
		}
	}
	return nil
}

// TestUserDefinedInitContainersInFileMode verifies that user-specified init
// containers are applied to the deployment with their volume mounts.
func TestUserDefinedInitContainersInFileMode(t *testing.T) {
	g := NewWithT(t)

	instance := createInstance()
	instance.Spec.InitContainers = []rhtasv1.InitContainerSpec{
		{
			Name:    "setup-init",
			Image:   "vendor-init:latest",
			Command: []string{"/bin/setup"},
			VolumeMounts: []v12.VolumeMount{
				{
					Name:      "custom-data",
					MountPath: "/mnt/data",
				},
			},
		},
	}

	dp, err := handleDeployment(t, instance)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(dp).ShouldNot(BeNil())

	// Verify init container is present
	g.Expect(dp.Spec.Template.Spec.InitContainers).Should(HaveLen(1))
	initContainer := dp.Spec.Template.Spec.InitContainers[0]
	g.Expect(initContainer.Name).Should(Equal("setup-init"))
	g.Expect(initContainer.Image).Should(Equal("vendor-init:latest"))
	g.Expect(initContainer.Command).Should(Equal([]string{"/bin/setup"}))

	// Verify volume mounts on init container
	initMount := findVolumeMount("custom-data", initContainer.VolumeMounts)
	g.Expect(initMount).ShouldNot(BeNil(), "custom-data mount should be present on init container")
	g.Expect(initMount.MountPath).Should(Equal("/mnt/data"))
}

// TestFulcioOperatorVolumePrecedence verifies that operator-managed volumes
// are NOT overwritten by user volumes with the same name.
func TestFulcioOperatorVolumePrecedence(t *testing.T) {
	g := NewWithT(t)

	instance := createInstance()
	// User tries to overwrite an operator-managed volume
	instance.Spec.Volumes = []rhtasv1.AdditionalVolume{
		{
			Name: "fulcio-config",
			AdditionalVolumeSource: rhtasv1.AdditionalVolumeSource{
				EmptyDir: &v12.EmptyDirVolumeSource{},
			},
		},
	}

	dp, err := handleDeployment(t, instance)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(dp).ShouldNot(BeNil())

	configVol := findVolume("fulcio-config", dp.Spec.Template.Spec.Volumes)
	g.Expect(configVol).ShouldNot(BeNil(), "fulcio-config volume should be present")
	g.Expect(configVol.ConfigMap).ShouldNot(BeNil(), "fulcio-config should have ConfigMap source (operator wins)")
	g.Expect(configVol.ConfigMap.Name).Should(Equal("config"))
	g.Expect(configVol.EmptyDir).Should(BeNil(), "fulcio-config should NOT have EmptyDir source")
}

func TestAuthInjectionInFileMode(t *testing.T) {
	g := NewWithT(t)

	instance := createInstance()
	instance.Spec.Auth = &rhtasv1.Auth{
		Env: []v12.EnvVar{
			{
				Name:  "VENDOR_TOKEN",
				Value: "test-token",
			},
		},
		SecretMount: []rhtasv1.SecretKeySelector{
			{
				LocalObjectReference: rhtasv1.LocalObjectReference{
					Name: "vendor-creds",
				},
			},
		},
	}

	dp, err := handleDeployment(t, instance)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(dp).ShouldNot(BeNil())

	container := dp.Spec.Template.Spec.Containers[0]
	var vendorTokenEnv *v12.EnvVar
	for i := range container.Env {
		if container.Env[i].Name == "VENDOR_TOKEN" {
			vendorTokenEnv = &container.Env[i]
			break
		}
	}
	g.Expect(vendorTokenEnv).ShouldNot(BeNil(), "VENDOR_TOKEN env var should be present on main container")
	g.Expect(vendorTokenEnv.Value).Should(Equal("test-token"))

	authVol := findVolume("auth", dp.Spec.Template.Spec.Volumes)
	g.Expect(authVol).ShouldNot(BeNil(), "auth volume should be present")
	g.Expect(authVol.Projected).ShouldNot(BeNil(), "auth volume should have Projected source")
	g.Expect(authVol.Projected.Sources).Should(HaveLen(1))
	g.Expect(authVol.Projected.Sources[0].Secret).ShouldNot(BeNil())
	g.Expect(authVol.Projected.Sources[0].Secret.Name).Should(Equal("vendor-creds"))

	authMount := findVolumeMount("auth", container.VolumeMounts)
	g.Expect(authMount).ShouldNot(BeNil(), "auth mount should be present on main container")
	g.Expect(authMount.MountPath).Should(Equal(ensure.AuthMountPath))
	g.Expect(authMount.ReadOnly).Should(BeTrue())
}

func createPKCS11Instance() *rhtasv1.Fulcio {
	return &rhtasv1.Fulcio{
		ObjectMeta: v1.ObjectMeta{
			Name:      "name",
			Namespace: "default",
		},
		Spec: rhtasv1.FulcioSpec{
			Signer: rhtasv1.FulcioSigner{
				Type: rhtasv1.FulcioSignerTypePKCS11,
				PKCS11: &rhtasv1.FulcioPKCS11Config{
					PKCS11Config: rhtasv1.PKCS11Config{
						KeyID:    ptr.To(int32(1)),
						KeyLabel: "signing-key",
					},
					ConfigRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "crypto11-config"},
						Key:                  "config.json",
					},
				},
				CertificateChain: rhtasv1.FulcioCertificateChain{
					CertificateChainRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "ca-cert"},
						Key:                  "cert.pem",
					},
				},
			},
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
				CARef: &rhtasv1.SecretKeySelector{
					Key:                  "cert.pem",
					LocalObjectReference: rhtasv1.LocalObjectReference{Name: "ca-cert"},
				},
			},
		},
	}
}

// TestFulcioPKCS11Deployment verifies that PKCS#11 mode sets up the expected
// args, volumes, and mounts, and removes file-mode resources.
func TestFulcioPKCS11Deployment(t *testing.T) {
	g := NewWithT(t)

	instance := createPKCS11Instance()
	dp, err := handleDeployment(t, instance)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(dp).ShouldNot(BeNil())

	container := dp.Spec.Template.Spec.Containers[0]

	// Verify --ca=pkcs11ca in args
	g.Expect(container.Args).Should(ContainElement(Equal("--ca=pkcs11ca")))

	// Verify --pkcs11-config-path arg
	g.Expect(container.Args).Should(ContainElement(
		Equal("--pkcs11-config-path=/var/run/pkcs11-config/config.json")))

	// Verify --hsm-caroot-id arg
	g.Expect(container.Args).Should(ContainElement(Equal("--hsm-caroot-id=1")))

	// Verify pkcs11-config volume exists
	pkcs11ConfigVol := findVolume(PKCS11ConfigVolumeName, dp.Spec.Template.Spec.Volumes)
	g.Expect(pkcs11ConfigVol).ShouldNot(BeNil(), "pkcs11-config volume should be present")
	g.Expect(pkcs11ConfigVol.Secret).ShouldNot(BeNil())
	g.Expect(pkcs11ConfigVol.Secret.SecretName).Should(Equal("crypto11-config"))

	// Verify fulcio-pkcs11-cert volume exists
	pkcs11CertVol := findVolume(PKCS11CertVolumeName, dp.Spec.Template.Spec.Volumes)
	g.Expect(pkcs11CertVol).ShouldNot(BeNil(), "fulcio-pkcs11-cert volume should be present")
	g.Expect(pkcs11CertVol.Secret).ShouldNot(BeNil())
	g.Expect(pkcs11CertVol.Secret.SecretName).Should(Equal("ca-cert"))

	// Verify hsm-tokens volume exists
	hsmTokensVol := findVolume(constants.HSMTokensVolumeName, dp.Spec.Template.Spec.Volumes)
	g.Expect(hsmTokensVol).ShouldNot(BeNil(), "hsm-tokens volume should be present")
	g.Expect(hsmTokensVol.EmptyDir).ShouldNot(BeNil())

	// Verify hsm-lib volume exists
	hsmLibVol := findVolume(constants.HSMLibVolumeName, dp.Spec.Template.Spec.Volumes)
	g.Expect(hsmLibVol).ShouldNot(BeNil(), "hsm-lib volume should be present")
	g.Expect(hsmLibVol.EmptyDir).ShouldNot(BeNil())

	// Verify fulcio-cert volume does NOT exist (cleaned up from file mode)
	g.Expect(findVolume("fulcio-cert", dp.Spec.Template.Spec.Volumes)).Should(BeNil(),
		"fulcio-cert volume should not be present in PKCS#11 mode")
}

// TestFulcioPKCS11FileModeCleansUp verifies that when using file mode, the
// PKCS#11-specific volumes are NOT present.
func TestFulcioPKCS11FileModeCleansUp(t *testing.T) {
	g := NewWithT(t)

	instance := createInstance()
	dp, err := handleDeployment(t, instance)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(dp).ShouldNot(BeNil())

	// Verify PKCS#11 volumes are NOT present
	g.Expect(findVolume(PKCS11ConfigVolumeName, dp.Spec.Template.Spec.Volumes)).Should(BeNil(),
		"pkcs11-config volume should not be present in file mode")
	g.Expect(findVolume(PKCS11CertVolumeName, dp.Spec.Template.Spec.Volumes)).Should(BeNil(),
		"fulcio-pkcs11-cert volume should not be present in file mode")
	g.Expect(findVolume(constants.HSMTokensVolumeName, dp.Spec.Template.Spec.Volumes)).Should(BeNil(),
		"hsm-tokens volume should not be present in file mode")
	g.Expect(findVolume(constants.HSMLibVolumeName, dp.Spec.Template.Spec.Volumes)).Should(BeNil(),
		"hsm-lib volume should not be present in file mode")

	// Verify file-mode volumes ARE present
	g.Expect(findVolume("fulcio-cert", dp.Spec.Template.Spec.Volumes)).ShouldNot(BeNil(),
		"fulcio-cert volume should be present in file mode")
	g.Expect(findVolume("fulcio-config", dp.Spec.Template.Spec.Volumes)).ShouldNot(BeNil(),
		"fulcio-config volume should be present in file mode")
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
