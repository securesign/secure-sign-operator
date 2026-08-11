//go:build pkcs11

package pkcs11

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestPKCS11(t *testing.T) {
	t.Setenv("TUF_ROOT", t.TempDir())
	RegisterFailHandler(Fail)
	log.SetLogger(GinkgoLogr)
	SetDefaultEventuallyTimeout(5 * time.Minute)
	SetDefaultEventuallyPollingInterval(1 * time.Second)
	EnforceDefaultTimeoutsWhenUsingContexts()
	RunSpecs(t, "PKCS#11 E2E Suite")

	// print whole stack in case of failure
	format.MaxLength = 0
}
