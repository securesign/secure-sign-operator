package actions

import (
	"maps"
	"slices"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure/deployment"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func createCTLogInstance() *rhtasv1.CTlog {
	return &rhtasv1.CTlog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ctlog",
			Namespace: "default",
		},
		Spec: rhtasv1.CTlogSpec{
			Trillian: rhtasv1.ServiceReference{
				URL: "trillian-logserver.default.svc:8091",
			},
			Logs: []rhtasv1.CTLogConfig{
				{
					LogId:  ptr.To(int64(123456)),
					Prefix: "trusted-artifact-signer",
					Active: ptr.To(true),
					Signer: &rhtasv1.CTlogSigner{Type: "file"},
					RootCerts: &rhtasv1.RootCertBinding{Roots: []rhtasv1.SecretKeySelector{
						{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "fulcio-secret"}, Key: "cert"},
					}},
				},
			},
		},
		Status: rhtasv1.CTlogStatus{
			ServerConfigRef: &rhtasv1.LocalObjectReference{Name: "ctlog-config"},
			TreeID:          ptr.To(int64(123456)),
		},
	}
}

func createCTLogDeployment(instance *rhtasv1.CTlog) (*apps.Deployment, error) {
	l := labels.For(ComponentName, DeploymentName, instance.Name)
	dp := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DeploymentName,
			Namespace: instance.Namespace,
		},
	}

	action := deployAction{}
	ensures := []func(*apps.Deployment) error{
		deployment.PodExtensions(instance.Spec.PodExtensions, containerName),
		action.ensureDeployment(instance, RBACName, l),
		ensure.Labels[*apps.Deployment](slices.Collect(maps.Keys(l)), l),
		deployment.Auth(containerName, instance.Spec.Auth),
	}
	for _, en := range ensures {
		if err := en(dp); err != nil {
			return nil, err
		}
	}
	return dp, nil
}

func findCTLogVolume(name string, volumes []core.Volume) *core.Volume {
	for i := range volumes {
		if volumes[i].Name == name {
			return &volumes[i]
		}
	}
	return nil
}

func findCTLogVolumeMount(name string, mounts []core.VolumeMount) *core.VolumeMount {
	for i := range mounts {
		if mounts[i].Name == name {
			return &mounts[i]
		}
	}
	return nil
}

// TestCTLogUserDefinedVolumesInFileMode verifies that user-specified Volumes and
// VolumeMounts on CTlogSpec are applied to the deployment alongside
// operator-managed volumes.
func TestCTLogUserDefinedVolumesInFileMode(t *testing.T) {
	g := NewWithT(t)

	instance := createCTLogInstance()
	// Add user-defined volumes
	instance.Spec.Volumes = []rhtasv1.AdditionalVolume{
		{
			Name: "custom-data",
			AdditionalVolumeSource: rhtasv1.AdditionalVolumeSource{
				PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
					ClaimName: "custom-data-pvc",
				},
			},
		},
		{
			Name: "custom-config",
			AdditionalVolumeSource: rhtasv1.AdditionalVolumeSource{
				ConfigMap: &core.ConfigMapVolumeSource{
					LocalObjectReference: core.LocalObjectReference{
						Name: "vendor-settings",
					},
				},
			},
		},
	}
	// Add user-defined volume mounts
	instance.Spec.VolumeMounts = []core.VolumeMount{
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

	dp, err := createCTLogDeployment(instance)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(dp).ShouldNot(BeNil())

	// Verify user-defined volumes are present with correct sources
	customDataVol := findCTLogVolume("custom-data", dp.Spec.Template.Spec.Volumes)
	g.Expect(customDataVol).ShouldNot(BeNil(), "custom-data volume should be present")
	g.Expect(customDataVol.PersistentVolumeClaim).ShouldNot(BeNil())
	g.Expect(customDataVol.PersistentVolumeClaim.ClaimName).Should(Equal("custom-data-pvc"))

	customConfigVol := findCTLogVolume("custom-config", dp.Spec.Template.Spec.Volumes)
	g.Expect(customConfigVol).ShouldNot(BeNil(), "custom-config volume should be present")
	g.Expect(customConfigVol.ConfigMap).ShouldNot(BeNil())
	g.Expect(customConfigVol.ConfigMap.Name).Should(Equal("vendor-settings"))
	g.Expect(customConfigVol.ConfigMap.Name).Should(Equal("vendor-settings"))

	// Verify user-defined volume mounts are present on the main container
	container := dp.Spec.Template.Spec.Containers[0]
	customDataMount := findCTLogVolumeMount("custom-data", container.VolumeMounts)
	g.Expect(customDataMount).ShouldNot(BeNil(), "custom-data mount should be present on main container")
	g.Expect(customDataMount.MountPath).Should(Equal("/var/lib/custom/data"))

	customConfigMount := findCTLogVolumeMount("custom-config", container.VolumeMounts)
	g.Expect(customConfigMount).ShouldNot(BeNil(), "custom-config mount should be present on main container")
	g.Expect(customConfigMount.MountPath).Should(Equal("/etc/custom"))
	g.Expect(customConfigMount.ReadOnly).Should(BeTrue())

	// Verify operator-managed "keys" volume still present
	keysVol := findCTLogVolume(volumeName, dp.Spec.Template.Spec.Volumes)
	g.Expect(keysVol).ShouldNot(BeNil(), "operator-managed keys volume should still be present")

	// Verify operator-managed "keys" volume mount still present at /ctfe-keys
	keysMount := findCTLogVolumeMount(volumeName, container.VolumeMounts)
	g.Expect(keysMount).ShouldNot(BeNil(), "operator-managed keys mount should still be present")
	g.Expect(keysMount.MountPath).Should(Equal("/ctfe-keys"))
}

