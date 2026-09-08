package actions

// Reproduction scenarios for the remaining Qodo findings on the CTLog
// sharding PR:
//
//   - resolveAllLogs dereferences specLog / specLog.Signer without a nil check
//   - resolveAllLogs never resolves an encrypted-PEM key password
//   - the deployment picks the first PKCS#11 shard's module path and silently
//     ignores the rest
//   - FIPS material collection ignores the auto-discovered Fulcio root that
//     lands in status.logs[*].rootCertificates

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	testAction "github.com/securesign/operator/internal/testing/action"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// recovered runs fn and returns the panic value, if any.
func recovered(fn func()) (v any) {
	defer func() { v = recover() }()
	fn()
	return nil
}

// Qodo finding #3: CTLogConfig.Signer is optional (the CRD only requires it
// for non-active logs, and mirrors legitimately have none). resolveAllLogs
// reads specLog.Signer.Type unconditionally and panics.
func TestQodo_ResolveAllLogs_NilSignerPanics(t *testing.T) {
	g := NewWithT(t)

	c := testAction.FakeClientBuilder().WithObjects(
		&core.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "mirror-keys", Namespace: "default"},
			Data:       map[string][]byte{"public": publicKey},
		},
		&core.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "mirror-root", Namespace: "default"},
			Data:       map[string][]byte{"cert": cert},
		},
	).Build()

	instance := &rhtasv1.CTlog{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: rhtasv1.CTlogSpec{
			Logs: []rhtasv1.CTLogConfig{
				{
					// Mirror log: signer intentionally omitted.
					Prefix: "mirror-1",
					LogId:  ptr.To(int64(444)),
					Mirror: ptr.To(true),
					RootCerts: []rhtasv1.SecretKeySelector{
						{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "mirror-root"}, Key: "cert"},
					},
				},
			},
		},
		Status: rhtasv1.CTlogStatus{
			Logs: []rhtasv1.CTlogLogStatus{
				{
					Prefix: "mirror-1",
					LogId:  ptr.To(int64(444)),
					PublicKeyRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "mirror-keys"},
						Key:                  "public",
					},
					RootCertificates: []rhtasv1.SecretKeySelector{
						{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "mirror-root"}, Key: "cert"},
					},
				},
			},
		},
	}

	a := serverConfig{}
	a.Client = c

	var err error
	p := recovered(func() { _, err = a.resolveAllLogs(t.Context(), instance) })

	g.Expect(p).To(BeNil(),
		"resolveAllLogs panicked on a signerless mirror log instead of handling it: %v", p)
	// Whatever the chosen resolution is (signerless mirror support or an
	// explicit rejection), it must be an error return, not a nil deref.
	_ = err
}

// Qodo finding #3 (adjacent): the same unguarded deref hits specLog itself.
// A status entry whose prefix is no longer present in spec.logs — e.g. the
// user removed a shard and the status has not been re-aligned yet — makes
// GetLog return nil and resolveAllLogs panics on specLog.Readonly.
func TestQodo_ResolveAllLogs_MissingSpecLogPanics(t *testing.T) {
	g := NewWithT(t)

	c := testAction.FakeClientBuilder().WithObjects(
		&core.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "keys", Namespace: "default"},
			Data:       map[string][]byte{"public": publicKey, "private": privateKey},
		},
	).Build()

	instance := &rhtasv1.CTlog{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: rhtasv1.CTlogSpec{
			Logs: []rhtasv1.CTLogConfig{
				{
					Prefix: "trusted-artifact-signer",
					Active: ptr.To(true),
					LogId:  ptr.To(int64(111)),
					Signer: &rhtasv1.CTlogSigner{Type: rhtasv1.SignerTypeFile},
				},
			},
		},
		Status: rhtasv1.CTlogStatus{
			Logs: []rhtasv1.CTlogLogStatus{
				{
					// Stale status entry: no matching spec.logs entry.
					Prefix: "removed-shard",
					LogId:  ptr.To(int64(222)),
				},
			},
		},
	}

	a := serverConfig{}
	a.Client = c

	p := recovered(func() { _, _ = a.resolveAllLogs(t.Context(), instance) })
	g.Expect(p).To(BeNil(),
		"resolveAllLogs panicked when status.logs contains a prefix absent from spec.logs: %v", p)
}

