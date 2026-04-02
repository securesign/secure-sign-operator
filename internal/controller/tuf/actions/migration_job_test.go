package actions

import (
	"context"
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
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var migrateJobTestContext = context.TODO()

func setupMigrateAction() migrationJobAction {
	migrateJobTestAction := migrationJobAction{
		BaseAction: common.BaseAction{
			Client:   fake.NewFakeClient(),
			Recorder: record.NewFakeRecorder(10),
			Logger:   logr.Logger{},
		},
	}

	utilruntime.Must(batchv1.AddToScheme(migrateJobTestAction.Client.Scheme()))
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
	result := migrateJobTestAction.CanHandle(migrateJobTestContext, instance)
	g.Expect(result).To(BeFalse())
}

func TestMigrateJob_NoRootKeySecret(t *testing.T) {
	g := NewWithT(t)

	migrateJobTestAction := setupMigrateAction()
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tufConstants.DeploymentName,
			Namespace: t.Name(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
		},
	}
	g.Expect(migrateJobTestAction.Client.Create(migrateJobTestContext, deployment)).To(Succeed())

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
	g.Expect(migrateJobTestAction.CanHandle(migrateJobTestContext, instance)).To(BeTrue())
	result := migrateJobTestAction.Handle(migrateJobTestContext, instance)
	g.Expect(result.Err).To(HaveOccurred())
	g.Expect(result.Err).To(MatchError(ContainSubstring("cannot migrate TUF: root key secret test-secret not found")))

	g.Expect(meta.FindStatusCondition(instance.Status.Conditions, constants.ReadyCondition).Reason).To(Equal(state.Failure.String()))
	g.Expect(meta.FindStatusCondition(instance.Status.Conditions, constants.ReadyCondition).Message).To(ContainSubstring("cannot migrate TUF: root key secret test-secret not found"))

	g.Expect(migrateJobTestAction.Client.Get(migrateJobTestContext, client.ObjectKeyFromObject(deployment), deployment)).To(Succeed())
	g.Expect(*deployment.Spec.Replicas).To(BeNumerically("==", 1))
}