// TestCTLogUserDefinedInitContainersInFileMode verifies that user-specified
// init containers are applied to the deployment.
func TestCTLogUserDefinedInitContainersInFileMode(t *testing.T) {
	g := NewWithT(t)

	instance := createCTLogInstance()
	instance.Spec.InitContainers = []rhtasv1.InitContainerSpec{
		{
			Name:    "setup-init",
			Image:   "vendor-init:latest",
			Command: []string{"/bin/setup"},
		},
	}

	dp, err := createCTLogDeployment(instance)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(dp).ShouldNot(BeNil())

	// Verify init container is present
	g.Expect(dp.Spec.Template.Spec.InitContainers).Should(HaveLen(1))
	initContainer := dp.Spec.Template.Spec.InitContainers[0]
	g.Expect(initContainer.Name).Should(Equal("setup-init"))
	g.Expect(initContainer.Image).Should(Equal("vendor-init:latest"))
	g.Expect(initContainer.Command).Should(Equal([]string{"/bin/setup"}))
}

// TestCTLogAuthInjectionInFileMode verifies that auth env vars and secret
// mounts from spec.signer.auth are applied to the deployment.
// mounts from spec.auth are applied to the deployment.
func TestCTLogAuthInjectionInFileMode(t *testing.T) {
	g := NewWithT(t)

	instance := createCTLogInstance()
	instance.Spec.Auth = &rhtasv1.Auth{
		Env: []core.EnvVar{
			{
				Name: "VENDOR_TOKEN",
				ValueFrom: &core.EnvVarSource{
					SecretKeyRef: &core.SecretKeySelector{
						LocalObjectReference: core.LocalObjectReference{
							Name: "vendor-credentials",
						},
						Key: "token",
					},
				},
			},
		},
	}

	dp, err := createCTLogDeployment(instance)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(dp).ShouldNot(BeNil())

	// Verify VENDOR_TOKEN env var is present on main container
	container := dp.Spec.Template.Spec.Containers[0]
	var vendorTokenEnv *core.EnvVar
	for i := range container.Env {
		if container.Env[i].Name == "VENDOR_TOKEN" {
			vendorTokenEnv = &container.Env[i]
			break
		}
	}
	g.Expect(vendorTokenEnv).ShouldNot(BeNil(), "VENDOR_TOKEN env var should be present on main container")
	g.Expect(vendorTokenEnv.ValueFrom).ShouldNot(BeNil())
	g.Expect(vendorTokenEnv.ValueFrom.SecretKeyRef.Name).Should(Equal("vendor-credentials"))

	// Verify "auth" volume is present
	authVol := findCTLogVolume("auth", dp.Spec.Template.Spec.Volumes)
	g.Expect(authVol).ShouldNot(BeNil(), "auth volume should be present")

	// Verify "auth" volume mount is present on main container
	authMount := findCTLogVolumeMount("auth", container.VolumeMounts)
	g.Expect(authMount).ShouldNot(BeNil(), "auth mount should be present on main container")
}

