package action

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	testenvhelper "github.com/securesign/operator/internal/testing/envtest"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	k8sClient   client.Client
	testEnv     *envtest.Environment
	errExpected = errors.New("expected error")
)

func TestMain(m *testing.M) {
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: testenvhelper.FindBinaryAssetsDir(),
	}

	if err := rhtasv1.AddToScheme(scheme.Scheme); err != nil {
		panic(err)
	}

	cfg, err := testEnv.Start()
	if err != nil {
		panic(err)
	}

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		panic(err)
	}

	code := m.Run()

	if err := testEnv.Stop(); err != nil {
		panic(err)
	}
	os.Exit(code)
}

func newBaseAction() *BaseAction {
	return &BaseAction{
		Client:   k8sClient,
		Logger:   logr.Discard(),
		Recorder: events.NewFakeRecorder(10),
	}
}

func newTufInstance(name string) *rhtasv1.Tuf {
	t := &rhtasv1.Tuf{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
	}
	t.Spec.SetDefaults()
	return t
}

func TestPersistStatus(t *testing.T) {
	t.Run("persists status when changed", func(t *testing.T) {
		g := gomega.NewGomegaWithT(t)
		ctx := t.Context()

		instance := newTufInstance("ps-changed")
		g.Expect(k8sClient.Create(ctx, instance)).To(gomega.Succeed())
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, instance) })

		a := newBaseAction()

		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:   "Ready",
			Status: metav1.ConditionTrue,
			Reason: "TestReason",
		})

		changed, err := a.PersistStatus(ctx, instance)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		g.Expect(changed).To(gomega.BeTrue())

		updated := &rhtasv1.Tuf{}
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(instance), updated)).To(gomega.Succeed())
		cond := meta.FindStatusCondition(updated.Status.Conditions, "Ready")
		g.Expect(cond).ToNot(gomega.BeNil())
		g.Expect(cond.Status).To(gomega.Equal(metav1.ConditionTrue))
		g.Expect(cond.Reason).To(gomega.Equal("TestReason"))
	})

	t.Run("skips update when status unchanged", func(t *testing.T) {
		g := gomega.NewGomegaWithT(t)
		ctx := t.Context()

		instance := newTufInstance("ps-noop")
		g.Expect(k8sClient.Create(ctx, instance)).To(gomega.Succeed())
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, instance) })

		a := newBaseAction()

		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:   "Ready",
			Status: metav1.ConditionFalse,
			Reason: "Pending",
		})
		changed, err := a.PersistStatus(ctx, instance)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		g.Expect(changed).To(gomega.BeTrue())

		// Re-read from server to sync (API server truncates time precision)
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(gomega.Succeed())
		rvBefore := instance.ResourceVersion

		// Call PersistStatus again with no changes
		changed, err = a.PersistStatus(ctx, instance)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		g.Expect(changed).To(gomega.BeFalse())

		// resourceVersion should NOT have changed — no API call was made
		after := &rhtasv1.Tuf{}
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(instance), after)).To(gomega.Succeed())
		g.Expect(after.ResourceVersion).To(gomega.Equal(rvBefore))
	})

	t.Run("returns error when object not found", func(t *testing.T) {
		g := gomega.NewGomegaWithT(t)
		ctx := t.Context()

		instance := newTufInstance("ps-notfound")
		a := newBaseAction()

		_, err := a.PersistStatus(ctx, instance)
		g.Expect(err).To(gomega.HaveOccurred())
	})

	t.Run("retries on conflict and succeeds", func(t *testing.T) {
		g := gomega.NewGomegaWithT(t)
		ctx := t.Context()

		instance := newTufInstance("ps-conflict")
		g.Expect(k8sClient.Create(ctx, instance)).To(gomega.Succeed())
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, instance) })

		a := newBaseAction()

		// Bump resourceVersion on the server so instance becomes stale
		serverCopy := instance.DeepCopy()
		meta.SetStatusCondition(&serverCopy.Status.Conditions, metav1.Condition{
			Type:   "Initializing",
			Status: metav1.ConditionTrue,
			Reason: "BumpRV",
		})
		g.Expect(k8sClient.Status().Update(ctx, serverCopy)).To(gomega.Succeed())

		// instance now has a stale resourceVersion — PersistStatus must retry
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:   "Ready",
			Status: metav1.ConditionTrue,
			Reason: "TestConflict",
		})

		changed, err := a.PersistStatus(ctx, instance)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		g.Expect(changed).To(gomega.BeTrue())

		updated := &rhtasv1.Tuf{}
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(instance), updated)).To(gomega.Succeed())
		g.Expect(meta.IsStatusConditionTrue(updated.Status.Conditions, "Ready")).To(gomega.BeTrue())
	})
}

