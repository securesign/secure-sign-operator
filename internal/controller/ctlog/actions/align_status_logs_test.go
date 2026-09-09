package actions

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestAlignStatusLogs_CanHandle(t *testing.T) {
	tests := []struct {
		name      string
		phase     state.State
		canHandle bool
	}{
		{"pending", state.Pending, false},
		{"creating", state.Creating, true},
		{"initialize", state.Initialize, true},
		{"ready", state.Ready, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			instance := &rhtasv1.CTlog{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Status: rhtasv1.CTlogStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Reason: tt.phase.String()},
					},
				},
			}
			c := testAction.FakeClientBuilder().Build()
			a := testAction.PrepareAction(c, NewAlignStatusLogsAction())
			g.Expect(a.CanHandle(t.Context(), instance)).To(Equal(tt.canHandle))
		})
	}
}

func TestAlignStatusLogs_RejectsEmptySpecLogs(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := &rhtasv1.CTlog{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Status: rhtasv1.CTlogStatus{
			Logs: []rhtasv1.CTlogLogStatus{
				{
					Prefix: "trusted-artifact-signer",
					Active: true,
					LogId:  ptr.To(int64(123456)),
					PrivateKeyRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "ctlog-keys-test-xyz99"},
						Key:                  "private",
					},
					PublicKeyRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "ctlog-keys-test-xyz99"},
						Key:                  "public",
					},
				},
			},
		},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()
	key := client.ObjectKeyFromObject(instance)
	storedBefore := &rhtasv1.CTlog{}
	g.Expect(c.Get(ctx, key, storedBefore)).To(Succeed())
	instanceBefore := instance.DeepCopy()

	a := testAction.PrepareAction(c, NewAlignStatusLogsAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).NotTo(BeNil())
	g.Expect(result.Err).To(MatchError("at least one log is required"))
	g.Expect(instance).To(Equal(instanceBefore))
	storedAfter := &rhtasv1.CTlog{}
	g.Expect(c.Get(ctx, key, storedAfter)).To(Succeed())
	g.Expect(storedAfter).To(Equal(storedBefore))
}

func TestAlignStatusLogs_ActiveLog(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	instance := &rhtasv1.CTlog{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: rhtasv1.CTlogSpec{
			Logs: []rhtasv1.CTLogConfig{
				{
					Prefix: "trusted-artifact-signer",
					Active: ptr.To(true),
					Signer: &rhtasv1.CTlogSigner{Type: "file"},
				},
			},
		},
		Status: rhtasv1.CTlogStatus{
			Logs: []rhtasv1.CTlogLogStatus{
				{
					Prefix: "trusted-artifact-signer",
					LogId:  ptr.To(int64(12345)),
					PrivateKeyRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "keys"},
						Key:                  "private",
					},
					PublicKeyRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "keys"},
						Key:                  "public",
					},
					PublicKey: "-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----\n",
					RootCertificates: []rhtasv1.SecretKeySelector{
						{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "root"}, Key: "cert"},
					},
				},
			},
			Conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Reason: state.Initialize.String()},
			},
		},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()
	a := testAction.PrepareAction(c, NewAlignStatusLogsAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).To(Equal(testAction.Return()))
	g.Expect(instance.Status.Logs).To(HaveLen(1))
	g.Expect(instance.Status.Logs[0].Prefix).To(Equal("trusted-artifact-signer"))
	g.Expect(instance.Status.Logs[0].LogId).To(Equal(ptr.To(int64(12345))))
	g.Expect(instance.Status.Logs[0].PrivateKeyRef.Name).To(Equal("keys"))
	g.Expect(instance.Status.Logs[0].PublicKeyRef.Name).To(Equal("keys"))
	g.Expect(instance.Status.Logs[0].PublicKey).To(ContainSubstring("PUBLIC KEY"))
	g.Expect(instance.Status.Logs[0].RootCertificates).To(HaveLen(1))
}

