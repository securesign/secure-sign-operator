//go:build integration

package e2e

import (
	"context"

	"github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/job"
	testSupportKubernetes "github.com/securesign/operator/test/e2e/support/kubernetes"
	"github.com/securesign/operator/test/e2e/support/tas/tuf"
	v3 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	apilabels "k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/test/e2e/support"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	tufPvcName = "tuf-init-test"
)

var _ = Describe("Securesign tuf-repository init test", Ordered, func() {
	cli, _ := support.CreateClient()
	ctx := context.TODO()

	var namespace *v1.Namespace
	var s *v1alpha1.Tuf

	AfterEach(func() {
		if CurrentSpecReport().Failed() && support.IsCIEnvironment() {
			support.DumpNamespace(ctx, cli, namespace.Name)
		}
	})

	BeforeAll(func() {
		namespace = support.CreateTestNamespace(ctx, cli)
		DeferCleanup(func() {
			_ = cli.Delete(ctx, namespace)
		})
		s = createInstance("test", namespace.Name)
	})

	Describe("Install with empty pre-created tuf volume", func() {
		It("create mock secret", func() {
			pub, priv, crt, err := support.CreateCertificates(false)
			Expect(err).NotTo(HaveOccurred())
			Expect(cli.Create(ctx, &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: namespace.Name,
				},
				Data: map[string][]byte{"public": pub, "private": priv, "cert": crt},
			})).To(Succeed())
		})

		It("Create tuf PVC and Tuf resource", func() {
			Expect(cli.Create(ctx, &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tufPvcName,
					Namespace: namespace.Name,
				},
				Spec: v1.PersistentVolumeClaimSpec{
					AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					Resources: v1.VolumeResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			})).To(Succeed())
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("Tuf is running", func() {
			tuf.Verify(ctx, cli, namespace.Name, s.Name)
		})

		It("Tuf repository was initialized", func() {
			verifyInitJob(ctx, cli, namespace.Name, s.Name, "Initializing empty repository")
		})

		It("Delete Tuf", func() {
			name := s.Name
			Expect(cli.Delete(ctx, s)).To(Succeed())
			Eventually(cli.Get).WithArguments(ctx, client.ObjectKey{Name: name, Namespace: s.Namespace}, s).ShouldNot(Succeed())

			Eventually(func(g Gomega) []v1.PersistentVolumeClaim {
				pvcList := &v1.PersistentVolumeClaimList{}
				g.Expect(cli.List(ctx, pvcList, client.InNamespace(namespace.Name))).To(Succeed())
				return pvcList.Items
			}).Should(HaveLen(1), "Tuf PVC retained")

			Eventually(func(g Gomega) []v3.Job {
				jobLabels := labels.ForResource(constants.ComponentName, constants.InitJobName, name, tufPvcName)
				initJobList := &v3.JobList{}
				selector := apilabels.SelectorFromSet(jobLabels)
				g.Expect(kubernetes.FindByLabelSelector(ctx, cli, initJobList, namespace.Name, selector.String())).To(Succeed())
				return initJobList.Items
			}).Should(BeEmpty())
		})

		It("Create Tuf with already initialized repo", func() {
			s = createInstance("test1", namespace.Name)
			e := cli.Create(ctx, s)
			Expect(e).To(Succeed())
			tuf.Verify(ctx, cli, namespace.Name, s.Name)
			verifyInitJob(ctx, cli, namespace.Name, s.Name, "Repo seems to already be initialized")
		})

	})
})

func createInstance(name, ns string) *v1alpha1.Tuf {
	return &v1alpha1.Tuf{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
		Spec: v1alpha1.TufSpec{
			Keys: []v1alpha1.TufKey{
				{
					Name: "rekor.pub",
					SecretRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "test",
						},
						Key: "public",
					},
				},
				{
					Name: "ctfe.pub",
					SecretRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "test",
						},
						Key: "public",
					},
				},
				{
					Name: "fulcio_v1.crt.pem",
					SecretRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "test",
						},
						Key: "cert",
					},
				},
				{
					Name: "tsa.certchain.pem",
					SecretRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "test",
						},
						Key: "cert",
					},
				},
			},
			Pvc: v1alpha1.TufPvc{
				Name: tufPvcName,
			},
		},
	}
}

func verifyInitJob(ctx context.Context, cli client.Client, ns, name, logOutput string) {
	jobLabels := labels.ForResource(constants.ComponentName, constants.InitJobName, name, tufPvcName)
	initJobList := &v3.JobList{}
	selector := apilabels.SelectorFromSet(jobLabels)
	Expect(kubernetes.FindByLabelSelector(ctx, cli, initJobList, ns, selector.String())).To(Succeed())

	Expect(initJobList.Items).To(HaveLen(1))
	Expect(job.IsCompleted(initJobList.Items[0])).To(BeTrue())
	Expect(job.IsFailed(initJobList.Items[0])).To(BeFalse())

	podList := &v1.PodList{}
	Expect(kubernetes.FindByLabelSelector(ctx, cli, podList, ns, "job-name = "+initJobList.Items[0].Name)).To(Succeed())
	Expect(podList.Items).To(HaveLen(1))

	logs, err := testSupportKubernetes.GetPodLogs(ctx, podList.Items[0].Name, "tuf-init", ns)
	Expect(err).NotTo(HaveOccurred())
	Expect(logs).To(ContainSubstring(logOutput))
}
