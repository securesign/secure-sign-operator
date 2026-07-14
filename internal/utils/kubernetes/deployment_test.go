package kubernetes

import (
	"errors"
	"testing"

	"github.com/onsi/gomega"
	testAction "github.com/securesign/operator/internal/testing/action"
	k8sTest "github.com/securesign/operator/internal/testing/kubernetes"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func fakeDeploymentClient(objs ...client.Object) client.Client {
	return testAction.FakeClientBuilder().WithObjects(objs...).Build()
}

func rolledOutDeployment(name string) *appsv1.Deployment {
	return k8sTest.RolledOutDeployment(name, "ns")
}

func TestDeploymentIsRunningByName(t *testing.T) {
	tests := []struct {
		name             string
		deployment       *appsv1.Deployment
		extraObjs        []client.Object
		wantOK           bool
		wantErrs         []error
		wantErrSubstring string
	}{
		{
			name:       "not found",
			deployment: nil,
			wantOK:     false,
			wantErrs:   []error{ErrDeploymentNotReady, ErrDeploymentNotFound},
		},
		{
			name:       "rolled out",
			deployment: rolledOutDeployment("ctlog"),
			wantOK:     true,
		},
		{
			name: "generation not observed",
			deployment: func() *appsv1.Deployment {
				d := rolledOutDeployment("ctlog")
				d.Generation = 2
				d.Status.ObservedGeneration = 1
				return d
			}(),
			wantOK:   false,
			wantErrs: []error{ErrDeploymentNotReady, ErrDeploymentNotObserved},
		},
		{
			name: "not available",
			deployment: func() *appsv1.Deployment {
				d := rolledOutDeployment("ctlog")
				d.Status.Conditions[0].Status = corev1.ConditionFalse
				return d
			}(),
			wantOK:   false,
			wantErrs: []error{ErrDeploymentNotReady, ErrDeploymentNotAvailable},
		},
		{
			name: "new replicaset not available, no revision",
			deployment: func() *appsv1.Deployment {
				d := rolledOutDeployment("ctlog")
				d.Status.Conditions[1].Status = corev1.ConditionFalse
				d.Status.Conditions[1].Reason = "ReplicaSetUpdated"
				return d
			}(),
			wantOK:   false,
			wantErrs: []error{ErrDeploymentNotReady, ErrNewReplicaSetNotAvailable},
		},
		{
			name: "revision without matching replicaset",
			deployment: func() *appsv1.Deployment {
				d := rolledOutDeployment("ctlog")
				d.Annotations = map[string]string{revisionAnnotation: "3"}
				return d
			}(),
			wantOK:   false,
			wantErrs: []error{ErrDeploymentNotReady, ErrReplicaSetRevisionNotExists},
		},
		{
			name: "revision matches replicaset, rolled out",
			deployment: func() *appsv1.Deployment {
				d := rolledOutDeployment("ctlog")
				d.UID = "dep-uid"
				d.Annotations = map[string]string{revisionAnnotation: "3"}
				d.Status.Conditions[1].Message = `ReplicaSet "ctlog-abc123" has successfully progressed.`
				return d
			}(),
			extraObjs: []client.Object{
				&appsv1.ReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "ctlog-abc123",
						Namespace:   "ns",
						Annotations: map[string]string{revisionAnnotation: "3"},
						Labels:      map[string]string{podTemplateHash: "ctlog-abc123"},
						OwnerReferences: []metav1.OwnerReference{
							{Controller: ptr.To(true), UID: "dep-uid"},
						},
					},
				},
			},
			wantOK: true,
		},
		{
			name: "revision matches replicaset, message missing template hash",
			deployment: func() *appsv1.Deployment {
				d := rolledOutDeployment("ctlog")
				d.UID = "dep-uid"
				d.Annotations = map[string]string{revisionAnnotation: "3"}
				d.Status.Conditions[1].Message = `ReplicaSet "ctlog-other" has successfully progressed.`
				return d
			}(),
			extraObjs: []client.Object{
				&appsv1.ReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "ctlog-abc123",
						Namespace:   "ns",
						Annotations: map[string]string{revisionAnnotation: "3"},
						Labels:      map[string]string{podTemplateHash: "ctlog-abc123"},
						OwnerReferences: []metav1.OwnerReference{
							{Controller: ptr.To(true), UID: "dep-uid"},
						},
					},
				},
			},
			wantOK:   false,
			wantErrs: []error{ErrDeploymentNotReady, ErrNewReplicaSetNotAvailable},
		},
		{
			name: "progress deadline exceeded",
			deployment: func() *appsv1.Deployment {
				d := rolledOutDeployment("ctlog")
				d.Status.Conditions = []appsv1.DeploymentCondition{
					{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
					{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, Reason: "ProgressDeadlineExceeded", Message: `ReplicaSet "ctlog-abc123" has timed out progressing.`},
				}
				return d
			}(),
			wantOK:           false,
			wantErrs:         []error{ErrDeploymentNotReady, ErrDeploymentProgressDeadlineExceeded},
			wantErrSubstring: `ReplicaSet "ctlog-abc123" has timed out progressing.`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			var objs []client.Object
			lookupName := "missing"
			if tt.deployment != nil {
				objs = append(objs, tt.deployment)
				lookupName = tt.deployment.Name
			}
			objs = append(objs, tt.extraObjs...)
			cli := fakeDeploymentClient(objs...)

			ok, err := DeploymentIsRunningByName(t.Context(), cli, "ns", lookupName)

			g.Expect(ok).To(gomega.Equal(tt.wantOK))
			for _, wantErr := range tt.wantErrs {
				g.Expect(errors.Is(err, wantErr)).To(gomega.BeTrue())
			}
			if tt.wantErrSubstring != "" {
				g.Expect(err.Error()).To(gomega.ContainSubstring(tt.wantErrSubstring))
			}
		})
	}
}