// Qodo finding #4 (controller half): even when a legacy encrypted PEM key is
// in play, resolveAllLogs never populates ShardConfig.PrivateKeyPassword, so
// there is nothing for the config serializer to emit.
func TestQodo_ResolveAllLogs_NeverResolvesKeyPassword(t *testing.T) {
	g := NewWithT(t)

	c := testAction.FakeClientBuilder().WithObjects(
		&core.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "keys", Namespace: "default"},
			Data: map[string][]byte{
				"public":   publicKey,
				"private":  privateKey,
				"password": []byte("s3cr3t"),
			},
		},
	).Build()

	instance := &rhtasv1.CTlog{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: rhtasv1.CTlogSpec{
			Logs: []rhtasv1.CTLogConfig{
				{
					Prefix: "trusted-artifact-signer",
					Active: ptr.To(true),
					LogId:  ptr.To(int64(111)),
					Signer: &rhtasv1.CTlogSigner{
						Type: rhtasv1.SignerTypeFile,
						File: &rhtasv1.CTlogFile{
							PrivateKeyRef: &rhtasv1.SecretKeySelector{
								LocalObjectReference: rhtasv1.LocalObjectReference{Name: "keys"},
								Key:                  "private",
							},
						},
					},
				},
			},
		},
		Status: rhtasv1.CTlogStatus{
			Logs: []rhtasv1.CTlogLogStatus{
				{
					Prefix: "trusted-artifact-signer",
					Active: true,
					LogId:  ptr.To(int64(111)),
					PrivateKeyRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "keys"},
						Key:                  "private",
					},
					PublicKeyRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "keys"},
						Key:                  "public",
					},
				},
			},
		},
	}

	a := serverConfig{}
	a.Client = c

	logs, err := a.resolveAllLogs(t.Context(), instance)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(logs).To(HaveLen(1))
	g.Expect(logs[0].PrivateKeyPassword).ToNot(BeEmpty(),
		"there is no API field or resolution path left for an encrypted PEM key password, "+
			"so ShardConfig.PrivateKeyPassword is always empty")
}

