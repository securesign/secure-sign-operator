package v1alpha1

// Reproduction scenarios for the Qodo findings about v1alpha1 -> v1 conversion
// discarding legacy-API edits.
//
// Both scenarios follow the same real-world flow:
//  1. A v1 (hub) CTlog exists with a sharded Logs array. It is served to a
//     legacy client, which stores the full v1 object in the conversion-data
//     annotation (ConvertFrom -> MarshalData).
//  2. The legacy client edits the deprecated top-level fields (treeID,
//     privateKeyRef, rootCertificates) and writes the object back.
//  3. ConvertTo runs. It must merge those edits into the
//     "trusted-artifact-signer" log while preserving v1-only shards.
//
// Today step 3 blindly overwrites Spec.Logs / Status.Logs with the stored
// copy, so every legacy edit is silently dropped.

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func shardedHubCTlogSpec() rhtasv1.CTlogSpec {
	return rhtasv1.CTlogSpec{
		Logs: []rhtasv1.CTLogConfig{
			{
				Prefix: v1alpha1Prefix,
				Active: ptr.To(true),
				LogId:  ptr.To(int64(111)),
				Signer: &rhtasv1.CTlogSigner{
					Type: rhtasv1.SignerTypeFile,
					File: &rhtasv1.CTlogFile{
						PrivateKeyRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{Name: "old-key"},
							Key:                  "private",
						},
					},
				},
				RootCerts: []rhtasv1.SecretKeySelector{
					{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "old-root"}, Key: "cert"},
				},
			},
			{
				// v1-only frozen shard: must survive the round trip.
				Prefix:   "shard-222",
				LogId:    ptr.To(int64(222)),
				Readonly: ptr.To(true),
				Signer: &rhtasv1.CTlogSigner{
					Type: rhtasv1.SignerTypeFile,
					File: &rhtasv1.CTlogFile{
						PrivateKeyRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-key"},
							Key:                  "private",
						},
					},
				},
				RootCerts: []rhtasv1.SecretKeySelector{
					{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "shard-root"}, Key: "cert"},
				},
			},
		},
	}
}

func shardedHubCTlogStatus() rhtasv1.CTlogStatus {
	return rhtasv1.CTlogStatus{
		Logs: []rhtasv1.CTlogLogStatus{
			{
				Prefix: v1alpha1Prefix,
				Active: true,
				LogId:  ptr.To(int64(111)),
				PrivateKeyRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{Name: "old-key"},
					Key:                  "private",
				},
				RootCertificates: []rhtasv1.SecretKeySelector{
					{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "old-root"}, Key: "cert"},
				},
			},
			{
				Prefix: "shard-222",
				LogId:  ptr.To(int64(222)),
			},
		},
	}
}

func findHubLog(logs []rhtasv1.CTLogConfig, prefix string) *rhtasv1.CTLogConfig {
	for i := range logs {
		if logs[i].Prefix == prefix {
			return &logs[i]
		}
	}
	return nil
}

func findHubLogStatus(logs []rhtasv1.CTlogLogStatus, prefix string) *rhtasv1.CTlogLogStatus {
	for i := range logs {
		if logs[i].Prefix == prefix {
			return &logs[i]
		}
	}
	return nil
}

// Qodo finding #1: CTlog.ConvertTo restores the stored Spec.Logs wholesale and
// throws away the legacy edits that Convert_v1alpha1_CTlogSpec_To_v1_CTlogSpec
// just merged into the "trusted-artifact-signer" entry.
func TestQodo_CTlogConvertTo_DiscardsLegacySpecEdits(t *testing.T) {
	g := NewWithT(t)

	hub := &rhtasv1.CTlog{
		ObjectMeta: metav1.ObjectMeta{Name: "ctlog", Namespace: "default"},
		Spec:       shardedHubCTlogSpec(),
	}

	// Step 1: hub -> spoke (stores the v1 object in the conversion annotation).
	spoke := &CTlog{}
	g.Expect(spoke.ConvertFrom(hub)).To(Succeed())
	g.Expect(spoke.Spec.TreeID).To(HaveValue(Equal(int64(111))), "sanity: legacy view of the active log")

	// Step 2: legacy client edits the deprecated fields.
	spoke.Spec.TreeID = ptr.To(int64(999))
	spoke.Spec.PrivateKeyRef = &SecretKeySelector{
		LocalObjectReference: LocalObjectReference{Name: "new-key"},
		Key:                  "private",
	}
	spoke.Spec.RootCertificates = []SecretKeySelector{
		{LocalObjectReference: LocalObjectReference{Name: "new-root"}, Key: "cert"},
	}

	// Step 3: spoke -> hub.
	out := &rhtasv1.CTlog{}
	g.Expect(spoke.ConvertTo(out)).To(Succeed())

	// The v1-only shard must be preserved (this part works today).
	g.Expect(findHubLog(out.Spec.Logs, "shard-222")).ToNot(BeNil(), "v1-only frozen shard was dropped")

	active := findHubLog(out.Spec.Logs, v1alpha1Prefix)
	g.Expect(active).ToNot(BeNil())

	// These are the edits the legacy client made. All three are lost because
	// ConvertTo does `dst.Spec.Logs = restored.Spec.Logs`.
	g.Expect(active.LogId).To(HaveValue(Equal(int64(999))),
		"spec.treeID edit made through v1alpha1 was overwritten by the stored Logs slice")
	g.Expect(active.Signer).ToNot(BeNil())
	g.Expect(active.Signer.File).ToNot(BeNil())
	g.Expect(active.Signer.File.PrivateKeyRef).ToNot(BeNil())
	g.Expect(active.Signer.File.PrivateKeyRef.Name).To(Equal("new-key"),
		"spec.privateKeyRef edit made through v1alpha1 was overwritten by the stored Logs slice")
	g.Expect(active.RootCerts).To(HaveLen(1))
	g.Expect(active.RootCerts[0].Name).To(Equal("new-root"),
		"spec.rootCertificates edit made through v1alpha1 was overwritten by the stored Logs slice")
}

