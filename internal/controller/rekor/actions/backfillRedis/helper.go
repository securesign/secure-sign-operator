package backfillredis

import (
	"regexp"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/utils"
)

func enabled(instance *v1alpha1.Rekor) bool {
	return utils.OptionalBool(instance.Spec.BackFillRedis.Enabled)
}

func envAsShellParams(option string) string {
	// we must transfer ENV patterns from $(ENV) to $ENV to be correctly interpreted by shell
	re := regexp.MustCompile(`\$\((.*?)\)`)
	replacement := `$$$1` //$ + first matching group

	return re.ReplaceAllString(option, replacement)
}
