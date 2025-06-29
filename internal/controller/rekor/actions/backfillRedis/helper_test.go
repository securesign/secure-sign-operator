package backfillredis

import (
	"testing"

	"github.com/onsi/gomega"
)

func TestEnvAsShellParams(t *testing.T) {
	g := gomega.NewWithT(t)
	t.Run("No env var replacement", func(t *testing.T) {
		str := "$_(_)"
		g.Expect(envAsShellParams(str)).To(gomega.Equal(str))
	})

	t.Run("Single env var replacement", func(t *testing.T) {
		g.Expect(envAsShellParams("$(PASSWORD)")).To(gomega.Equal("$PASSWORD"))
	})

	t.Run("Multiple env var replacement", func(t *testing.T) {
		g.Expect(envAsShellParams("mysql://pas$wo()@$(FIRST):?$(SECOND)")).To(gomega.Equal("mysql://pas$wo()@$FIRST:?$SECOND"))
	})
}