func TestAlignStatusLogs_ActiveAndReadonlyShards(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	instance := &rhtasv1.CTlog{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: rhtasv1.CTlogSpec{
			Logs: []rhtasv1.CTLogConfig{
				{
					Prefix: "trusted-artifact-signer",
					Active: ptr.To(true),
					Signer: &rhtasv1.CTlogSigner{Type: "file"},
				},
				{
					Prefix:   "shard-2024",
					Readonly: ptr.To(true),
					LogId:    ptr.To(int64(99999)),
					Signer: &rhtasv1.CTlogSigner{
						Type: "file",
						File: &rhtasv1.CTlogFile{
							PrivateKeyRef: &rhtasv1.SecretKeySelector{
								LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-keys"},
								Key:                  "private",
							},
							PublicKeyRef: &rhtasv1.SecretKeySelector{
								LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-keys"},
								Key:                  "public",
							},
						},
					},
					RootCerts: []rhtasv1.SecretKeySelector{
						{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-root"}, Key: "cert"},
					},
				},
			},
		},
		Status: rhtasv1.CTlogStatus{
			Logs: []rhtasv1.CTlogLogStatus{
				{
					Prefix: "trusted-artifact-signer",
					LogId:  ptr.To(int64(12345)),
					PrivateKeyRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "keys"},
						Key:                  "private",
					},
					PublicKeyRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "keys"},
						Key:                  "public",
					},
					PublicKey: "-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----\n",
					RootCertificates: []rhtasv1.SecretKeySelector{
						{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "root"}, Key: "cert"},
					},
				},
			},
			Conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Reason: state.Initialize.String()},
			},
		},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()
	a := testAction.PrepareAction(c, NewAlignStatusLogsAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).To(Equal(testAction.Return()))
	g.Expect(instance.Status.Logs).To(HaveLen(2))

	// Active log
	g.Expect(instance.Status.Logs[0].Prefix).To(Equal("trusted-artifact-signer"))
	g.Expect(instance.Status.Logs[0].LogId).To(Equal(ptr.To(int64(12345))))
	g.Expect(instance.Status.Logs[0].PrivateKeyRef.Name).To(Equal("keys"))

	// Readonly shard
	g.Expect(instance.Status.Logs[1].Prefix).To(Equal("shard-2024"))
	g.Expect(instance.Status.Logs[1].LogId).To(Equal(ptr.To(int64(99999))))
	g.Expect(instance.Status.Logs[1].PrivateKeyRef.Name).To(Equal("shard-keys"))
	g.Expect(instance.Status.Logs[1].PublicKeyRef.Name).To(Equal("shard-keys"))
	g.Expect(instance.Status.Logs[1].RootCertificates).To(HaveLen(1))
	g.Expect(instance.Status.Logs[1].RootCertificates[0].Name).To(Equal("shard-root"))
}

func TestAlignStatusLogs_SpecOverride(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	instance := &rhtasv1.CTlog{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: rhtasv1.CTlogSpec{
			Logs: []rhtasv1.CTLogConfig{
				{
					Prefix: "trusted-artifact-signer",
					Active: ptr.To(true),
					Signer: &rhtasv1.CTlogSigner{
						Type: "file",
						File: &rhtasv1.CTlogFile{
							PrivateKeyRef: &rhtasv1.SecretKeySelector{
								LocalObjectReference: rhtasv1.LocalObjectReference{Name: "keys"},
								Key:                  "new-key",
							},
						},
					},
					LogId: ptr.To(int64(54321)),
				},
			},
		},
		Status: rhtasv1.CTlogStatus{
			Logs: []rhtasv1.CTlogLogStatus{
				{
					Prefix: "trusted-artifact-signer",
					LogId:  ptr.To(int64(12345)),
					PrivateKeyRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "keys"},
						Key:                  "private",
					},
					PublicKeyRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "keys"},
						Key:                  "public",
					},
					PublicKey: "-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----\n",
					RootCertificates: []rhtasv1.SecretKeySelector{
						{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "root"}, Key: "cert"},
					},
				},
			},
			Conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Reason: state.Initialize.String()},
			},
		},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()
	a := testAction.PrepareAction(c, NewAlignStatusLogsAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).To(Equal(testAction.Return()))
	g.Expect(instance.Status.Logs).To(HaveLen(1))
	g.Expect(instance.Status.Logs[0].Prefix).To(Equal("trusted-artifact-signer"))
	g.Expect(instance.Status.Logs[0].LogId).To(Equal(ptr.To(int64(54321))))
	g.Expect(instance.Status.Logs[0].PrivateKeyRef.Name).To(Equal("keys"))
	g.Expect(instance.Status.Logs[0].PrivateKeyRef.Key).To(Equal("new-key"))
	g.Expect(instance.Status.Logs[0].PublicKeyRef.Name).To(Equal("keys"))
	g.Expect(instance.Status.Logs[0].PublicKey).To(ContainSubstring("PUBLIC KEY"))
	g.Expect(instance.Status.Logs[0].RootCertificates).To(HaveLen(1))
}

