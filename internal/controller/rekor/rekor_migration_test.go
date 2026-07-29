package rekor

import (
	"context"
	"fmt"
	"time"

	rhtasv1 "github.com/securesign/operator/api/v1"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/migration"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("Rekor SearchUI migration", Ordered, func() {
	const (
		Name      = "migration-test"
		Namespace = "migration"
	)

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: Namespace,
		},
	}

	typeNamespaceName := types.NamespacedName{Name: Name, Namespace: Namespace}

	BeforeAll(func(ctx SpecContext) {
		Expect(suite.Client().Create(ctx, namespace)).To(Succeed())
	})

	AfterEach(func(ctx SpecContext) {
		found := &rhtasv1.Rekor{}
		if err := suite.Client().Get(ctx, typeNamespaceName, found); err == nil {
			Expect(suite.Client().Delete(ctx, found)).To(Succeed())
		}
	})

	AfterAll(func(ctx SpecContext) {
		_ = suite.Client().Delete(ctx, namespace)
	})

	It("creates Console CR when Rekor is created via v1alpha1 with RekorSearchUI enabled", func(ctx SpecContext) {
		treeID := int64(456)
		instance := &rhtasv1alpha1.Rekor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      Name,
				Namespace: Namespace,
			},
			Spec: rhtasv1alpha1.RekorSpec{
				TreeID: &treeID,
				Trillian: rhtasv1alpha1.TrillianService{
					Address: "trillian.default.svc",
					Port:    ptr.To(int32(8091)),
				},
				Monitoring: rhtasv1alpha1.MonitoringWithTLogConfig{
					MonitoringConfig: rhtasv1alpha1.MonitoringConfig{Enabled: false},
					TLog:             rhtasv1alpha1.TlogMonitoring{Interval: metav1.Duration{Duration: 10 * time.Minute}},
				},
				BackFillRedis: rhtasv1alpha1.BackFillRedis{
					Enabled:  ptr.To(true),
					Schedule: "0 0 * * *",
				},
				RekorSearchUI: rhtasv1alpha1.RekorSearchUI{
					Enabled: ptr.To(true),
					Host:    "rekor-ui.example.com",
					RouteSelectorLabels: map[string]string{
						"router": "internal",
					},
				},
			},
		}

		Expect(suite.Client().Create(ctx, instance)).To(Succeed())

		By("Console CR is created with correct values")
		consoleName := types.NamespacedName{Name: Name + "-console", Namespace: Namespace}
		Eventually(func(ctx context.Context) error {
			return suite.Client().Get(ctx, consoleName, &rhtasv1.Console{})
		}).WithContext(ctx).Should(Succeed())

		console := &rhtasv1.Console{}
		Expect(suite.Client().Get(ctx, consoleName, console)).To(Succeed())
		Expect(console.Spec.UI.Ingress.Enabled).To(Equal(ptr.To(true)))
		Expect(console.Spec.UI.Ingress.Host).To(Equal("rekor-ui.example.com"))
		Expect(console.Spec.UI.Ingress.Labels).To(Equal(map[string]string{"router": "internal"}))
		Expect(console.Spec.UI.Rekor.Ref).ToNot(BeNil())
		Expect(console.Spec.UI.Rekor.Ref.Name).To(Equal(Name))
		Expect(console.Spec.UI.Rekor.Ref.Namespace).To(Equal(Namespace))

		By("Migration annotation is removed after processing")
		Eventually(func(g Gomega, ctx context.Context) {
			found := &rhtasv1.Rekor{}
			g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).To(Succeed())
			g.Expect(migration.Has(found, rhtasv1alpha1.MigrationSearchUIData)).To(BeFalse())
		}).WithContext(ctx).Should(Succeed())
	})

	It("cleans up orphaned resources and removes UiAvailable condition", func(ctx SpecContext) {
		treeID := int64(789)
		instance := &rhtasv1alpha1.Rekor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      Name,
				Namespace: Namespace,
			},
			Spec: rhtasv1alpha1.RekorSpec{
				TreeID: &treeID,
				Trillian: rhtasv1alpha1.TrillianService{
					Address: "trillian.default.svc",
					Port:    ptr.To(int32(8091)),
				},
				Monitoring: rhtasv1alpha1.MonitoringWithTLogConfig{
					MonitoringConfig: rhtasv1alpha1.MonitoringConfig{Enabled: false},
					TLog:             rhtasv1alpha1.TlogMonitoring{Interval: metav1.Duration{Duration: 10 * time.Minute}},
				},
				BackFillRedis: rhtasv1alpha1.BackFillRedis{
					Enabled:  ptr.To(true),
					Schedule: "0 0 * * *",
				},
			},
		}

		Expect(suite.Client().Create(ctx, instance)).To(Succeed())

		By("Simulating pre-existing orphaned deployment owned by this Rekor")
		v1Instance := &rhtasv1.Rekor{}
		Eventually(func(ctx context.Context) error {
			return suite.Client().Get(ctx, typeNamespaceName, v1Instance)
		}).WithContext(ctx).Should(Succeed())

		orphanedDeploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.SearchUiDeploymentName,
				Namespace: Namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "rekor-search-ui"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "rekor-search-ui"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "ui", Image: "test"}}},
				},
			},
		}
		Expect(suite.Client().Create(ctx, orphanedDeploy)).To(Succeed())
		Expect(controllerutil.SetControllerReference(v1Instance, orphanedDeploy, suite.Client().Scheme())).To(Succeed())
		Expect(suite.Client().Update(ctx, orphanedDeploy)).To(Succeed())

		By("Continuously injecting UiAvailable condition until cleanup fires and deletes deployment")
		deployKey := client.ObjectKeyFromObject(orphanedDeploy)
		Eventually(func(ctx context.Context) error {
			found := &rhtasv1.Rekor{}
			if err := suite.Client().Get(ctx, typeNamespaceName, found); err != nil {
				return err
			}
			if meta.FindStatusCondition(found.Status.Conditions, actions.UICondition) == nil {
				meta.SetStatusCondition(&found.Status.Conditions, metav1.Condition{
					Type:   actions.UICondition,
					Status: metav1.ConditionTrue,
					Reason: "Ready",
				})
				_ = suite.Client().Status().Update(ctx, found)
			}

			if err := suite.Client().Get(ctx, deployKey, &appsv1.Deployment{}); err == nil {
				return fmt.Errorf("orphaned deployment still exists")
			}
			return nil
		}).WithContext(ctx).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

		By("UiAvailable condition is removed after cleanup")
		Eventually(func(g Gomega, ctx context.Context) {
			found := &rhtasv1.Rekor{}
			g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).To(Succeed())
			g.Expect(meta.FindStatusCondition(found.Status.Conditions, actions.UICondition)).To(BeNil())
		}).WithContext(ctx).Should(Succeed())
	})
})
