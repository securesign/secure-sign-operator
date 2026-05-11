package analyzer_test

import (
	"testing"

	"github.com/securesign/operator/pkg/analyzer"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPersistStatusErr(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, analyzer.PersistStatusErr, "persiststatuserr")
}