// TestCTLogPKCS11VolumesAndMounts verifies that PKCS#11 mode adds the expected
// HSM volumes, mounts, and --pkcs11_module_path argument.
func TestCTLogPKCS11VolumesAndMounts(t *testing.T) {
	g := NewWithT(t)

	instance := createCTLogInstance()
	instance.Spec.Logs[0].Signer.Type = rhtasv1.SignerTypePKCS11
	instance.Spec.Logs[0].Signer.PKCS11 = &rhtasv1.CTlogPKCS11Config{
		ModulePath: "/usr/lib64/pkcs11/libsofthsm2.so",
		TokenLabel: "test-token",
		PinSecretRef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "pin-secret"},
			Key:                  "pin",
		},
		PublicKeyRef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "pubkey-secret"},
			Key:                  "public",
		},
	}

	dp, err := createCTLogDeployment(instance)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(dp).ShouldNot(BeNil())

	// Verify hsm-tokens volume exists (defaults to EmptyDir when not user-provided)
	hsmTokensVol := findCTLogVolume(constants.HSMTokensVolumeName, dp.Spec.Template.Spec.Volumes)
	g.Expect(hsmTokensVol).ShouldNot(BeNil(), "hsm-tokens volume should be present")
	g.Expect(hsmTokensVol.EmptyDir).ShouldNot(BeNil(), "hsm-tokens should default to EmptyDir")

	// Verify hsm-lib volume exists (always EmptyDir)
	hsmLibVol := findCTLogVolume(constants.HSMLibVolumeName, dp.Spec.Template.Spec.Volumes)
	g.Expect(hsmLibVol).ShouldNot(BeNil(), "hsm-lib volume should be present")
	g.Expect(hsmLibVol.EmptyDir).ShouldNot(BeNil(), "hsm-lib should be EmptyDir")

	// Verify volume mounts on main container
	container := dp.Spec.Template.Spec.Containers[0]
	hsmTokensMount := findCTLogVolumeMount(constants.HSMTokensVolumeName, container.VolumeMounts)
	g.Expect(hsmTokensMount).ShouldNot(BeNil(), "hsm-tokens mount should be present on main container")
	g.Expect(hsmTokensMount.MountPath).Should(Equal(constants.HSMTokensMountPath))

	hsmLibMount := findCTLogVolumeMount(constants.HSMLibVolumeName, container.VolumeMounts)
	g.Expect(hsmLibMount).ShouldNot(BeNil(), "hsm-lib mount should be present on main container")
	g.Expect(hsmLibMount.MountPath).Should(Equal(constants.HSMLibMountPath))
	g.Expect(hsmLibMount.ReadOnly).Should(BeTrue())

	// Verify --pkcs11_module_path arg uses path.Base of modulePath
	g.Expect(container.Args).Should(ContainElement(
		Equal("--pkcs11_module_path=/var/run/hsm-lib/libsofthsm2.so")))
}

// TestCTLogPKCS11CleanupOnFileMode verifies that switching from PKCS#11 to file mode
// removes the hsm-tokens and hsm-lib volumes.
func TestCTLogPKCS11CleanupOnFileMode(t *testing.T) {
	g := NewWithT(t)

	// First, create a deployment in PKCS#11 mode
	instance := createCTLogInstance()
	instance.Spec.Logs[0].Signer.Type = rhtasv1.SignerTypePKCS11
	instance.Spec.Logs[0].Signer.PKCS11 = &rhtasv1.CTlogPKCS11Config{
		ModulePath: "/usr/lib64/pkcs11/libsofthsm2.so",
		TokenLabel: "test-token",
		PinSecretRef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "pin-secret"},
			Key:                  "pin",
		},
		PublicKeyRef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "pubkey-secret"},
			Key:                  "public",
		},
	}

	dp, err := createCTLogDeployment(instance)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(findCTLogVolume(constants.HSMTokensVolumeName, dp.Spec.Template.Spec.Volumes)).ShouldNot(BeNil(),
		"precondition: hsm-tokens should be present in PKCS#11 mode")
	g.Expect(findCTLogVolume(constants.HSMLibVolumeName, dp.Spec.Template.Spec.Volumes)).ShouldNot(BeNil(),
		"precondition: hsm-lib should be present in PKCS#11 mode")

	// Now switch to file mode and re-apply the deployment ensures
	instance.Spec.Logs[0].Signer.Type = rhtasv1.SignerTypeFile
	instance.Spec.Logs[0].Signer.PKCS11 = nil

	l := labels.For(ComponentName, DeploymentName, instance.Name)
	action := deployAction{}
	ensures := []func(*apps.Deployment) error{
		deployment.PodExtensions(instance.Spec.PodExtensions, containerName),
		action.ensureDeployment(instance, RBACName, l),
		ensure.Labels[*apps.Deployment](slices.Collect(maps.Keys(l)), l),
		deployment.Auth(containerName, instance.Spec.Auth),
	}
	for _, en := range ensures {
		err := en(dp)
		g.Expect(err).ShouldNot(HaveOccurred())
	}

	// Verify PKCS#11 volumes are removed
	g.Expect(findCTLogVolume(constants.HSMTokensVolumeName, dp.Spec.Template.Spec.Volumes)).Should(BeNil(),
		"hsm-tokens volume should be removed in file mode")
	g.Expect(findCTLogVolume(constants.HSMLibVolumeName, dp.Spec.Template.Spec.Volumes)).Should(BeNil(),
		"hsm-lib volume should be removed in file mode")

	// Verify PKCS#11 mounts are removed from main container
	container := dp.Spec.Template.Spec.Containers[0]
	g.Expect(findCTLogVolumeMount(constants.HSMTokensVolumeName, container.VolumeMounts)).Should(BeNil(),
		"hsm-tokens mount should be removed in file mode")
	g.Expect(findCTLogVolumeMount(constants.HSMLibVolumeName, container.VolumeMounts)).Should(BeNil(),
		"hsm-lib mount should be removed in file mode")
}

