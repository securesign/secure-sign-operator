package actions

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
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
	g.Expect(instance.Status.Logs[0].PrivateKeyRef.Name).To(Equal("keys-new"))
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
