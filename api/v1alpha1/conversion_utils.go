package v1alpha1

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
)

func marshalConvert(src, dst runtime.Object) error {
	dstGVK := dst.GetObjectKind().GroupVersionKind()

	data, err := json.Marshal(src)
	if err != nil {
		return fmt.Errorf("failed to marshal source: %w", err)
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("failed to unmarshal into destination: %w", err)
	}

	dst.GetObjectKind().SetGroupVersionKind(dstGVK)
	return nil
}
