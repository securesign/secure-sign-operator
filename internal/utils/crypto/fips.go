package cryptoutil

import (
	"os"
	"strings"
)

var FIPSEnabled bool

func init() {
	FIPSEnabled = IsFIPS()
}

// IsFIPS returns true when the host env is FIPS enabled
func IsFIPS() bool {
	const fipsPath = "/proc/sys/crypto/fips_enabled"

	data, err := os.ReadFile(fipsPath)
	if err != nil {
		return false
	}

	return strings.TrimSpace(string(data)) == "1"
}
