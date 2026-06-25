//go:build integration

package install

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestInstall(t *testing.T) {
	t.Setenv("TUF_ROOT", t.TempDir())
	RegisterFailHandler(Fail)
	log.SetLogger(GinkgoLogr)
	SetDefaultEventuallyTimeout(time.Duration(3) * time.Minute)
	SetDefaultEventuallyPollingInterval(1 * time.Second)
	EnforceDefaultTimeoutsWhenUsingContexts()
	RunSpecs(t, "Install E2E Suite")

	// print whole stack in case of failure
	format.MaxLength = 0
}
