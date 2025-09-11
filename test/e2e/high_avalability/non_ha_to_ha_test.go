//go:build ha

package ha

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/securesign/operator/api/v1alpha1"
	ctlogactions "github.com/securesign/operator/internal/controller/ctlog/actions"
	fulcioactions "github.com/securesign/operator/internal/controller/fulcio/actions"
	rekoractions "github.com/securesign/operator/internal/controller/rekor/actions"
	trillianactions "github.com/securesign/operator/internal/controller/trillian/actions"
	tsaactions "github.com/securesign/operator/internal/controller/tsa/actions"
	"github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/kubernetes"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/ctlog"
	"github.com/securesign/operator/test/e2e/support/tas/fulcio"
	"github.com/securesign/operator/test/e2e/support/tas/rekor"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	"github.com/securesign/operator/test/e2e/support/tas/trillian"
	"github.com/securesign/operator/test/e2e/support/tas/tsa"
	"github.com/securesign/operator/test/e2e/support/tas/tuf"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

var _ = Describe("Securesign install with certificate generation", Ordered, func() {

	cli, err := support.CreateClient()
	Expect(err).ToNot(HaveOccurred())

	var targetImageName string
	var namespace *v1.Namespace
	var s *v1alpha1.Securesign

	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))

	BeforeAll(func(ctx SpecContext) {
		s = securesign.Create(namespace.Name, "test",
			securesign.WithDefaults(),
			securesign.WithSearchUI(),
			securesign.WithMonitoring(),
			func(v *v1alpha1.Securesign) {
				v.Spec.Rekor.Attestations.Enabled = ptr.To(true)
			},
		)
	})

	BeforeAll(func(ctx SpecContext) {
		targetImageName = support.PrepareImage(ctx)
	})

	Describe("Non HA to HA test", func() {
		replicas := ptr.To(int32(2))
		newRekorPVCName := "nfs-rekor"
		newTufPVCName := "nfs-tuf"

		BeforeAll(func(ctx SpecContext) {
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("All other components are running", func(ctx SpecContext) {
			tas.VerifyAllComponents(ctx, cli, s, true)
		})

		It("Use cosign cli", func(ctx SpecContext) {
			tas.VerifyByCosign(ctx, cli, s, targetImageName)
		})

		It("migrates from non-HA to HA by copying and reconfiguring PVCs", func(ctx SpecContext) {
			// scale tuf to 0
			tuf.SetTufReplicaCount(ctx, cli, namespace.Name, s.Name, 0)

			// scale rekor to 0
			rekor.SetRekorReplicaCount(ctx, cli, namespace.Name, s.Name, 0)

			// create new persistent volume claims for rekor and tuf
			for _, pvcName := range []string{newRekorPVCName, newTufPVCName} {
				pvc := kubernetes.CreateTestPVC(pvcName, namespace.Name)
				err := cli.Create(ctx, pvc)
				Expect(err).ToNot(HaveOccurred())
			}

			// create two pvc copy jobs for rekor and tuf pvcs
			rrekor := rekor.Get(ctx, cli, namespace.Name, s.Name)
			Expect(rrekor).ToNot(BeNil())
			rekorPVCName := rrekor.Status.PvcName

			ttuf := tuf.Get(ctx, cli, namespace.Name, s.Name)
			Expect(ttuf).ToNot(BeNil())
			tufPVCName := ttuf.Status.PvcName

			for k, v := range map[string]string{rekorPVCName: newRekorPVCName, tufPVCName: newTufPVCName} {
				job := kubernetes.CreatePVCCopyJob(namespace.Name, k, v)
				err := cli.Create(ctx, job)
				Expect(err).ToNot(HaveOccurred())

				Eventually(func(g Gomega, ctx SpecContext) {
					j := &batchv1.Job{}
					g.Expect(cli.Get(ctx, client.ObjectKey{Name: job.Name, Namespace: namespace.Name}, j)).To(Succeed())
					g.Expect(j.Status.Succeeded).To(BeNumerically(">", 0))
				}).WithContext(ctx).Should(Succeed())
			}

			// set claim name, pvc config and scale tuf to 1
			Eventually(func(g Gomega, ctx SpecContext) {
				s := securesign.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(s).ToNot(BeNil())

				s.Spec.Tuf.Pvc.Name = newTufPVCName
				s.Spec.Tuf.Pvc.Retain = ptr.To(true)
				s.Spec.Tuf.Pvc.Size = ptr.To(resource.MustParse("100Mi"))
				s.Spec.Tuf.Pvc.AccessModes = []v1alpha1.PersistentVolumeAccessMode{"ReadWriteMany"}
				s.Spec.Tuf.Pvc.StorageClass = "nfs-csi"

				err := cli.Update(ctx, s)
				g.Expect(err).ToNot(HaveOccurred())

				tuf.SetTufReplicaCount(ctx, cli, namespace.Name, s.Name, 1)
			}).WithContext(ctx).Should(Succeed())

			// set claim name, pvc config and scale rekor to 1
			Eventually(func(g Gomega, ctx SpecContext) {
				s := securesign.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(s).ToNot(BeNil())

				s.Spec.Rekor.Pvc.Name = newRekorPVCName
				s.Spec.Rekor.Pvc.Retain = ptr.To(true)
				s.Spec.Rekor.Pvc.Size = ptr.To(resource.MustParse("100Mi"))
				s.Spec.Rekor.Pvc.AccessModes = []v1alpha1.PersistentVolumeAccessMode{"ReadWriteMany"}
				s.Spec.Rekor.Pvc.StorageClass = "nfs-csi"

				err := cli.Update(ctx, s)
				g.Expect(err).ToNot(HaveOccurred())

				rekor.SetRekorReplicaCount(ctx, cli, namespace.Name, s.Name, 1)
			}).WithContext(ctx).Should(Succeed())

		})

		It("should verify HA", func(ctx SpecContext) {

			s := securesign.Get(ctx, cli, namespace.Name, s.Name)
			Expect(s).ToNot(BeNil())

			securesign.WithReplicas(ptr.To(int32(2)))(s)
			err := cli.Update(ctx, s)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(f.Spec.Replicas).To(gstruct.PointTo(BeNumerically(">=", *replicas)), "fulcio should have more than one replica")
			}).WithContext(ctx).Should(Succeed())

			Eventually(func(g Gomega) {
				r := rekor.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(r.Spec.Replicas).To(gstruct.PointTo(BeNumerically(">=", *replicas)), "rekor should have more than one replica")
				g.Expect(r.Spec.RekorSearchUI.Replicas).To(gstruct.PointTo(BeNumerically(">=", *replicas)), "rekor search ui should have more than one replica")
			}).WithContext(ctx).Should(Succeed())

			Eventually(func(g Gomega) {
				c := ctlog.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(c.Spec.Replicas).To(gstruct.PointTo(BeNumerically(">=", *replicas)), "ctlog should have more than one replica")
			}).WithContext(ctx).Should(Succeed())

			Eventually(func(g Gomega) {
				t := tsa.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(t.Spec.Replicas).To(gstruct.PointTo(BeNumerically(">=", *replicas)), "tsa should have more than one replica")
			}).WithContext(ctx).Should(Succeed())

			Eventually(func(g Gomega) {
				t := tuf.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(t.Spec.Replicas).To(gstruct.PointTo(BeNumerically(">=", *replicas)), "tuf should have more than one replica")
			}).WithContext(ctx).Should(Succeed())

			Eventually(func(g Gomega) {
				tr := trillian.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(tr.Spec.LogServer.Replicas).To(gstruct.PointTo(BeNumerically(">=", *replicas)), "log server should have more than one replica")
				g.Expect(tr.Spec.LogSigner.Replicas).To(gstruct.PointTo(BeNumerically(">=", *replicas)), "log signer should have more than one replica")
			}).WithContext(ctx).Should(Succeed())

			endpointNames := []string{
				ctlogactions.ComponentName,
				fulcioactions.DeploymentName,
				rekoractions.SearchUiDeploymentName,
				rekoractions.ServerComponentName,
				trillianactions.LogServerComponentName,
				trillianactions.LogSignerComponentName,
				tsaactions.DeploymentName,
				constants.ComponentName,
			}
			for _, endpointName := range endpointNames {
				Eventually(kubernetes.ExpectServiceHasAtLeastNReadyEndpoints).
					WithContext(ctx).
					WithArguments(cli, namespace.Name, endpointName, 2).
					Should(Succeed(), "expected service to have n ready endpoints")
			}

			tas.VerifyAllComponents(ctx, cli, s, true)
		})

		It("Use cosign cli", func(ctx SpecContext) {
			tas.VerifyByCosign(ctx, cli, s, targetImageName)
		})
	})
})