func TestDeploymentIsRunning_LabelBased(t *testing.T) {
	labels := map[string]string{"app.kubernetes.io/component": "ctlog"}

	t.Run("no matching deployments", func(t *testing.T) {
		g := gomega.NewWithT(t)
		cli := fakeDeploymentClient()
		ok, err := DeploymentIsRunning(t.Context(), cli, "ns", labels)
		g.Expect(ok).To(gomega.BeFalse())
		g.Expect(errors.Is(err, ErrDeploymentNotFound)).To(gomega.BeTrue())
	})

	t.Run("one matching, rolled out", func(t *testing.T) {
		g := gomega.NewWithT(t)
		d := rolledOutDeployment("ctlog")
		d.Labels = labels
		cli := fakeDeploymentClient(d)
		ok, err := DeploymentIsRunning(t.Context(), cli, "ns", labels)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		g.Expect(ok).To(gomega.BeTrue())
	})

	t.Run("multiple matching, one not rolled out", func(t *testing.T) {
		g := gomega.NewWithT(t)
		ready := rolledOutDeployment("ctlog-a")
		ready.Labels = labels
		notReady := rolledOutDeployment("ctlog-b")
		notReady.Labels = labels
		notReady.Status.Conditions[0].Status = corev1.ConditionFalse
		cli := fakeDeploymentClient(ready, notReady)
		ok, err := DeploymentIsRunning(t.Context(), cli, "ns", labels)
		g.Expect(ok).To(gomega.BeFalse())
		g.Expect(errors.Is(err, ErrDeploymentNotAvailable)).To(gomega.BeTrue())
	})
}

func TestRemoveVolumeByName(t *testing.T) {
	type args struct {
		podSpec    *corev1.PodSpec
		volumeName string
	}
	tests := []struct {
		name   string
		args   args
		verify func(gomega.Gomega, *corev1.PodSpec)
	}{
		{
			name: "empty slice",
			args: args{
				podSpec:    &corev1.PodSpec{},
				volumeName: "volume",
			},
			verify: func(g gomega.Gomega, podSpec *corev1.PodSpec) {
				g.Expect(podSpec.Volumes).To(gomega.BeEmpty())
			},
		},
		{
			name: "remove existing volume mount",
			args: args{
				podSpec: &corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "volume",
						},
						{
							Name: "volume2",
						},
					},
				},
				volumeName: "volume",
			},
			verify: func(g gomega.Gomega, podSpec *corev1.PodSpec) {
				g.Expect(podSpec.Volumes).To(gomega.HaveLen(1))
				g.Expect(podSpec.Volumes[0].Name).To(gomega.Equal("volume2"))
			},
		},
		{
			name: "remove not existing volume mount",
			args: args{
				podSpec: &corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "volume",
						},
						{
							Name: "volume2",
						},
					},
				},
				volumeName: "volume3",
			},
			verify: func(g gomega.Gomega, podSpec *corev1.PodSpec) {
				g.Expect(podSpec.Volumes).To(gomega.HaveLen(2))
				g.Expect(podSpec.Volumes[0].Name).To(gomega.Equal("volume"))
				g.Expect(podSpec.Volumes[1].Name).To(gomega.Equal("volume2"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			RemoveVolumeByName(tt.args.podSpec, tt.args.volumeName)
			tt.verify(g, tt.args.podSpec)
		})
	}
}

func TestRemoveVolumeMountByName(t *testing.T) {
	type args struct {
		container  *corev1.Container
		volumeName string
	}
	tests := []struct {
		name   string
		args   args
		verify func(gomega.Gomega, *corev1.Container)
	}{
		{
			name: "empty slice",
			args: args{
				container:  &corev1.Container{},
				volumeName: "volume",
			},
			verify: func(g gomega.Gomega, container *corev1.Container) {
				g.Expect(container.VolumeMounts).To(gomega.BeEmpty())
			},
		},
		{
			name: "remove existing volume mount",
			args: args{
				container: &corev1.Container{
					VolumeMounts: []corev1.VolumeMount{
						{
							Name: "volume",
						},
						{
							Name: "volume2",
						},
					},
				},
				volumeName: "volume",
			},
			verify: func(g gomega.Gomega, container *corev1.Container) {
				g.Expect(container.VolumeMounts).To(gomega.HaveLen(1))
				g.Expect(container.VolumeMounts[0].Name).To(gomega.Equal("volume2"))
			},
		},
		{
			name: "remove not existing volume mount",
			args: args{
				container: &corev1.Container{
					VolumeMounts: []corev1.VolumeMount{
						{
							Name: "volume",
						},
						{
							Name: "volume2",
						},
					},
				},
				volumeName: "volume3",
			},
			verify: func(g gomega.Gomega, container *corev1.Container) {
				g.Expect(container.VolumeMounts).To(gomega.HaveLen(2))
				g.Expect(container.VolumeMounts[0].Name).To(gomega.Equal("volume"))
				g.Expect(container.VolumeMounts[1].Name).To(gomega.Equal("volume2"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			RemoveVolumeMountByName(tt.args.container, tt.args.volumeName)
			tt.verify(g, tt.args.container)
		})
	}
}
