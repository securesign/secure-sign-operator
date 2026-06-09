/*
Copyright 2023.

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

// Package conversion provides test utilities for CRD conversion round-trip
// testing, following the pattern established by Cluster API's utilconversion.
package conversion

import (
	"math/rand"
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	"k8s.io/apimachinery/pkg/api/equality"
	metafuzzer "k8s.io/apimachinery/pkg/apis/meta/fuzzer"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

const fuzzIterations = 10000

// FuzzTestFuncInput contains the input parameters for FuzzTestFunc.
// Modeled after Cluster API's utilconversion.FuzzTestFuncInput.
type FuzzTestFuncInput struct {
	// Scheme used for the round-trip test. Defaults to a new empty scheme if nil.
	Scheme *runtime.Scheme

	// Hub is a zero-value instance of the hub (v1) type.
	Hub conversion.Hub

	// Spoke is a zero-value instance of the spoke (v1alpha1) type.
	Spoke conversion.Convertible

	// HubAfterMutation is called after fuzzing and before comparison to
	// normalize fields that legitimately change during conversion.
	HubAfterMutation func(conversion.Hub)

	// SpokeAfterMutation is called after fuzzing and before comparison to
	// normalize fields that legitimately change during conversion.
	SpokeAfterMutation func(conversion.Convertible)

	// FuzzerFuncs are additional custom fuzzer functions for domain types.
	FuzzerFuncs []fuzzer.FuzzerFuncs
}

// FuzzTestFunc returns a test function that runs round-trip fuzz testing
// for a hub/spoke type pair. It tests both directions:
//   - hub → spoke → hub (verifies no hub data is lost)
//   - spoke → hub → spoke (verifies no spoke data is lost)
//
// Each direction runs fuzzIterations (10,000) iterations.
func FuzzTestFunc(input FuzzTestFuncInput) func(*testing.T) {
	return func(t *testing.T) {
		t.Helper()

		scheme := input.Scheme
		if scheme == nil {
			scheme = runtime.NewScheme()
		}
		codecFactory := runtimeserializer.NewCodecFactory(scheme)

		allFuncs := make([]fuzzer.FuzzerFuncs, 0, 1+len(input.FuzzerFuncs))
		allFuncs = append(allFuncs, metafuzzer.Funcs)
		allFuncs = append(allFuncs, input.FuzzerFuncs...)
		fuzzerFuncs := fuzzer.MergeFuzzerFuncs(allFuncs...)

		t.Run("hub-spoke-hub", func(t *testing.T) {
			for i := 0; i < fuzzIterations; i++ {
				f := fuzzer.FuzzerFor(fuzzerFuncs, rand.NewSource(int64(i)), codecFactory)

				hubBefore := input.Hub.DeepCopyObject().(conversion.Hub)
				f.Fill(hubBefore)

				if input.HubAfterMutation != nil {
					input.HubAfterMutation(hubBefore)
				}

				spoke := input.Spoke.DeepCopyObject().(conversion.Convertible)
				if err := spoke.ConvertFrom(hubBefore); err != nil {
					t.Fatalf("iteration %d: hub → spoke failed: %v", i, err)
				}

				hubAfter := input.Hub.DeepCopyObject().(conversion.Hub)
				if err := spoke.ConvertTo(hubAfter); err != nil {
					t.Fatalf("iteration %d: spoke → hub failed: %v", i, err)
				}

				if !equality.Semantic.DeepEqual(hubBefore, hubAfter) {
					t.Errorf("iteration %d: hub-spoke-hub round-trip mismatch (-want +got):\n%s",
						i, cmp.Diff(hubBefore, hubAfter))
				}
			}
		})

		t.Run("spoke-hub-spoke", func(t *testing.T) {
			for i := 0; i < fuzzIterations; i++ {
				f := fuzzer.FuzzerFor(fuzzerFuncs, rand.NewSource(int64(i)), codecFactory)

				spokeBefore := input.Spoke.DeepCopyObject().(conversion.Convertible)
				f.Fill(spokeBefore)

				if input.SpokeAfterMutation != nil {
					input.SpokeAfterMutation(spokeBefore)
				}

				hub := input.Hub.DeepCopyObject().(conversion.Hub)
				if err := spokeBefore.ConvertTo(hub); err != nil {
					t.Fatalf("iteration %d: spoke → hub failed: %v", i, err)
				}

				spokeAfter := input.Spoke.DeepCopyObject().(conversion.Convertible)
				if err := spokeAfter.ConvertFrom(hub); err != nil {
					t.Fatalf("iteration %d: hub → spoke failed: %v", i, err)
				}

				if !equality.Semantic.DeepEqual(spokeBefore, spokeAfter) {
					t.Errorf("iteration %d: spoke-hub-spoke round-trip mismatch (-want +got):\n%s",
						i, cmp.Diff(spokeBefore, spokeAfter))
				}
			}
		})
	}
}
