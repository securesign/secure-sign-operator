//go:build integration

package e2e

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas/trillian"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Rekor Monitor", Ordered, func() {
	cli, _ := support.CreateClient()

	var (
		namespace  *v1.Namespace
		trillianCR *v1alpha1.Trillian
		rekor      *v1alpha1.Rekor
	)

	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))

	BeforeAll(func(ctx SpecContext) {
		trillianCR = &v1alpha1.Trillian{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-trillian",
				Namespace: namespace.Name,
			},
			Spec: v1alpha1.TrillianSpec{
				Db: v1alpha1.TrillianDB{Create: ptr.To(true)},
			},
		}

		Expect(cli.Create(ctx, trillianCR)).To(Succeed())

		By("Waiting for Trillian to be ready")
		trillian.Verify(ctx, cli, namespace.Name, trillianCR.Name, true)
	})

	BeforeAll(func(ctx SpecContext) {
		rekor = &v1alpha1.Rekor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rekor-monitor",
				Namespace: namespace.Name,
			},
			Spec: v1alpha1.RekorSpec{
				Monitoring: v1alpha1.MonitoringWithTLogConfig{
					MonitoringConfig: v1alpha1.MonitoringConfig{
						Enabled: true,
					},
					TLog: v1alpha1.TlogMonitoring{
						Enabled:  true,
						Interval: metav1.Duration{Duration: time.Minute * 5},
					},
				},
				Trillian: v1alpha1.TrillianService{
					Address: fmt.Sprintf("trillian-logserver.%s.svc.cluster.local", namespace.Name),
					Port:    ptr.To(int32(8091)),
				},
			},
		}

		Expect(cli.Create(ctx, rekor)).To(Succeed())
	})

	Describe("Monitor Pod Deployment", func() {
		It("should create and run the rekor-monitor StatefulSet", func(ctx SpecContext) {
			Eventually(func(g Gomega) {
				statefulSet := &appsv1.StatefulSet{}
				err := cli.Get(ctx, types.NamespacedName{
					Namespace: namespace.Name,
					Name:      actions.MonitorStatefulSetName,
				}, statefulSet)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(statefulSet.Status.Replicas).To(Equal(int32(1)))
				g.Expect(statefulSet.Status.ReadyReplicas).To(Equal(int32(1)))
			}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
		})

		It("should have a running rekor-monitor pod", func(ctx SpecContext) {
			Eventually(func(g Gomega) {
				podList := &v1.PodList{}
				err := cli.List(ctx, podList,
					ctrl.InNamespace(namespace.Name),
					ctrl.MatchingLabels{
						labels.LabelAppComponent: actions.MonitorComponentName,
					})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(podList.Items).To(HaveLen(1))

				pod := podList.Items[0]
				g.Expect(pod.Status.Phase).To(Equal(v1.PodRunning))

				g.Expect(pod.Status.ContainerStatuses).To(HaveLen(1))
				g.Expect(pod.Status.ContainerStatuses[0].Ready).To(BeTrue())
			}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
		})

		It("should verify the Rekor CR has monitor condition created", func(ctx SpecContext) {
			Eventually(func(g Gomega) {
				updated := &v1alpha1.Rekor{}
				err := cli.Get(ctx, types.NamespacedName{
					Namespace: namespace.Name,
					Name:      rekor.Name,
				}, updated)
				g.Expect(err).ToNot(HaveOccurred())

				monitorCondition := meta.FindStatusCondition(updated.Status.Conditions, actions.MonitorCondition)
				g.Expect(monitorCondition).ToNot(BeNil())
				g.Expect(monitorCondition.Type).To(Equal(actions.MonitorCondition))
				// The condition may be False with reason "Creating" - that's fine, it means monitor was created
				g.Expect(monitorCondition.Reason).To(BeElementOf([]string{"Creating", "Ready"}))
			}).WithTimeout(2 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
		})
	})
})