func TestAlignStatusLogs_NoChangeSkips(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	instance := &rhtasv1.CTlog{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: rhtasv1.CTlogSpec{
			Logs: []rhtasv1.CTLogConfig{
				{
					Prefix: "trusted-artifact-signer",
					Active: ptr.To(true),
				},
			},
		},
		Status: rhtasv1.CTlogStatus{
			Logs: []rhtasv1.CTlogLogStatus{
				{
					Prefix: "trusted-artifact-signer",
					LogId:  ptr.To(int64(12345)),
					Active: true,
				},
			},
			Conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Reason: state.Initialize.String()},
			},
		},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()
	a := testAction.PrepareAction(c, NewAlignStatusLogsAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).To(Equal(testAction.Continue()))
}

func TestAlignStatusLogs_PreservesEncryptedKeyPassword(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Regression test: encrypted legacy keys preserve password refs during alignment
	// when private key ref remains unchanged. This ensures password migration path
	// doesn't lose the password during status reconciliation.
	instance := &rhtasv1.CTlog{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: rhtasv1.CTlogSpec{
			Logs: []rhtasv1.CTLogConfig{
				{
					Prefix: "trusted-artifact-signer",
					Active: ptr.To(true),
					Signer: &rhtasv1.CTlogSigner{
						Type: "file",
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
					LogId:  ptr.To(int64(12345)),
					PrivateKeyRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "keys"},
						Key:                  "private",
					},
					PublicKeyRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "keys"},
						Key:                  "public",
					},
					PublicKey: "-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----\n",
					// Password ref from legacy encrypted key migration
					PrivateKeyPasswordRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "keys"},
						Key:                  "password",
					},
					RootCertificates: []rhtasv1.SecretKeySelector{
						{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "root"}, Key: "cert"},
					},
				},
			},
			Conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Reason: state.Initialize.String()},
			},
		},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()
	a := testAction.PrepareAction(c, NewAlignStatusLogsAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).To(Equal(testAction.Return()))
	g.Expect(instance.Status.Logs).To(HaveLen(1))
	g.Expect(instance.Status.Logs[0].Prefix).To(Equal("trusted-artifact-signer"))
	g.Expect(instance.Status.Logs[0].LogId).To(Equal(ptr.To(int64(12345))))
	// Private key ref unchanged: password ref should be preserved
	g.Expect(instance.Status.Logs[0].PrivateKeyRef.Name).To(Equal("keys"))
	g.Expect(instance.Status.Logs[0].PrivateKeyRef.Key).To(Equal("private"))
	g.Expect(instance.Status.Logs[0].PrivateKeyPasswordRef).NotTo(BeNil())
	g.Expect(instance.Status.Logs[0].PrivateKeyPasswordRef.Name).To(Equal("keys"))
	g.Expect(instance.Status.Logs[0].PrivateKeyPasswordRef.Key).To(Equal("password"))
}