func TestMigrateJob_Succeeded(t *testing.T) {
	g := NewWithT(t)

	migrateJobTestAction := setupMigrateAction()
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tufConstants.DeploymentName,
			Namespace: t.Name(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
		},
	}
	g.Expect(migrateJobTestAction.Client.Create(migrateJobTestContext, deployment)).To(Succeed())

	g.Expect(migrateJobTestAction.Client.Create(migrateJobTestContext, &corev1.Secret{
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
	g.Expect(migrateJobTestAction.Client.Create(migrateJobTestContext, instance)).To(Succeed())
	g.Expect(migrateJobTestAction.CanHandle(migrateJobTestContext, instance)).To(BeTrue())
	result := migrateJobTestAction.Handle(migrateJobTestContext, instance)
	g.Expect(result).To(Equal(action.Requeue()))

	g.Expect(migrateJobTestAction.Client.Get(migrateJobTestContext, client.ObjectKeyFromObject(deployment), deployment)).To(Succeed())
	g.Expect(*deployment.Spec.Replicas).To(BeNumerically("==", 0))

	jobList := &batchv1.JobList{}
	g.Expect(migrateJobTestAction.Client.List(migrateJobTestContext, jobList, client.MatchingLabels(labels.ForResource(tufConstants.ComponentName, tufConstants.MigrationJobName, instance.Name, instance.Status.PvcName)))).To(Succeed())
	g.Expect(jobList.Items).To(HaveLen(1))
	job := &jobList.Items[0]

	job.Status.Succeeded = 1
	job.Status.Failed = 0
	job.Status.Conditions = []batchv1.JobCondition{
		{
			Type:   batchv1.JobComplete,
			Status: corev1.ConditionTrue,
		},
	}
	g.Expect(migrateJobTestAction.Client.Status().Update(migrateJobTestContext, job)).To(Succeed())

	g.Expect(migrateJobTestAction.CanHandle(migrateJobTestContext, instance)).To(BeTrue())
	result = migrateJobTestAction.Handle(migrateJobTestContext, instance)
	g.Expect(result).To(Equal(action.Requeue()))
	found := &v1alpha1.Tuf{}
	g.Expect(migrateJobTestAction.Client.Get(migrateJobTestContext, client.ObjectKeyFromObject(instance), found)).To(Succeed())
	g.Expect(found.Annotations[tufConstants.RepositoryVersionAnnotation]).To(Equal(tufConstants.TufVersionV1))

	g.Expect(migrateJobTestAction.Client.Get(migrateJobTestContext, types.NamespacedName{Namespace: t.Name(), Name: tufConstants.DeploymentName}, deployment)).To(Succeed())
	g.Expect(*deployment.Spec.Replicas).To(BeNumerically("==", 1))

	g.Expect(migrateJobTestAction.CanHandle(migrateJobTestContext, instance)).To(BeFalse())
}

func TestMigrateJob_Failed(t *testing.T) {
	g := NewWithT(t)

	migrateJobTestAction := setupMigrateAction()
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tufConstants.DeploymentName,
			Namespace: t.Name(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
		},
	}
	g.Expect(migrateJobTestAction.Client.Create(migrateJobTestContext, deployment)).To(Succeed())

	g.Expect(migrateJobTestAction.Client.Create(migrateJobTestContext, &corev1.Secret{
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
	g.Expect(migrateJobTestAction.Client.Create(migrateJobTestContext, instance)).To(Succeed())
	g.Expect(migrateJobTestAction.CanHandle(migrateJobTestContext, instance)).To(BeTrue())
	result := migrateJobTestAction.Handle(migrateJobTestContext, instance)
	g.Expect(result).To(Equal(action.Requeue()))

	g.Expect(migrateJobTestAction.Client.Get(migrateJobTestContext, client.ObjectKeyFromObject(deployment), deployment)).To(Succeed())
	g.Expect(*deployment.Spec.Replicas).To(BeNumerically("==", 0))

	jobList := &batchv1.JobList{}
	g.Expect(migrateJobTestAction.Client.List(migrateJobTestContext, jobList, client.MatchingLabels(labels.ForResource(tufConstants.ComponentName, tufConstants.MigrationJobName, instance.Name, instance.Status.PvcName)))).To(Succeed())
	g.Expect(jobList.Items).To(HaveLen(1))
	job := &jobList.Items[0]

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
	g.Expect(migrateJobTestAction.Client.Status().Update(migrateJobTestContext, job)).To(Succeed())

	result = migrateJobTestAction.Handle(migrateJobTestContext, instance)
	g.Expect(result.Err).To(HaveOccurred())
	g.Expect(result.Err).To(MatchError(ContainSubstring("tuf-repository-migration job failed")))

	g.Expect(meta.FindStatusCondition(instance.Status.Conditions, constants.ReadyCondition).Reason).To(Equal(state.Failure.String()))
	g.Expect(meta.FindStatusCondition(instance.Status.Conditions, constants.ReadyCondition).Message).To(ContainSubstring("tuf-repository-migration job failed"))

	found := &v1alpha1.Tuf{}
	g.Expect(migrateJobTestAction.Client.Get(migrateJobTestContext, client.ObjectKeyFromObject(instance), found)).To(Succeed())
	g.Expect(found.Annotations).ToNot(HaveKey(tufConstants.RepositoryVersionAnnotation))

	g.Expect(migrateJobTestAction.Client.Get(migrateJobTestContext, types.NamespacedName{Namespace: t.Name(), Name: tufConstants.DeploymentName}, deployment)).To(Succeed())
	g.Expect(*deployment.Spec.Replicas).To(BeNumerically("==", 1))

}