// Qodo finding #1 (status half): the same wholesale restore applies to
// Status.Logs, so deprecated status edits are dropped too.
func TestQodo_CTlogConvertTo_DiscardsLegacyStatusEdits(t *testing.T) {
	g := NewWithT(t)

	hub := &rhtasv1.CTlog{
		ObjectMeta: metav1.ObjectMeta{Name: "ctlog", Namespace: "default"},
		Spec:       shardedHubCTlogSpec(),
		Status:     shardedHubCTlogStatus(),
	}

	spoke := &CTlog{}
	g.Expect(spoke.ConvertFrom(hub)).To(Succeed())
	g.Expect(spoke.Status.TreeID).To(HaveValue(Equal(int64(111))), "sanity")

	spoke.Status.TreeID = ptr.To(int64(999))
	spoke.Status.PrivateKeyRef = &SecretKeySelector{
		LocalObjectReference: LocalObjectReference{Name: "new-key"},
		Key:                  "private",
	}

	out := &rhtasv1.CTlog{}
	g.Expect(spoke.ConvertTo(out)).To(Succeed())

	g.Expect(findHubLogStatus(out.Status.Logs, "shard-222")).ToNot(BeNil(), "v1-only shard status was dropped")

	active := findHubLogStatus(out.Status.Logs, v1alpha1Prefix)
	g.Expect(active).ToNot(BeNil())
	g.Expect(active.LogId).To(HaveValue(Equal(int64(999))),
		"status.treeID edit made through v1alpha1 was overwritten by `dst.Status.Logs = restored.Status.Logs`")
	g.Expect(active.PrivateKeyRef).ToNot(BeNil())
	g.Expect(active.PrivateKeyRef.Name).To(Equal("new-key"),
		"status.privateKeyRef edit made through v1alpha1 was overwritten by the stored Logs slice")
}

// Qodo finding #2: Securesign.ConvertTo has the same defect for the embedded
// CTlog spec — `dst.Spec.Ctlog.Logs = restored.Spec.Ctlog.Logs`.
func TestQodo_SecuresignConvertTo_DiscardsLegacyCtlogEdits(t *testing.T) {
	g := NewWithT(t)

	hub := &rhtasv1.Securesign{
		ObjectMeta: metav1.ObjectMeta{Name: "securesign", Namespace: "default"},
		Spec: rhtasv1.SecuresignSpec{
			Ctlog:  shardedHubCTlogSpec(),
			Fulcio: rhtasv1.FulcioSpec{Signer: rhtasv1.FulcioSigner{Type: "file"}},
		},
	}

	spoke := &Securesign{}
	g.Expect(spoke.ConvertFrom(hub)).To(Succeed())
	g.Expect(spoke.Spec.Ctlog.TreeID).To(HaveValue(Equal(int64(111))), "sanity")

	spoke.Spec.Ctlog.TreeID = ptr.To(int64(999))
	spoke.Spec.Ctlog.PrivateKeyRef = &SecretKeySelector{
		LocalObjectReference: LocalObjectReference{Name: "new-key"},
		Key:                  "private",
	}
	spoke.Spec.Ctlog.RootCertificates = []SecretKeySelector{
		{LocalObjectReference: LocalObjectReference{Name: "new-root"}, Key: "cert"},
	}

	out := &rhtasv1.Securesign{}
	g.Expect(spoke.ConvertTo(out)).To(Succeed())

	g.Expect(findHubLog(out.Spec.Ctlog.Logs, "shard-222")).ToNot(BeNil(), "v1-only frozen shard was dropped")

	active := findHubLog(out.Spec.Ctlog.Logs, v1alpha1Prefix)
	g.Expect(active).ToNot(BeNil())
	g.Expect(active.LogId).To(HaveValue(Equal(int64(999))),
		"spec.ctlog.treeID edit made through v1alpha1 was overwritten by the stored Logs slice")
	g.Expect(active.Signer).ToNot(BeNil())
	g.Expect(active.Signer.File).ToNot(BeNil())
	g.Expect(active.Signer.File.PrivateKeyRef).ToNot(BeNil())
	g.Expect(active.Signer.File.PrivateKeyRef.Name).To(Equal("new-key"),
		"spec.ctlog.privateKeyRef edit made through v1alpha1 was overwritten by the stored Logs slice")
	g.Expect(active.RootCerts).To(HaveLen(1))
	g.Expect(active.RootCerts[0].Name).To(Equal("new-root"),
		"spec.ctlog.rootCertificates edit made through v1alpha1 was overwritten by the stored Logs slice")
}
