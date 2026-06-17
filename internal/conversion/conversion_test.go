/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Adapted from sigs.k8s.io/cluster-api/util/conversion.

package conversion

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"

	rhtasv1 "github.com/securesign/operator/api/v1"
)

var (
	oldSecuresignGVK = schema.GroupVersionKind{
		Group:   rhtasv1.GroupVersion.Group,
		Version: "v1old",
		Kind:    "Securesign",
	}
)

func TestMarshalData(t *testing.T) {
	t.Run("MarshalData should write source object to destination", func(*testing.T) {
		g := NewWithT(t)

		src := &rhtasv1.Securesign{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
				Labels: map[string]string{
					"label1": "",
				},
			},
			Spec: rhtasv1.SecuresignSpec{
				Rekor: rhtasv1.RekorSpec{
					Signer: rhtasv1.RekorSigner{KMS: "secret"},
					TreeID: ptr.To[int64](12345),
				},
			},
		}

		dst := &unstructured.Unstructured{}
		dst.SetGroupVersionKind(oldSecuresignGVK)
		dst.SetName("test-1")

		g.Expect(MarshalData(src, dst)).To(Succeed())
		// ensure the src object is not modified
		g.Expect(src.GetLabels()).ToNot(BeEmpty())

		g.Expect(dst.GetAnnotations()[DataAnnotation]).ToNot(BeEmpty())
		g.Expect(dst.GetAnnotations()[DataAnnotation]).To(ContainSubstring("secret"))
		g.Expect(dst.GetAnnotations()[DataAnnotation]).To(ContainSubstring("12345"))
		g.Expect(dst.GetAnnotations()[DataAnnotation]).ToNot(ContainSubstring("metadata"))
		g.Expect(dst.GetAnnotations()[DataAnnotation]).ToNot(ContainSubstring("label1"))
	})

	t.Run("MarshalDataUnsafeNoCopy should write source object to destination", func(t *testing.T) {
		g := NewWithT(t)

		src := &rhtasv1.Securesign{
			ObjectMeta: metav1.ObjectMeta{
				Name:                       "test-1",
				GenerateName:               "test-",
				Namespace:                  "default",
				SelfLink:                   "test",
				UID:                        "123456",
				ResourceVersion:            "123",
				Generation:                 15,
				CreationTimestamp:          metav1.Now(),
				DeletionTimestamp:          ptr.To(metav1.Now()),
				DeletionGracePeriodSeconds: ptr.To[int64](10),
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "test/v1beta1",
					Kind:       "TestKind",
					Name:       "name",
					UID:        "1234567",
				}},
				Finalizers: []string{"finalizer"},
				ManagedFields: []metav1.ManagedFieldsEntry{
					{
						Manager: "test-manager",
					},
				},
				Labels: map[string]string{
					"label1": "",
				},
				Annotations: map[string]string{
					"annotation1": "",
				},
			},
			Spec: rhtasv1.SecuresignSpec{
				Rekor: rhtasv1.RekorSpec{
					Signer: rhtasv1.RekorSigner{KMS: "secret"},
					TreeID: ptr.To[int64](12345),
				},
			},
		}

		dst := &unstructured.Unstructured{}
		dst.SetGroupVersionKind(oldSecuresignGVK)
		dst.SetName("test-1")

		g.Expect(MarshalDataUnsafeNoCopy(src, dst)).To(Succeed())

		g.Expect(dst.GetAnnotations()[DataAnnotation]).ToNot(BeEmpty())
		g.Expect(dst.GetAnnotations()[DataAnnotation]).To(ContainSubstring("secret"))
		g.Expect(dst.GetAnnotations()[DataAnnotation]).To(ContainSubstring("12345"))
		g.Expect(dst.GetAnnotations()[DataAnnotation]).ToNot(ContainSubstring("metadata"))
		g.Expect(dst.GetAnnotations()[DataAnnotation]).ToNot(ContainSubstring("label1"))
	})

	t.Run("MarshalData should append the annotation", func(*testing.T) {
		g := NewWithT(t)

		src := &rhtasv1.Securesign{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
			},
		}
		dst := &unstructured.Unstructured{}
		dst.SetGroupVersionKind(rhtasv1.GroupVersion.WithKind("Securesign"))
		dst.SetName("test-1")
		dst.SetAnnotations(map[string]string{
			"annotation": "1",
		})

		g.Expect(MarshalData(src, dst)).To(Succeed())
		g.Expect(dst.GetAnnotations()).To(HaveLen(2))
	})

	t.Run("MarshalDataUnsafeNoCopy should append the annotation", func(*testing.T) {
		g := NewWithT(t)

		src := &rhtasv1.Securesign{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
			},
		}
		dst := &unstructured.Unstructured{}
		dst.SetGroupVersionKind(rhtasv1.GroupVersion.WithKind("Securesign"))
		dst.SetName("test-1")
		dst.SetAnnotations(map[string]string{
			"annotation": "1",
		})

		g.Expect(MarshalDataUnsafeNoCopy(src, dst)).To(Succeed())
		g.Expect(dst.GetAnnotations()).To(HaveLen(2))
	})
}

func TestUnmarshalData(t *testing.T) {
	g := NewWithT(t)

	t.Run("should return false without errors if annotation doesn't exist", func(*testing.T) {
		src := &rhtasv1.Securesign{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
			},
		}
		dst := &unstructured.Unstructured{}
		dst.SetGroupVersionKind(oldSecuresignGVK)
		dst.SetName("test-1")

		ok, err := UnmarshalData(src, dst)
		g.Expect(ok).To(BeFalse())
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should return true when a valid annotation with data exists", func(*testing.T) {
		src := &unstructured.Unstructured{}
		src.SetGroupVersionKind(oldSecuresignGVK)
		src.SetName("test-1")
		src.SetAnnotations(map[string]string{
			DataAnnotation: `{"metadata":{"name":"test-1","creationTimestamp":null,"labels":{"label1":""}},"spec":{"rekor":{"signer":{"kms":"secret"}}}}`,
		})

		dst := &rhtasv1.Securesign{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
			},
		}

		ok, err := UnmarshalData(src, dst)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ok).To(BeTrue())

		g.Expect(dst.GetLabels()).To(HaveLen(1))
		g.Expect(dst.GetName()).To(Equal("test-1"))
		g.Expect(dst.GetLabels()).To(HaveKeyWithValue("label1", ""))
		g.Expect(dst.GetAnnotations()).To(BeEmpty())
	})

	t.Run("should clean the annotation on successful unmarshal", func(*testing.T) {
		src := &unstructured.Unstructured{}
		src.SetGroupVersionKind(oldSecuresignGVK)
		src.SetName("test-1")
		src.SetAnnotations(map[string]string{
			"annotation-1": "",
			DataAnnotation: `{"metadata":{"name":"test-1","creationTimestamp":null,"labels":{"label1":""}},"spec":{"rekor":{"signer":{"kms":"secret"}}}}`,
		})

		dst := &rhtasv1.Securesign{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-1",
			},
		}

		ok, err := UnmarshalData(src, dst)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ok).To(BeTrue())

		g.Expect(src.GetAnnotations()).ToNot(HaveKey(DataAnnotation))
		g.Expect(src.GetAnnotations()).To(HaveLen(1))
	})
}
