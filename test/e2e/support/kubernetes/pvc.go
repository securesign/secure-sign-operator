package kubernetes

import (
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func CreateTestPVC(name, namespace string) *v1.PersistentVolumeClaim {
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{
				v1.ReadWriteMany,
			},
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse("100Mi"),
				},
			},
			StorageClassName: ptr.To("nfs-csi"),
		},
	}
	return pvc
}

func CreatePVCCopyJob(namespace, srcPVC, destPVC string) *batchv1.Job {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "pvc-copy-",
			Namespace:    namespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            ptr.To(int32(1)),
			TTLSecondsAfterFinished: ptr.To(int32(600)),
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					RestartPolicy: v1.RestartPolicyOnFailure,
					Containers: []v1.Container{
						{
							Name:    "pvc-copy",
							Image:   "registry.redhat.io/openshift4/ose-cli:latest",
							Command: []string{"/bin/bash", "-lc"},
							Args: []string{`
								set -euo pipefail
								echo "Copying from /src to /dest..."
								rsync -rlH --no-perms --no-owner --no-group --numeric-ids /src/ /dest/
								echo "Done."`,
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "src",
									MountPath: "/src",
									ReadOnly:  true,
								},
								{
									Name:      "dest",
									MountPath: "/dest",
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "src",
							VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
									ClaimName: srcPVC,
									ReadOnly:  true},
							},
						},
						{
							Name: "dest",
							VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
									ClaimName: destPVC,
								},
							}},
					},
				},
			},
		},
	}
	return job
}

// Mimic docs/copy-pvc-to-pvc-data.md