// TestCTLogPKCS11UserPVCPreserved verifies that a user-defined PVC for hsm-tokens
// is preserved and not overwritten with EmptyDir.
func TestCTLogPKCS11UserPVCPreserved(t *testing.T) {
	g := NewWithT(t)

	instance := createCTLogInstance()
	instance.Spec.Logs[0].Signer.Type = rhtasv1.SignerTypePKCS11
	instance.Spec.Logs[0].Signer.PKCS11 = &rhtasv1.CTlogPKCS11Config{
		ModulePath: "/usr/lib64/pkcs11/libsofthsm2.so",
		TokenLabel: "test-token",
		PinSecretRef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "pin-secret"},
			Key:                  "pin",
		},
		PublicKeyRef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "pubkey-secret"},
			Key:                  "public",
		},
	}
	// User defines hsm-tokens as a PVC
	instance.Spec.Volumes = []rhtasv1.AdditionalVolume{
		{
			Name: constants.HSMTokensVolumeName,
			AdditionalVolumeSource: rhtasv1.AdditionalVolumeSource{
				PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
					ClaimName: "softhsm-tokens-pvc",
				},
			},
		},
	}

	dp, err := createCTLogDeployment(instance)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(dp).ShouldNot(BeNil())

	// Verify the PVC source is preserved (not overwritten with EmptyDir)
	hsmTokensVol := findCTLogVolume(constants.HSMTokensVolumeName, dp.Spec.Template.Spec.Volumes)
	g.Expect(hsmTokensVol).ShouldNot(BeNil(), "hsm-tokens volume should be present")
	g.Expect(hsmTokensVol.PersistentVolumeClaim).ShouldNot(BeNil(),
		"hsm-tokens PVC source should be preserved")
	g.Expect(hsmTokensVol.PersistentVolumeClaim.ClaimName).Should(Equal("softhsm-tokens-pvc"))
	g.Expect(hsmTokensVol.EmptyDir).Should(BeNil(),
		"hsm-tokens should NOT have EmptyDir when user provides PVC")
}

// TestCTLogOperatorVolumePrecedence verifies that an operator-managed volume
// is NOT overwritten by a user volume with the same name.
func TestCTLogOperatorVolumePrecedence(t *testing.T) {
	g := NewWithT(t)

	instance := createCTLogInstance()
	// Add user volume with same name as operator-managed volume
	instance.Spec.Volumes = []rhtasv1.AdditionalVolume{
		{
			Name: volumeName, // "keys" - same as operator-managed volume
			AdditionalVolumeSource: rhtasv1.AdditionalVolumeSource{
				EmptyDir: &core.EmptyDirVolumeSource{},
			},
		},
	}

	dp, err := createCTLogDeployment(instance)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(dp).ShouldNot(BeNil())

	// Verify the "keys" volume has Secret source (operator wins), NOT EmptyDir
	keysVol := findCTLogVolume(volumeName, dp.Spec.Template.Spec.Volumes)
	g.Expect(keysVol).ShouldNot(BeNil(), "keys volume should be present")
	g.Expect(keysVol.Secret).ShouldNot(BeNil(), "keys volume should have Secret source (operator wins)")
	g.Expect(keysVol.Secret.SecretName).Should(Equal("ctlog-config"))
	g.Expect(keysVol.EmptyDir).Should(BeNil(), "keys volume should NOT have EmptyDir source")
}
