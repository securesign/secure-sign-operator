//go:build !race

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

package v1

import (
	"testing"

	"github.com/securesign/operator/api/v1alpha1"
	utilconversion "github.com/securesign/operator/internal/testing/conversion"
)

func TestSecuresignConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &Securesign{},
		Spoke: &v1alpha1.Securesign{},
	}))
}

func TestCTlogConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &CTlog{},
		Spoke: &v1alpha1.CTlog{},
	}))
}

func TestRekorConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &Rekor{},
		Spoke: &v1alpha1.Rekor{},
	}))
}

func TestFulcioConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &Fulcio{},
		Spoke: &v1alpha1.Fulcio{},
	}))
}

func TestTrillianConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &Trillian{},
		Spoke: &v1alpha1.Trillian{},
	}))
}

func TestTufConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &Tuf{},
		Spoke: &v1alpha1.Tuf{},
	}))
}

func TestTimestampAuthorityConversion(t *testing.T) {
	t.Run("roundtrip", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &TimestampAuthority{},
		Spoke: &v1alpha1.TimestampAuthority{},
	}))
}
