package actions

import (
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	common "github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/testing/action"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func setupMigrateAction() migrationJobAction {
	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(batchv1.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	migrateJobTestAction := migrationJobAction{
		BaseAction: common.BaseAction{
			Client:   fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1alpha1.Tuf{}).Build(),
			Recorder: record.NewFakeRecorder(10),
			Logger:   logr.Logger{},
		},
	}

	utilruntime.Must(v1alpha1.AddToScheme(migrateJobTestAction.Client.Scheme()))
	return migrateJobTestAction
}

func TestMigrateJob_AlreadyMigrated(t *testing.T) {
	g := NewWithT(t)

	migrateJobTestAction := setupMigrateAction()

	instance := &v1alpha1.Tuf{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: t.Name(),
			Annotations: map[string]string{
				tufConstants.RepositoryVersionAnnotation: tufConstants.TufVersionV1,
			},
		},
		Spec: v1alpha1.TufSpec{
			SigningConfigURLMode: v1alpha1.SigningConfigURLInternal,
		},
		Status: v1alpha1.TufStatus{Conditions: []metav1.Condition{
			{
				Type:   constants.ReadyCondition,
				Reason: state.Initialize.String(),
				Status: metav1.ConditionFalse,
			},
		}}}
	result := migrateJobTestAction.CanHandle(t.Context(), instance)
	g.Expect(result).To(BeFalse())
}

func TestMigrateJob_NoRootKeySecret(t *testing.T) {
	g := NewWithT(t)

	migrateJobTestAction := setupMigrateAction()

	instance := &v1alpha1.Tuf{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: t.Name(),
		},
		Spec: v1alpha1.TufSpec{
			RootKeySecretRef: &v1alpha1.LocalObjectReference{
				// root key is specified but secret does not exist
				Name: "test-secret",
			},
			SigningConfigURLMode: v1alpha1.SigningConfigURLInternal,
		},
		Status: v1alpha1.TufStatus{Conditions: []metav1.Condition{
			{
				Type:   constants.ReadyCondition,
				Reason: state.Initialize.String(),
				Status: metav1.ConditionFalse,
			},
		}}}
	g.Expect(migrateJobTestAction.Client.Create(t.Context(), instance)).To(Succeed())

	g.Expect(migrateJobTestAction.CanHandle(t.Context(), instance)).To(BeTrue())
	result := migrateJobTestAction.Handle(t.Context(), instance)
	g.Expect(result.Err).To(HaveOccurred())
	g.Expect(result.Err).To(MatchError(ContainSubstring("cannot migrate TUF: root key secret test-secret not found")))

	g.Expect(meta.FindStatusCondition(instance.Status.Conditions, constants.ReadyCondition).Reason).To(Equal(state.Failure.String()))
	g.Expect(meta.FindStatusCondition(instance.Status.Conditions, constants.ReadyCondition).Message).To(ContainSubstring("cannot migrate TUF: root key secret test-secret not found"))
}

func TestMigrateJob_Succeeded(t *testing.T) {
	g := NewWithT(t)

	migrateJobTestAction := setupMigrateAction()

	g.Expect(migrateJobTestAction.Client.Create(t.Context(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: t.Name(),
		},
	})).To(Succeed())

	instance := &v1alpha1.Tuf{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: t.Name(),
		},
		Spec: v1alpha1.TufSpec{
			RootKeySecretRef: &v1alpha1.LocalObjectReference{
				Name: "test-secret",
			},
			PodRequirements: v1alpha1.PodRequirements{
				Replicas: ptr.To(int32(1)),
			},
			SigningConfigURLMode: v1alpha1.SigningConfigURLInternal,
		},
		Status: v1alpha1.TufStatus{Conditions: []metav1.Condition{
			{
				Type:   constants.ReadyCondition,
				Reason: state.Initialize.String(),
				Status: metav1.ConditionFalse,
			},
		}}}
	g.Expect(migrateJobTestAction.Client.Create(t.Context(), instance)).To(Succeed())
	g.Expect(migrateJobTestAction.CanHandle(t.Context(), instance)).To(BeTrue())
	result := migrateJobTestAction.Handle(t.Context(), instance)
	g.Expect(result).To(Equal(action.StatusUpdate()))
	g.Expect(instance.Status.Conditions).To(ContainElement(metav1.Condition{
		Type:    constants.ReadyCondition,
		Reason:  state.Initialize.String(),
		Status:  metav1.ConditionFalse,
		Message: "migration job created",
	}))

	jobList := &batchv1.JobList{}
	g.Expect(migrateJobTestAction.Client.List(t.Context(), jobList, client.MatchingLabels(labels.ForResource(tufConstants.ComponentName, tufConstants.MigrationJobName, instance.Name, instance.Status.PvcName)))).To(Succeed())
	g.Expect(jobList.Items).To(HaveLen(1))
	job := &jobList.Items[0]

	g.Expect(job.Spec.Template.Spec.Affinity).To(Not(BeNil()))
	g.Expect(job.Spec.Template.Spec.Affinity.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution).To(HaveLen(1))
	g.Expect(job.Spec.Template.Spec.Affinity.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution[0].LabelSelector.MatchLabels).To(Equal(labels.For(tufConstants.ComponentName, tufConstants.DeploymentName, instance.Name)))
	g.Expect(job.Spec.Template.Spec.Affinity.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution[0].TopologyKey).To(Equal("kubernetes.io/hostname"))

	// another reconciliation before job is completed
	result = migrateJobTestAction.Handle(t.Context(), instance)
	g.Expect(instance.Status.Conditions).To(ContainElement(metav1.Condition{
		Type:    constants.ReadyCondition,
		Reason:  state.Initialize.String(),
		Status:  metav1.ConditionFalse,
		Message: "waiting for migration job to complete",
	}))
	g.Expect(result).To(Equal(action.Requeue()))

	job.Status.Succeeded = 1
	job.Status.Failed = 0
	job.Status.Conditions = []batchv1.JobCondition{
		{
			Type:   batchv1.JobComplete,
			Status: corev1.ConditionTrue,
		},
	}
	g.Expect(migrateJobTestAction.Client.Status().Update(t.Context(), job)).To(Succeed())

	g.Expect(migrateJobTestAction.CanHandle(t.Context(), instance)).To(BeTrue())
	result = migrateJobTestAction.Handle(t.Context(), instance)
	g.Expect(instance.Status.Conditions).To(ContainElement(metav1.Condition{
		Type:    constants.ReadyCondition,
		Reason:  state.Initialize.String(),
		Status:  metav1.ConditionFalse,
		Message: "migration job passed",
	}))
	g.Expect(result).To(Equal(action.StatusUpdate()))

	found := &v1alpha1.Tuf{}
	g.Expect(migrateJobTestAction.Client.Get(t.Context(), client.ObjectKeyFromObject(instance), found)).To(Succeed())
	g.Expect(found.Annotations[tufConstants.RepositoryVersionAnnotation]).To(Equal(tufConstants.TufVersionV1))

	g.Expect(migrateJobTestAction.CanHandle(t.Context(), instance)).To(BeFalse())
}