// Qodo finding #5: two PKCS#11 shards may declare different module paths, but
// the deployment appends only the first and breaks out of the loop, so the
// second log's module is never loaded and the mismatch is not reported.
func TestQodo_Deployment_IgnoresSecondPKCS11ModulePath(t *testing.T) {
	g := NewWithT(t)

	instance := createCTLogInstance()
	pin := &rhtasv1.SecretKeySelector{
		LocalObjectReference: rhtasv1.LocalObjectReference{Name: "hsm-pin"},
		Key:                  "pin",
	}
	pub := &rhtasv1.SecretKeySelector{
		LocalObjectReference: rhtasv1.LocalObjectReference{Name: "hsm-pub"},
		Key:                  "public",
	}
	instance.Spec.Logs = []rhtasv1.CTLogConfig{
		{
			LogId:  ptr.To(int64(123456)),
			Prefix: "trusted-artifact-signer",
			Active: ptr.To(true),
			Signer: &rhtasv1.CTlogSigner{
				Type: rhtasv1.SignerTypePKCS11,
				PKCS11: &rhtasv1.CTlogPKCS11Config{
					ModulePath:   "/usr/lib/softhsm/libsofthsm2.so",
					TokenLabel:   "active-token",
					PinSecretRef: pin,
					PublicKeyRef: pub,
				},
			},
		},
		{
			LogId:    ptr.To(int64(222222)),
			Prefix:   "shard-222222",
			Readonly: ptr.To(true),
			Signer: &rhtasv1.CTlogSigner{
				Type: rhtasv1.SignerTypePKCS11,
				PKCS11: &rhtasv1.CTlogPKCS11Config{
					// Different vendor module for the frozen shard.
					ModulePath:   "/usr/lib/luna/libCryptoki2_64.so",
					TokenLabel:   "frozen-token",
					PinSecretRef: pin,
					PublicKeyRef: pub,
				},
			},
			RootCerts: []rhtasv1.SecretKeySelector{
				{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "fulcio-secret"}, Key: "cert"},
			},
		},
	}

	dp, err := createCTLogDeployment(instance)

	// Either the conflict is rejected up front, or both modules are made
	// available. Today neither happens: the deployment is built happily with
	// only the first module path.
	if err != nil {
		g.Expect(err.Error()).To(ContainSubstring("module"),
			"unexpected error while building deployment: %v", err)
		return
	}

	var moduleArgs []string
	for _, arg := range dp.Spec.Template.Spec.Containers[0].Args {
		if strings.HasPrefix(arg, "--pkcs11_module_path=") {
			moduleArgs = append(moduleArgs, arg)
		}
	}

	g.Expect(moduleArgs).To(ContainElement(
		fmt.Sprintf("--pkcs11_module_path=%s/libCryptoki2_64.so", constants.HSMLibMountPath)),
		"the second PKCS#11 shard's module path was silently dropped; args were %v", moduleArgs)
}

// Qodo finding #6: when the active log's root certificate is auto-discovered
// from Fulcio, it is recorded in status.logs[*].rootCertificates and mounted
// into the CTFE config — but FIPS validation only walks spec.logs[*].rootCerts,
// so the certificate that is actually used is never validated.
func TestQodo_FIPSMaterial_SkipsAutodiscoveredRootCert(t *testing.T) {
	g := NewWithT(t)

	autodiscovered := []byte("-----BEGIN CERTIFICATE-----\nautodiscovered-fulcio-root\n-----END CERTIFICATE-----\n")

	c := testAction.FakeClientBuilder().WithObjects(
		&core.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "ctlog-fulcio-root-test", Namespace: "default"},
			Data:       map[string][]byte{"fulcio-root": autodiscovered},
		},
	).Build()

	instance := &rhtasv1.CTlog{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: rhtasv1.CTlogSpec{
			// No spec.logs[0].rootCerts — the active log resolves its root
			// from spec.fulcio.
			Logs: []rhtasv1.CTLogConfig{
				{
					Prefix: "trusted-artifact-signer",
					Active: ptr.To(true),
					Signer: &rhtasv1.CTlogSigner{Type: rhtasv1.SignerTypeFile},
				},
			},
			Fulcio: rhtasv1.ServiceReference{URL: "http://fulcio.default.svc"},
		},
		Status: rhtasv1.CTlogStatus{
			Logs: []rhtasv1.CTlogLogStatus{
				{
					Prefix: "trusted-artifact-signer",
					Active: true,
					LogId:  ptr.To(int64(111)),
					RootCertificates: []rhtasv1.SecretKeySelector{
						{
							LocalObjectReference: rhtasv1.LocalObjectReference{Name: "ctlog-fulcio-root-test"},
							Key:                  "fulcio-root",
						},
					},
				},
			},
		},
	}

	refs, err := ctlogCryptoMaterial(t.Context(), instance, client.Client(c))
	g.Expect(err).ToNot(HaveOccurred())

	var collected [][]byte
	for _, r := range refs {
		collected = append(collected, r.Data)
	}
	g.Expect(collected).To(ContainElement(autodiscovered),
		"the auto-discovered Fulcio root recorded in status.logs[0].rootCertificates is mounted "+
			"into the CTFE config but is never FIPS-validated; collected refs: %d", len(refs))
}
