package utils

import (
	"testing"

	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
)

func TestMergeImagePullSecrets(t *testing.T) {
	tests := []struct {
		name     string
		base     []v1.LocalObjectReference
		override []v1.LocalObjectReference
		verify   func(Gomega, []v1.LocalObjectReference)
	}{
		{
			name:     "both lists empty",
			base:     []v1.LocalObjectReference{},
			override: []v1.LocalObjectReference{},
			verify: func(g Gomega, result []v1.LocalObjectReference) {
				g.Expect(result).To(BeNil())
			},
		},
		{
			name:     "both lists nil",
			base:     nil,
			override: nil,
			verify: func(g Gomega, result []v1.LocalObjectReference) {
				g.Expect(result).To(BeNil())
			},
		},
		{
			name: "only base has secrets",
			base: []v1.LocalObjectReference{
				{Name: "base-secret-1"},
				{Name: "base-secret-2"},
			},
			override: nil,
			verify: func(g Gomega, result []v1.LocalObjectReference) {
				g.Expect(result).To(HaveLen(2))
				g.Expect(result).To(ContainElement(v1.LocalObjectReference{Name: "base-secret-1"}))
				g.Expect(result).To(ContainElement(v1.LocalObjectReference{Name: "base-secret-2"}))
			},
		},
		{
			name: "only override has secrets",
			base: nil,
			override: []v1.LocalObjectReference{
				{Name: "override-secret-1"},
				{Name: "override-secret-2"},
			},
			verify: func(g Gomega, result []v1.LocalObjectReference) {
				g.Expect(result).To(HaveLen(2))
				g.Expect(result).To(ContainElement(v1.LocalObjectReference{Name: "override-secret-1"}))
				g.Expect(result).To(ContainElement(v1.LocalObjectReference{Name: "override-secret-2"}))
			},
		},
		{
			name: "both have different secrets",
			base: []v1.LocalObjectReference{
				{Name: "base-secret"},
			},
			override: []v1.LocalObjectReference{
				{Name: "override-secret"},
			},
			verify: func(g Gomega, result []v1.LocalObjectReference) {
				g.Expect(result).To(HaveLen(2))
				g.Expect(result).To(ContainElement(v1.LocalObjectReference{Name: "base-secret"}))
				g.Expect(result).To(ContainElement(v1.LocalObjectReference{Name: "override-secret"}))
			},
		},
		{
			name: "both have same secret - deduplication",
			base: []v1.LocalObjectReference{
				{Name: "shared-secret"},
			},
			override: []v1.LocalObjectReference{
				{Name: "shared-secret"},
			},
			verify: func(g Gomega, result []v1.LocalObjectReference) {
				g.Expect(result).To(HaveLen(1))
				g.Expect(result).To(ContainElement(v1.LocalObjectReference{Name: "shared-secret"}))
			},
		},
		{
			name: "filters out empty names",
			base: []v1.LocalObjectReference{
				{Name: "base-secret"},
				{Name: ""},
			},
			override: []v1.LocalObjectReference{
				{Name: "override-secret"},
				{Name: ""},
			},
			verify: func(g Gomega, result []v1.LocalObjectReference) {
				g.Expect(result).To(HaveLen(2))
				g.Expect(result).To(ContainElement(v1.LocalObjectReference{Name: "base-secret"}))
				g.Expect(result).To(ContainElement(v1.LocalObjectReference{Name: "override-secret"}))
			},
		},
		{
			name: "merge with overlapping secrets",
			base: []v1.LocalObjectReference{
				{Name: "base-secret-1"},
				{Name: "shared-secret"},
				{Name: "base-secret-2"},
			},
			override: []v1.LocalObjectReference{
				{Name: "override-secret-1"},
				{Name: "shared-secret"},
				{Name: "override-secret-2"},
			},
			verify: func(g Gomega, result []v1.LocalObjectReference) {
				g.Expect(result).To(HaveLen(5))
				g.Expect(result).To(ContainElement(v1.LocalObjectReference{Name: "base-secret-1"}))
				g.Expect(result).To(ContainElement(v1.LocalObjectReference{Name: "base-secret-2"}))
				g.Expect(result).To(ContainElement(v1.LocalObjectReference{Name: "shared-secret"}))
				g.Expect(result).To(ContainElement(v1.LocalObjectReference{Name: "override-secret-1"}))
				g.Expect(result).To(ContainElement(v1.LocalObjectReference{Name: "override-secret-2"}))
			},
		},
		{
			name: "all empty names returns nil",
			base: []v1.LocalObjectReference{
				{Name: ""},
			},
			override: []v1.LocalObjectReference{
				{Name: ""},
			},
			verify: func(g Gomega, result []v1.LocalObjectReference) {
				g.Expect(result).To(BeEmpty())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			result := MergeImagePullSecrets(tt.base, tt.override)
			tt.verify(g, result)
		})
	}
}