func TestMigrateJob_Failed(t *testing.T) {
	g := NewWithT(t)

	migrateJobTestAction := setupMigrateAction()

	g.Expect(migrateJobTestAction.Client.Create(t.Context(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: t.Name(),
		},
	})).To(Succeed())

	instance := &v1alpha1.Tuf{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: t.Name(),
		},
		Spec: v1alpha1.TufSpec{
			RootKeySecretRef: &v1alpha1.LocalObjectReference{
				Name: "test-secret",
			},
			PodRequirements: v1alpha1.PodRequirements{
				Replicas: ptr.To(int32(1)),
			},
			SigningConfigURLMode: v1alpha1.SigningConfigURLInternal,
		},
		Status: v1alpha1.TufStatus{Conditions: []metav1.Condition{
			{
				Type:   constants.ReadyCondition,
				Reason: state.Initialize.String(),
				Status: metav1.ConditionFalse,
			},
		}}}
	g.Expect(migrateJobTestAction.Client.Create(t.Context(), instance)).To(Succeed())
	g.Expect(migrateJobTestAction.CanHandle(t.Context(), instance)).To(BeTrue())
	result := migrateJobTestAction.Handle(t.Context(), instance)
	g.Expect(result).To(Equal(action.StatusUpdate()))
	g.Expect(instance.Status.Conditions).To(ContainElement(metav1.Condition{
		Type:    constants.ReadyCondition,
		Reason:  state.Initialize.String(),
		Status:  metav1.ConditionFalse,
		Message: "migration job created",
	}))

	jobList := &batchv1.JobList{}
	g.Expect(migrateJobTestAction.Client.List(t.Context(), jobList, client.MatchingLabels(labels.ForResource(tufConstants.ComponentName, tufConstants.MigrationJobName, instance.Name, instance.Status.PvcName)))).To(Succeed())
	g.Expect(jobList.Items).To(HaveLen(1))
	job := &jobList.Items[0]

	result = migrateJobTestAction.Handle(t.Context(), instance)
	g.Expect(instance.Status.Conditions).To(ContainElement(metav1.Condition{
		Type:    constants.ReadyCondition,
		Reason:  state.Initialize.String(),
		Status:  metav1.ConditionFalse,
		Message: "waiting for migration job to complete",
	}))
	g.Expect(result).To(Equal(action.Requeue()))

	job.Status.Succeeded = 0
	job.Status.Failed = 1
	job.Status.Conditions = []batchv1.JobCondition{
		{
			Type:   batchv1.JobComplete,
			Status: corev1.ConditionTrue,
		},
		{
			Type:   batchv1.JobFailed,
			Status: corev1.ConditionTrue,
		},
	}
	g.Expect(migrateJobTestAction.Client.Status().Update(t.Context(), job)).To(Succeed())

	result = migrateJobTestAction.Handle(t.Context(), instance)
	g.Expect(result.Err).To(HaveOccurred())
	g.Expect(result.Err).To(MatchError(ContainSubstring("tuf-repository-migration job failed")))

	g.Expect(meta.FindStatusCondition(instance.Status.Conditions, constants.ReadyCondition).Reason).To(Equal(state.Failure.String()))
	g.Expect(meta.FindStatusCondition(instance.Status.Conditions, constants.ReadyCondition).Message).To(ContainSubstring("tuf-repository-migration job failed"))

	found := &v1alpha1.Tuf{}
	g.Expect(migrateJobTestAction.Client.Get(t.Context(), client.ObjectKeyFromObject(instance), found)).To(Succeed())
	g.Expect(found.Annotations).ToNot(HaveKey(tufConstants.RepositoryVersionAnnotation))

}
