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
	for _, v := range volumes {
		if v.Name == name {
			return &v
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
		testAction.ensureDeployment(instance, RBACName, labels),
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

func TestEnsureVolumeDefaultMode(t *testing.T) {
	g := NewWithT(t)

	defaultMode := ptr.To(int32(0644))

	tests := []struct {
		name   string
		volume v12.Volume
		verify func(g Gomega, v *v12.Volume)
	}{
		{
			name: "ConfigMap without DefaultMode gets default",
			volume: v12.Volume{
				Name: "config",
				VolumeSource: v12.VolumeSource{
					ConfigMap: &v12.ConfigMapVolumeSource{
						LocalObjectReference: v12.LocalObjectReference{Name: "my-config"},
					},
				},
			},
			verify: func(g Gomega, v *v12.Volume) {
				g.Expect(v.ConfigMap.DefaultMode).Should(Equal(defaultMode))
			},
		},
		{
			name: "ConfigMap with explicit DefaultMode is preserved",
			volume: v12.Volume{
				Name: "config",
				VolumeSource: v12.VolumeSource{
					ConfigMap: &v12.ConfigMapVolumeSource{
						LocalObjectReference: v12.LocalObjectReference{Name: "my-config"},
						DefaultMode:          ptr.To(int32(0600)),
					},
				},
			},
			verify: func(g Gomega, v *v12.Volume) {
				g.Expect(v.ConfigMap.DefaultMode).Should(Equal(ptr.To(int32(0600))))
			},
		},
		{
			name: "Secret without DefaultMode gets default",
			volume: v12.Volume{
				Name: "secret",
				VolumeSource: v12.VolumeSource{
					Secret: &v12.SecretVolumeSource{SecretName: "my-secret"},
				},
			},
			verify: func(g Gomega, v *v12.Volume) {
				g.Expect(v.Secret.DefaultMode).Should(Equal(defaultMode))
			},
		},
		{
			name: "Projected without DefaultMode gets default",
			volume: v12.Volume{
				Name: "projected",
				VolumeSource: v12.VolumeSource{
					Projected: &v12.ProjectedVolumeSource{},
				},
			},
			verify: func(g Gomega, v *v12.Volume) {
				g.Expect(v.Projected.DefaultMode).Should(Equal(defaultMode))
			},
		},
		{
			name: "EmptyDir is not modified",
			volume: v12.Volume{
				Name: "empty",
				VolumeSource: v12.VolumeSource{
					EmptyDir: &v12.EmptyDirVolumeSource{},
				},
			},
			verify: func(g Gomega, v *v12.Volume) {
				g.Expect(v.EmptyDir).ShouldNot(BeNil())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := tt.volume
			ensureVolumeDefaultMode(&v)
			tt.verify(g, &v)
		})
	}
}

func TestPKCS11UserDefinedVolumesGetDefaultMode(t *testing.T) {
	g := NewWithT(t)

	instance := createPKCS11Instance()
	instance.Spec.Certificate.PKCS11.Volumes = []v12.Volume{
		{
			Name: "softhsm-config",
			VolumeSource: v12.VolumeSource{
				ConfigMap: &v12.ConfigMapVolumeSource{
					LocalObjectReference: v12.LocalObjectReference{Name: "softhsm-config"},
				},
			},
		},
	}
	labels := labels.For(componentName, deploymentName, instance.Name)
	dp, err := createDeployment(instance, labels)
	g.Expect(err).ShouldNot(HaveOccurred())

	softhsmVol := findVolume("softhsm-config", dp.Spec.Template.Spec.Volumes)
	g.Expect(softhsmVol).ShouldNot(BeNil())
	g.Expect(softhsmVol.ConfigMap.DefaultMode).Should(Equal(ptr.To(int32(0644))),
		"User-defined ConfigMap volume should get Kubernetes default DefaultMode to avoid reconcile loops")
}

func createPKCS11Instance() *rhtasv1.Fulcio {
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
			Certificate: rhtasv1.FulcioCert{
				CAType: rhtasv1.CATypePKCS11,
				CARef: &rhtasv1.SecretKeySelector{
					Key:                  "cert.pem",
					LocalObjectReference: rhtasv1.LocalObjectReference{Name: "fulcio-root-ca"},
				},
				PKCS11: &rhtasv1.FulcioPKCS11Config{
					CredentialsRef: rhtasv1.SecretKeySelector{
						Key:                  "pin",
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "hsm-credentials"},
					},
					PKCS11ConfigRef: rhtasv1.SecretKeySelector{
						Key:                  "crypto11.conf",
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "pkcs11-config"},
					},
					KeyConfig: rhtasv1.PKCS11KeyConfig{
						ID:    99,
						Label: "PKCS11CA",
					},
					InitContainers: []rhtasv1.PKCS11InitContainerSpec{
						{
							Name:  "hsm-init",
							Image: "quay.io/example/hsm-init:latest",
						},
					},
				},
			},
		},
		Status: rhtasv1.FulcioStatus{
			ServerConfigRef: &rhtasv1.LocalObjectReference{Name: "config"},
			Certificate: &rhtasv1.FulcioCertStatus{
				CARef: &rhtasv1.SecretKeySelector{
					Key:                  "cert.pem",
					LocalObjectReference: rhtasv1.LocalObjectReference{Name: "fulcio-root-ca"},
				},
			},
			PKCS11: &rhtasv1.FulcioPKCS11Status{
				PKCS11ConfigRef: &rhtasv1.SecretKeySelector{
					Key:                  "crypto11.conf",
					LocalObjectReference: rhtasv1.LocalObjectReference{Name: "fulcio-pkcs11-config"},
				},
				CredentialsRef: &rhtasv1.SecretKeySelector{
					Key:                  "pin",
					LocalObjectReference: rhtasv1.LocalObjectReference{Name: "hsm-credentials"},
				},
			},
		},
	}
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
