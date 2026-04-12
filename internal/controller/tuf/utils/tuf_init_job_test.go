package utils

import (
	"testing"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEnsureTufInitJob_NilRootKeySecretRef(t *testing.T) {
	instance := &rhtasv1alpha1.Tuf{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tuf",
			Namespace: "test-ns",
		},
		Spec: rhtasv1alpha1.TufSpec{
			Port: 8080,
			Keys: []rhtasv1alpha1.TufKey{
				{
					Name: "rekor.pub",
					SecretRef: &rhtasv1alpha1.SecretKeySelector{
						LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "rekor-pub-key"},
						Key:                  "public",
					},
				},
			},
			RootKeySecretRef: nil,
		},
	}

	job := &batchv1.Job{}
	ensureFn := EnsureTufInitJob(instance, "test-sa", map[string]string{"app": "tuf"}, nil)
	err := ensureFn(job)
	if err == nil {
		t.Fatal("expected error when RootKeySecretRef is nil, got nil")
	}
	if err.Error() != "rootKeySecretRef is not set" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestEnsureTufInitJob_WithRootKeySecretRef(t *testing.T) {
	instance := &rhtasv1alpha1.Tuf{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tuf",
			Namespace: "test-ns",
		},
		Spec: rhtasv1alpha1.TufSpec{
			SigningConfigURLMode: rhtasv1alpha1.SigningConfigURLInternal,
			Port:                 8080,
			Keys: []rhtasv1alpha1.TufKey{
				{
					Name: "rekor.pub",
					SecretRef: &rhtasv1alpha1.SecretKeySelector{
						LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "rekor-pub-key"},
						Key:                  "public",
					},
				},
			},
			RootKeySecretRef: &rhtasv1alpha1.LocalObjectReference{Name: "tuf-root-keys"},
		},
		Status: rhtasv1alpha1.TufStatus{
			Keys: []rhtasv1alpha1.TufKey{
				{
					Name: "rekor.pub",
					SecretRef: &rhtasv1alpha1.SecretKeySelector{
						LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: "rekor-pub-key"},
						Key:                  "public",
					},
				},
			},
			PvcName: "tuf-pvc",
		},
	}

	job := &batchv1.Job{}
	ensureFn := EnsureTufInitJob(instance, "test-sa", map[string]string{"app": "tuf"}, nil)
	err := ensureFn(job)
	if err != nil {
		t.Fatalf("unexpected error when RootKeySecretRef is set: %v", err)
	}

	// Verify the job was configured with the correct export-keys argument
	container := job.Spec.Template.Spec.Containers[0]
	found := false
	for _, arg := range container.Args {
		if arg != "" && len(arg) > 0 {
			if contains(arg, "--export-keys tuf-root-keys") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("expected --export-keys tuf-root-keys in args, got: %v", container.Args)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
