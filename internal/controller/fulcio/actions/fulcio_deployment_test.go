package actions

import (
	"maps"
	"slices"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/controller/fulcio/utils"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/fips"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure/deployment"
	v13 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
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

func TestCtlogConfig(t *testing.T) {
	tests := []struct {
		name   string
		args   rhtasv1.CtlogService
		verify func(Gomega, *v13.Deployment, error)
	}{
		{
			name: "missing address",
			args: rhtasv1.CtlogService{
				Port:   ptr.To(int32(1234)),
				Prefix: "prefix",
			},
			verify: func(g Gomega, deployment *v13.Deployment, err error) {
				g.Expect(err).Should(Succeed())
				g.Expect(deployment.Spec.Template.Spec.Containers[0].Args).Should(ContainElement(Equal("--ct-log-url=http://ctlog.default.svc/prefix")))

			},
		},
		{
			name: "missing prefix",
			args: rhtasv1.CtlogService{
				Address: "http://address",
				Port:    ptr.To(int32(1234)),
			},
			verify: func(g Gomega, deployment *v13.Deployment, err error) {
				g.Expect(err).Should(HaveOccurred())
				g.Expect(err).Should(MatchError(utils.ErrCtlogPrefixNotSpecified))
			},
		},
		{
			name: "valid",
			args: rhtasv1.CtlogService{
				Address: "http://address",
				Port:    ptr.To(int32(1234)),
				Prefix:  "prefix",
			},
			verify: func(g Gomega, deployment *v13.Deployment, err error) {
				g.Expect(err).Should(Succeed())
				g.Expect(deployment.Spec.Template.Spec.Containers[0].Args).Should(ContainElement(Equal("--ct-log-url=http://address:1234/prefix")))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			instance := createInstance()
			instance.Spec.Ctlog = tt.args
			deployment, err := createDeployment(instance, map[string]string{})
			tt.verify(g, deployment, err)
		})
	}
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
	port := int32(80)
	return &rhtasv1.Fulcio{
		ObjectMeta: v1.ObjectMeta{
			Name:      "name",
			Namespace: "default",
		},
		Spec: rhtasv1.FulcioSpec{
			Ctlog: rhtasv1.CtlogService{
				Address: "http://ctlog.default.svc",
				Port:    &port,
				Prefix:  "prefix",
			},
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
	testAction := deployAction{}
	d := &v13.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      DeploymentName,
			Namespace: instance.Namespace,
		},
	}

	ensures := []func(*v13.Deployment) error{
		deployment.PodResources(instance.Spec.InitContainers, instance.Spec.Volumes,
			instance.Spec.VolumeMounts, containerName),
		testAction.ensureFileCADeployment(instance, RBACName, labels),
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

// TestUserDefinedVolumesInFileMode verifies that user-specified Volumes and
// VolumeMounts on FulcioSpec are applied to the deployment even in file signer
// mode. This validates that user-defined volumes and mounts are applied regardless of signer type.
func TestUserDefinedVolumesInFileMode(t *testing.T) {
	g := NewWithT(t)

	instance := createInstance()
	// Add user-defined volumes and volume mounts
	instance.Spec.Volumes = []v12.Volume{
		{
			Name: "custom-data",
			VolumeSource: v12.VolumeSource{
				PersistentVolumeClaim: &v12.PersistentVolumeClaimVolumeSource{
					ClaimName: "custom-data-pvc",
				},
			},
		},
		{
			Name: "custom-config",
			VolumeSource: v12.VolumeSource{
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

	labels := labels.For(componentName, DeploymentName, instance.Name)
	dp, err := createDeployment(instance, labels)
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
	// EnsureVolumeDefaultMode should have set DefaultMode on ConfigMap
	g.Expect(customConfigVol.ConfigMap.DefaultMode).Should(HaveValue(Equal(int32(0644))))

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

	labels := labels.For(componentName, DeploymentName, instance.Name)
	dp, err := createDeployment(instance, labels)
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
	instance.Spec.Volumes = []v12.Volume{
		{
			Name: "fulcio-config",
			VolumeSource: v12.VolumeSource{
				EmptyDir: &v12.EmptyDirVolumeSource{},
			},
		},
	}

	labels := labels.For(componentName, DeploymentName, instance.Name)
	dp, err := createDeployment(instance, labels)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(dp).ShouldNot(BeNil())

	configVol := findVolume("fulcio-config", dp.Spec.Template.Spec.Volumes)
	g.Expect(configVol).ShouldNot(BeNil(), "fulcio-config volume should be present")
	g.Expect(configVol.ConfigMap).ShouldNot(BeNil(), "fulcio-config should have ConfigMap source (operator wins)")
	g.Expect(configVol.ConfigMap.Name).Should(Equal("config"))
	g.Expect(configVol.EmptyDir).Should(BeNil(), "fulcio-config should NOT have EmptyDir source")
}

// TestAuthInjectionInFileMode verifies that auth env vars and secret mounts
// from spec.signer.auth are applied to the deployment.
func TestAuthInjectionInFileMode(t *testing.T) {
	g := NewWithT(t)

	instance := createInstance()
	instance.Spec.Signer.Auth = &rhtasv1.Auth{
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

	labels := labels.For(componentName, DeploymentName, instance.Name)
	dp, err := createDeployment(instance, labels)
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

	authVol := findVolume("signer-auth", dp.Spec.Template.Spec.Volumes)
	g.Expect(authVol).ShouldNot(BeNil(), "signer-auth volume should be present")
	g.Expect(authVol.Projected).ShouldNot(BeNil(), "signer-auth volume should have Projected source")
	g.Expect(authVol.Projected.Sources).Should(HaveLen(1))
	g.Expect(authVol.Projected.Sources[0].Secret).ShouldNot(BeNil())
	g.Expect(authVol.Projected.Sources[0].Secret.Name).Should(Equal("vendor-creds"))

	authMount := findVolumeMount("signer-auth", container.VolumeMounts)
	g.Expect(authMount).ShouldNot(BeNil(), "signer-auth mount should be present on main container")
	g.Expect(authMount.MountPath).Should(Equal(ensure.AuthMountPath))
	g.Expect(authMount.ReadOnly).Should(BeTrue())
}

func TestResolveCTLUrl(t *testing.T) {
	g := NewWithT(t)
	action := deployAction{}

	tests := []struct {
		name   string
		ctl    rhtasv1.CtlogService
		tls    bool
		assert func(g Gomega, url string, err error)
	}{
		{
			name: "empty preffix",
			ctl:  rhtasv1.CtlogService{Prefix: ""},
			assert: func(g Gomega, url string, err error) {
				g.Expect(err).Should(HaveOccurred())
				g.Expect(err).Should(MatchError(utils.ErrCtlogPrefixNotSpecified))
			},
		},
		{
			name: "address no port",
			ctl:  rhtasv1.CtlogService{Prefix: "test", Address: "http://ctlog.default.svc", Port: nil},
			assert: func(g Gomega, url string, err error) {
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(url).Should(Equal("http://ctlog.default.svc/test"))
			},
		},
		{
			name: "address with port",
			ctl:  rhtasv1.CtlogService{Prefix: "test", Address: "http://ctlog.default.svc", Port: ptr.To(int32(8080))},
			assert: func(g Gomega, url string, err error) {
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(url).Should(Equal("http://ctlog.default.svc:8080/test"))
			},
		},
		{
			name: "address with port",
			ctl:  rhtasv1.CtlogService{Prefix: "test", Address: "http://ctlog.default.svc", Port: ptr.To(int32(8080))},
			assert: func(g Gomega, url string, err error) {
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(url).Should(Equal("http://ctlog.default.svc:8080/test"))
			},
		},
		{
			name: "autoresolve address no TLS",
			ctl:  rhtasv1.CtlogService{Prefix: "test"},
			tls:  false,
			assert: func(g Gomega, url string, err error) {
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(url).Should(Equal("http://ctlog.default.svc/test"))
			},
		},
		{
			name: "autoresolve address TLS",
			ctl:  rhtasv1.CtlogService{Prefix: "test"},
			tls:  true,
			assert: func(g Gomega, url string, err error) {
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(url).Should(Equal("https://ctlog.default.svc/test"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := createInstance()
			instance.Spec.Ctlog = tt.ctl
			if tt.tls {
				instance.Spec.TrustedCA = &rhtasv1.LocalObjectReference{}
			}
			url, err := action.resolveCTlogUrl(instance)
			tt.assert(g, url, err)
		})
	}
}
