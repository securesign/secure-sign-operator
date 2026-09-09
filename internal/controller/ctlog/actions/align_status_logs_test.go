package actions

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func keyRef(name, key string) *rhtasv1.SecretKeySelector {
	return &rhtasv1.SecretKeySelector{
		LocalObjectReference: rhtasv1.LocalObjectReference{Name: name},
		Key:                  key,
	}
}

func TestAlignStatusLogs_CanHandle(t *testing.T) {
	tests := []struct {
		name string
		state.State
		want bool
	}{
		{name: "pending", State: state.Pending},
		{name: "creating", State: state.Creating, want: true},
		{name: "initialize", State: state.Initialize, want: true},
		{name: "ready", State: state.Ready, want: true},
	}

	a := NewAlignStatusLogsAction()
	NewWithT(t).Expect(a.Name()).To(Equal("align-status-logs"))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &rhtasv1.CTlog{Status: rhtasv1.CTlogStatus{
				Conditions: []metav1.Condition{{Type: constants.ReadyCondition, Reason: tt.String()}},
			}}
			NewWithT(t).Expect(a.CanHandle(t.Context(), instance)).To(Equal(tt.want))
		})
	}
}

func TestAlignStatusLogs_Handle(t *testing.T) {
	type want struct {
		result          *action.Result
		err             string
		logs            []rhtasv1.CTlogLogStatus
		unchanged       bool
		storedUnchanged bool
		verify          func(Gomega, *rhtasv1.CTlog)
	}

	tests := []struct {
		name     string
		instance *rhtasv1.CTlog
		want     want
	}{
		{
			name: "reject empty spec without mutation",
			instance: &rhtasv1.CTlog{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Status: rhtasv1.CTlogStatus{Logs: []rhtasv1.CTlogLogStatus{{
					Prefix:        "trusted-artifact-signer",
					Active:        true,
					LogId:         ptr.To(int64(123456)),
					PrivateKeyRef: keyRef("ctlog-keys-test", "private"),
					PublicKeyRef:  keyRef("ctlog-keys-test", "public"),
				}}},
			},
			want: want{
				err:             "at least one log is required",
				unchanged:       true,
				storedUnchanged: true,
			},
		},
		{
			name: "continue when status is aligned",
			instance: &rhtasv1.CTlog{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: rhtasv1.CTlogSpec{Logs: []rhtasv1.CTLogConfig{{
					Prefix: "trusted-artifact-signer",
					Active: ptr.To(true),
				}}},
				Status: rhtasv1.CTlogStatus{Logs: []rhtasv1.CTlogLogStatus{{
					Prefix: "trusted-artifact-signer",
					Active: true,
					LogId:  ptr.To(int64(12345)),
				}}},
			},
			want: want{
				result:          testAction.Continue(),
				unchanged:       true,
				storedUnchanged: true,
			},
		},
		{
			name: "align multiple logs in spec order and remove stale status",
			instance: &rhtasv1.CTlog{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: rhtasv1.CTlogSpec{Logs: []rhtasv1.CTLogConfig{
					{Prefix: "active", Active: ptr.To(true)},
					{Prefix: "new", LogId: ptr.To(int64(2)), Active: ptr.To(false)},
				}},
				Status: rhtasv1.CTlogStatus{Logs: []rhtasv1.CTlogLogStatus{
					{Prefix: "removed", LogId: ptr.To(int64(3))},
					{Prefix: "active", LogId: ptr.To(int64(1)), PublicKey: "public-key"},
				}},
			},
			want: want{
				result: testAction.Return(),
				logs: []rhtasv1.CTlogLogStatus{
					{Prefix: "active", Active: true, LogId: ptr.To(int64(1)), PublicKey: "public-key"},
					{Prefix: "new", LogId: ptr.To(int64(2))},
				},
			},
		},
		{
			name: "reject duplicate log IDs without persisting",
			instance: &rhtasv1.CTlog{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: rhtasv1.CTlogSpec{Logs: []rhtasv1.CTLogConfig{
					{Prefix: "first", LogId: ptr.To(int64(7))},
					{Prefix: "second", LogId: ptr.To(int64(7))},
				}},
			},
			want: want{
				err:             "duplicate logIds",
				storedUnchanged: true,
				verify: func(g Gomega, instance *rhtasv1.CTlog) {
					condition := meta.FindStatusCondition(instance.Status.Conditions, constants.ReadyCondition)
					g.Expect(condition).NotTo(BeNil())
					g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
					g.Expect(condition.Reason).To(Equal("InvalidLogConfiguration"))
					g.Expect(condition.Message).To(ContainSubstring("first"))
					g.Expect(condition.Message).To(ContainSubstring("second"))
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := t.Context()
			c := testAction.FakeClientBuilder().
				WithObjects(tt.instance).
				WithStatusSubresource(tt.instance).
				Build()
			key := client.ObjectKeyFromObject(tt.instance)
			storedBefore := &rhtasv1.CTlog{}
			g.Expect(c.Get(ctx, key, storedBefore)).To(Succeed())
			before := tt.instance.DeepCopy()

			result := testAction.PrepareAction(c, NewAlignStatusLogsAction()).Handle(ctx, tt.instance)
			if tt.want.err != "" {
				g.Expect(result).NotTo(BeNil())
				g.Expect(result.Err).To(MatchError(ContainSubstring(tt.want.err)))
			} else {
				g.Expect(result).To(Equal(tt.want.result))
			}
			if tt.want.unchanged {
				g.Expect(tt.instance).To(Equal(before))
			} else if tt.want.logs != nil {
				g.Expect(tt.instance.Status.Logs).To(Equal(tt.want.logs))
			}
			if tt.want.verify != nil {
				tt.want.verify(g, tt.instance)
			}

			storedAfter := &rhtasv1.CTlog{}
			g.Expect(c.Get(ctx, key, storedAfter)).To(Succeed())
			if tt.want.storedUnchanged {
				g.Expect(storedAfter).To(Equal(storedBefore))
			} else {
				g.Expect(storedAfter.Status.Logs).To(Equal(tt.want.logs))
			}
		})
	}
}

func TestBuildStatusLogs(t *testing.T) {
	tests := []struct {
		name     string
		spec     []rhtasv1.CTLogConfig
		status   []rhtasv1.CTlogLogStatus
		expected []rhtasv1.CTlogLogStatus
	}{
		{
			name:     "new log defaults inactive",
			spec:     []rhtasv1.CTLogConfig{{Prefix: "new"}},
			expected: []rhtasv1.CTlogLogStatus{{Prefix: "new"}},
		},
		{
			name: "preserve existing status fields",
			spec: []rhtasv1.CTLogConfig{{Prefix: "log", Active: ptr.To(true)}},
			status: []rhtasv1.CTlogLogStatus{{
				Prefix:                "log",
				LogId:                 ptr.To(int64(1)),
				PublicKey:             "public-key",
				PrivateKeyRef:         keyRef("keys", "private"),
				PublicKeyRef:          keyRef("keys", "public"),
				RootCertificates:      []rhtasv1.SecretKeySelector{*keyRef("root", "cert")},
				SignerType:            rhtasv1.SignerTypePKCS11,
				PrivateKeyPasswordRef: keyRef("keys", "password"),
			}},
			expected: []rhtasv1.CTlogLogStatus{{
				Prefix:                "log",
				Active:                true,
				LogId:                 ptr.To(int64(1)),
				PublicKey:             "public-key",
				PrivateKeyRef:         keyRef("keys", "private"),
				PublicKeyRef:          keyRef("keys", "public"),
				RootCertificates:      []rhtasv1.SecretKeySelector{*keyRef("root", "cert")},
				SignerType:            rhtasv1.SignerTypePKCS11,
				PrivateKeyPasswordRef: keyRef("keys", "password"),
			}},
		},
		{
			name: "spec overrides status",
			spec: []rhtasv1.CTLogConfig{{
				Prefix:    "log",
				LogId:     ptr.To(int64(2)),
				RootCerts: []rhtasv1.SecretKeySelector{*keyRef("new-root", "cert")},
				Signer: &rhtasv1.CTlogSigner{
					Type: rhtasv1.SignerTypeFile,
					File: &rhtasv1.CTlogFile{
						PrivateKeyRef: keyRef("new-keys", "private"),
						PublicKeyRef:  keyRef("new-keys", "public"),
					},
				},
			}},
			status: []rhtasv1.CTlogLogStatus{{
				Prefix:                "log",
				LogId:                 ptr.To(int64(1)),
				PublicKey:             "public-key",
				PrivateKeyRef:         keyRef("old-keys", "private"),
				PublicKeyRef:          keyRef("old-keys", "public"),
				RootCertificates:      []rhtasv1.SecretKeySelector{*keyRef("old-root", "cert")},
				SignerType:            rhtasv1.SignerTypePKCS11,
				PrivateKeyPasswordRef: keyRef("old-keys", "password"),
			}},
			expected: []rhtasv1.CTlogLogStatus{{
				Prefix:           "log",
				LogId:            ptr.To(int64(2)),
				PublicKey:        "public-key",
				PrivateKeyRef:    keyRef("new-keys", "private"),
				PublicKeyRef:     keyRef("new-keys", "public"),
				RootCertificates: []rhtasv1.SecretKeySelector{*keyRef("new-root", "cert")},
				SignerType:       rhtasv1.SignerTypeFile,
			}},
		},
		{
			name: "unchanged file key preserves password and derives public key",
			spec: []rhtasv1.CTLogConfig{{
				Prefix: "log",
				Signer: &rhtasv1.CTlogSigner{Type: rhtasv1.SignerTypeFile, File: &rhtasv1.CTlogFile{
					PrivateKeyRef: keyRef("keys", "private"),
				}},
			}},
			status: []rhtasv1.CTlogLogStatus{{
				Prefix:                "log",
				PrivateKeyRef:         keyRef("keys", "private"),
				PrivateKeyPasswordRef: keyRef("keys", "password"),
			}},
			expected: []rhtasv1.CTlogLogStatus{{
				Prefix:                "log",
				PrivateKeyRef:         keyRef("keys", "private"),
				PublicKeyRef:          keyRef("keys", "public"),
				SignerType:            rhtasv1.SignerTypeFile,
				PrivateKeyPasswordRef: keyRef("keys", "password"),
			}},
		},
		{
			name: "new file key clears password and derives public key",
			spec: []rhtasv1.CTLogConfig{{
				Prefix: "log",
				Signer: &rhtasv1.CTlogSigner{Type: rhtasv1.SignerTypeFile, File: &rhtasv1.CTlogFile{
					PrivateKeyRef: keyRef("keys", "private"),
				}},
			}},
			status: []rhtasv1.CTlogLogStatus{{
				Prefix:                "log",
				PrivateKeyPasswordRef: keyRef("old-keys", "password"),
			}},
			expected: []rhtasv1.CTlogLogStatus{{
				Prefix:        "log",
				PrivateKeyRef: keyRef("keys", "private"),
				PublicKeyRef:  keyRef("keys", "public"),
				SignerType:    rhtasv1.SignerTypeFile,
			}},
		},
		{
			name: "changed file key clears password",
			spec: []rhtasv1.CTLogConfig{{
				Prefix: "log",
				Signer: &rhtasv1.CTlogSigner{Type: rhtasv1.SignerTypeFile, File: &rhtasv1.CTlogFile{
					PrivateKeyRef: keyRef("keys", "new-private"),
				}},
			}},
			status: []rhtasv1.CTlogLogStatus{{
				Prefix:                "log",
				PrivateKeyRef:         keyRef("keys", "old-private"),
				PrivateKeyPasswordRef: keyRef("keys", "password"),
			}},
			expected: []rhtasv1.CTlogLogStatus{{
				Prefix:        "log",
				PrivateKeyRef: keyRef("keys", "new-private"),
				PublicKeyRef:  keyRef("keys", "public"),
				SignerType:    rhtasv1.SignerTypeFile,
			}},
		},
		{
			name: "file signer derives public key from existing private key",
			spec: []rhtasv1.CTLogConfig{{
				Prefix: "log",
				Signer: &rhtasv1.CTlogSigner{Type: rhtasv1.SignerTypeFile, File: &rhtasv1.CTlogFile{}},
			}},
			status: []rhtasv1.CTlogLogStatus{{
				Prefix:                "log",
				PrivateKeyRef:         keyRef("keys", "private"),
				PrivateKeyPasswordRef: keyRef("keys", "password"),
			}},
			expected: []rhtasv1.CTlogLogStatus{{
				Prefix:                "log",
				PrivateKeyRef:         keyRef("keys", "private"),
				PublicKeyRef:          keyRef("keys", "public"),
				SignerType:            rhtasv1.SignerTypeFile,
				PrivateKeyPasswordRef: keyRef("keys", "password"),
			}},
		},
		{
			name: "PKCS11 public key overrides status and clears password",
			spec: []rhtasv1.CTLogConfig{{
				Prefix: "log",
				Signer: &rhtasv1.CTlogSigner{
					Type:   rhtasv1.SignerTypePKCS11,
					PKCS11: &rhtasv1.CTlogPKCS11Config{PublicKeyRef: keyRef("hsm", "public")},
				},
			}},
			status: []rhtasv1.CTlogLogStatus{{
				Prefix:                "log",
				PrivateKeyRef:         keyRef("keys", "private"),
				PublicKeyRef:          keyRef("keys", "public"),
				SignerType:            rhtasv1.SignerTypeFile,
				PrivateKeyPasswordRef: keyRef("keys", "password"),
			}},
			expected: []rhtasv1.CTlogLogStatus{{
				Prefix:        "log",
				PrivateKeyRef: keyRef("keys", "private"),
				PublicKeyRef:  keyRef("hsm", "public"),
				SignerType:    rhtasv1.SignerTypePKCS11,
			}},
		},
		{
			name: "PKCS11 without public key preserves status public key and clears password",
			spec: []rhtasv1.CTLogConfig{{
				Prefix: "log",
				Signer: &rhtasv1.CTlogSigner{
					Type:   rhtasv1.SignerTypePKCS11,
					PKCS11: &rhtasv1.CTlogPKCS11Config{},
				},
			}},
			status: []rhtasv1.CTlogLogStatus{{
				Prefix:                "log",
				PublicKeyRef:          keyRef("keys", "public"),
				PrivateKeyPasswordRef: keyRef("keys", "password"),
			}},
			expected: []rhtasv1.CTlogLogStatus{{
				Prefix:       "log",
				PublicKeyRef: keyRef("keys", "public"),
				SignerType:   rhtasv1.SignerTypePKCS11,
			}},
		},
		{
			name: "multiple logs follow spec order and stale status is removed",
			spec: []rhtasv1.CTLogConfig{
				{Prefix: "second", Active: ptr.To(true)},
				{Prefix: "new"},
				{Prefix: "first", Active: ptr.To(false)},
			},
			status: []rhtasv1.CTlogLogStatus{
				{Prefix: "first", LogId: ptr.To(int64(1))},
				{Prefix: "removed", LogId: ptr.To(int64(3))},
				{Prefix: "second", LogId: ptr.To(int64(2))},
			},
			expected: []rhtasv1.CTlogLogStatus{
				{Prefix: "second", Active: true, LogId: ptr.To(int64(2))},
				{Prefix: "new"},
				{Prefix: "first", LogId: ptr.To(int64(1))},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &rhtasv1.CTlog{
				Spec:   rhtasv1.CTlogSpec{Logs: tt.spec},
				Status: rhtasv1.CTlogStatus{Logs: tt.status},
			}
			NewWithT(t).Expect(buildStatusLogs(instance)).To(Equal(tt.expected))
		})
	}
}