func TestAlignStatusLogs_RejectsDuplicateLogIds(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Regression test: duplicate logIds would cause secret data corruption
	// since logId is used as part of the secret key name (log-{logId}-root-{idx}).
	instance := &rhtasv1.CTlog{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: rhtasv1.CTlogSpec{
			Logs: []rhtasv1.CTLogConfig{
				{
					Prefix: "trusted-artifact-signer",
					Active: ptr.To(true),
					Signer: &rhtasv1.CTlogSigner{Type: "file"},
				},
				{
					Prefix:   "shard-2024",
					Readonly: ptr.To(true),
					LogId:    ptr.To(int64(99999)),
					Signer: &rhtasv1.CTlogSigner{
						Type: "file",
						File: &rhtasv1.CTlogFile{
							PrivateKeyRef: &rhtasv1.SecretKeySelector{
								LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-keys"},
								Key:                  "private",
							},
						},
					},
					RootCerts: []rhtasv1.SecretKeySelector{
						{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-root"}, Key: "cert"},
					},
				},
				{
					Prefix:   "shard-2025",
					Readonly: ptr.To(true),
					LogId:    ptr.To(int64(99999)), // DUPLICATE!
					Signer: &rhtasv1.CTlogSigner{
						Type: "file",
						File: &rhtasv1.CTlogFile{
							PrivateKeyRef: &rhtasv1.SecretKeySelector{
								LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-keys"},
								Key:                  "private",
							},
						},
					},
					RootCerts: []rhtasv1.SecretKeySelector{
						{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-root"}, Key: "cert"},
					},
				},
			},
		},
		Status: rhtasv1.CTlogStatus{
			Conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Reason: state.Initialize.String()},
			},
		},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()
	a := testAction.PrepareAction(c, NewAlignStatusLogsAction())
	result := a.Handle(ctx, instance)

	// Should return error due to duplicate logIds
	g.Expect(result).NotTo(BeNil())
	g.Expect(result.Err).To(HaveOccurred())
	g.Expect(result.Err.Error()).To(ContainSubstring("duplicate logIds"))
	g.Expect(result.Err.Error()).To(ContainSubstring("99999"))
	// Status should be updated with error condition
	g.Expect(instance.Status.Conditions).NotTo(BeEmpty())
	readyCondition := meta.FindStatusCondition(instance.Status.Conditions, constants.ReadyCondition)
	g.Expect(readyCondition).NotTo(BeNil())
	g.Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(readyCondition.Reason).To(Equal("InvalidLogConfiguration"))
}

func TestAlignStatusLogs_DerivesPublicKeyForFrozenShards(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Regression test: frozen/readonly shards with only private key reference
	// must have public key derived automatically. This ensures non-active file-backed
	// logs can serialize correctly in CTFE config without requiring explicit public key.
	instance := &rhtasv1.CTlog{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: rhtasv1.CTlogSpec{
			Logs: []rhtasv1.CTLogConfig{
				{
					Prefix:   "shard-2024",
					LogId:    ptr.To(int64(99999)),
					Readonly: ptr.To(true),
					Signer: &rhtasv1.CTlogSigner{
						Type: "file",
						File: &rhtasv1.CTlogFile{
							PrivateKeyRef: &rhtasv1.SecretKeySelector{
								LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-keys"},
								Key:                  "private",
							},
							// No explicit public key ref
						},
					},
					RootCerts: []rhtasv1.SecretKeySelector{
						{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-root"}, Key: "cert"},
					},
				},
			},
		},
		Status: rhtasv1.CTlogStatus{
			Logs: []rhtasv1.CTlogLogStatus{
				{
					Prefix: "shard-2024",
					LogId:  ptr.To(int64(99999)),
					PrivateKeyRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-keys"},
						Key:                  "private",
					},
					RootCertificates: []rhtasv1.SecretKeySelector{
						{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-root"}, Key: "cert"},
					},
					// No public key initially
				},
			},
			Conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Reason: state.Initialize.String()},
			},
		},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()
	a := testAction.PrepareAction(c, NewAlignStatusLogsAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).To(Equal(testAction.Return()))
	g.Expect(instance.Status.Logs).To(HaveLen(1))
	g.Expect(instance.Status.Logs[0].Prefix).To(Equal("shard-2024"))
	g.Expect(instance.Status.Logs[0].LogId).To(Equal(ptr.To(int64(99999))))
	g.Expect(instance.Status.Logs[0].PrivateKeyRef.Name).To(Equal("shard-keys"))
	// Public key should be derived from private key reference
	g.Expect(instance.Status.Logs[0].PublicKeyRef).NotTo(BeNil())
	g.Expect(instance.Status.Logs[0].PublicKeyRef.Name).To(Equal("shard-keys"))
	g.Expect(instance.Status.Logs[0].PublicKeyRef.Key).To(Equal("public"))
}
