package analyzer_test

import (
	"testing"

	"github.com/securesign/operator/pkg/analyzer"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestSpecMutation(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, analyzer.SpecMutation, "specmutation")
}
