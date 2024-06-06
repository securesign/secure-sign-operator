package cli

import (
	"os/exec"

	"github.com/onsi/ginkgo/v2/dsl/core"
)

func Execute(command string, args ...string) error {
	cmd := exec.Command(command, args...)
	cmd.Stderr = core.GinkgoWriter
	cmd.Stdout = core.GinkgoWriter
	return cmd.Run()
}
