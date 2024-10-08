package actions

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (g generateSigner) validateExistingObject(obj metav1.Object, instance *v1alpha1.TimestampAuthority) bool {
	expectedAnnotations := g.computeObjectAnnotations(instance)
	for key, expectedValue := range expectedAnnotations {
		if value, exists := obj.GetAnnotations()[key]; !exists || value != expectedValue {
			return false
		}
	}
	return true
}

func (g generateSigner) computeObjectAnnotations(instance *v1alpha1.TimestampAuthority) map[string]string {
	annotations := make(map[string]string)
	specHash, err := computeSpecHash(instance.Spec.Signer)
	if err != nil {
		g.Logger.Error(err, "Error computing spec hash")
		return annotations
	}
	annotations[constants.LabelNamespace+"/specHash"] = specHash
	return annotations
}

func computeSpecHash(spec interface{}) (string, error) {
	specBytes, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(specBytes)
	return fmt.Sprintf("%x", hash), nil
}
