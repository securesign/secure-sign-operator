package ha

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestHA(t *testing.T) {
	RegisterFailHandler(Fail)
	log.SetLogger(GinkgoLogr)
	SetDefaultEventuallyTimeout(time.Duration(3) * time.Minute)
	EnforceDefaultTimeoutsWhenUsingContexts()
	RunSpecs(t, "With HA install")

	// print whole stack in case of failure
	format.MaxLength = 0
}
