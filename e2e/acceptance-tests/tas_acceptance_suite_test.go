package acceptance_tests

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

func TestAcceptance(t *testing.T) {
	RegisterFailHandler(Fail)
	log.SetLogger(GinkgoLogr)
	RunSpecs(t, "Trusted Artifact Signer Acceptance Tests Suite")

	format.MaxLength = 0
}
