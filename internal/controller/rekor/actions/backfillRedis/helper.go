package backfillredis

import (
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/utils"
)

func enabled(instance *v1alpha1.Rekor) bool {
	return utils.OptionalBool(instance.Spec.BackFillRedis.Enabled)
}
