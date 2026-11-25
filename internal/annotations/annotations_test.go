package annotations

import (
	"testing"

	"github.com/onsi/gomega"
)

func TestHAProxyThrottlingDefaults(t *testing.T) {
	t.Parallel()
	g := gomega.NewWithT(t)

	result := HAProxyThrottlingDefaults(100, 50, 200)

	g.Expect(result).To(gomega.HaveLen(4))
	g.Expect(result[HAProxyRateLimitEnabled]).To(gomega.Equal("true"))
	g.Expect(result[HAProxyRateLimitConcurrentTCP]).To(gomega.Equal("100"))
	g.Expect(result[HAProxyRateLimitRateHTTP]).To(gomega.Equal("50"))
	g.Expect(result[HAProxyRateLimitRateTCP]).To(gomega.Equal("200"))
}