func TestRequeue(t *testing.T) {
	t.Run("default delay", func(t *testing.T) {
		g := gomega.NewGomegaWithT(t)
		a := &BaseAction{}
		result := a.Requeue()
		g.Expect(result.Err).ToNot(gomega.HaveOccurred())
		g.Expect(result.Result.RequeueAfter).To(gomega.Equal(100 * time.Millisecond))
	})

	t.Run("custom delay", func(t *testing.T) {
		g := gomega.NewGomegaWithT(t)
		a := &BaseAction{}
		result := a.RequeueAfter(15 * time.Second)
		g.Expect(result.Err).ToNot(gomega.HaveOccurred())
		g.Expect(result.Result.RequeueAfter).To(gomega.Equal(15 * time.Second))
	})
}

func TestReturn(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	a := &BaseAction{}
	result := a.Return()
	g.Expect(result.Err).ToNot(gomega.HaveOccurred())
	g.Expect(result.Result).To(gomega.Equal(reconcile.Result{}))
}

func TestContinue(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	a := &BaseAction{}
	result := a.Continue()
	g.Expect(result).To(gomega.BeNil())
}

func stubFn(changed bool, err error) func(context.Context, client.Object) (bool, error) {
	return func(context.Context, client.Object) (bool, error) {
		return changed, err
	}
}

func TestReturnOnChange(t *testing.T) {
	t.Run("returns Continue when status unchanged", func(t *testing.T) {
		g := gomega.NewGomegaWithT(t)
		ctx := t.Context()

		instance := newTufInstance("roc-unchanged")
		g.Expect(k8sClient.Create(ctx, instance)).To(gomega.Succeed())
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, instance) })

		a := newBaseAction()
		result := a.ReturnOnChange(stubFn(false, nil))(ctx, instance)
		g.Expect(result).To(gomega.BeNil())
	})

	t.Run("returns Return when status changed", func(t *testing.T) {
		g := gomega.NewGomegaWithT(t)
		ctx := t.Context()

		instance := newTufInstance("roc-changed")
		g.Expect(k8sClient.Create(ctx, instance)).To(gomega.Succeed())
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, instance) })

		a := newBaseAction()
		result := a.ReturnOnChange(stubFn(true, nil))(ctx, instance)
		g.Expect(result).ToNot(gomega.BeNil())
		g.Expect(result.Err).ToNot(gomega.HaveOccurred())
		g.Expect(result.Result).To(gomega.Equal(reconcile.Result{}))
	})

	t.Run("returns Error when error occurred", func(t *testing.T) {
		g := gomega.NewGomegaWithT(t)
		ctx := t.Context()

		instance := newTufInstance("roc-error")
		g.Expect(k8sClient.Create(ctx, instance)).To(gomega.Succeed())
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, instance) })

		a := newBaseAction()
		result := a.ReturnOnChange(stubFn(false, errExpected))(ctx, instance)
		g.Expect(result).ToNot(gomega.BeNil())
		g.Expect(result.Err).To(gomega.MatchError(errExpected))
	})

	t.Run("error takes precedence over changed flag", func(t *testing.T) {
		g := gomega.NewGomegaWithT(t)
		ctx := t.Context()

		instance := newTufInstance("roc-err-precedence")
		g.Expect(k8sClient.Create(ctx, instance)).To(gomega.Succeed())
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, instance) })

		a := newBaseAction()
		result := a.ReturnOnChange(stubFn(true, errExpected))(ctx, instance)
		g.Expect(result).ToNot(gomega.BeNil())
		g.Expect(result.Err).To(gomega.MatchError(errExpected))
	})

	t.Run("works with PersistStatus end-to-end", func(t *testing.T) {
		g := gomega.NewGomegaWithT(t)
		ctx := t.Context()

		instance := newTufInstance("roc-e2e")
		g.Expect(k8sClient.Create(ctx, instance)).To(gomega.Succeed())
		t.Cleanup(func() { _ = k8sClient.Delete(ctx, instance) })

		a := newBaseAction()

		// First call: status changes → Return
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:   "Ready",
			Status: metav1.ConditionTrue,
			Reason: "Deployed",
		})
		result := a.ReturnOnChange(a.PersistStatus)(ctx, instance)
		g.Expect(result).ToNot(gomega.BeNil())
		g.Expect(result.Err).ToNot(gomega.HaveOccurred())
		g.Expect(result.Result).To(gomega.Equal(reconcile.Result{}))

		// Re-read from server to sync
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(gomega.Succeed())

		// Second call: status unchanged → Continue
		result = a.ReturnOnChange(a.PersistStatus)(ctx, instance)
		g.Expect(result).To(gomega.BeNil())
	})
}
