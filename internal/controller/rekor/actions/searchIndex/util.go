package searchIndex

import (
	"fmt"
	"strconv"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/controller/rekor/actions/searchIndex/redis"
	"github.com/securesign/operator/internal/utils"
)

const RedisPasswordEnv = "REDIS_PASSWORD"

// Options describes a search index independently of its container representation.
type Options struct {
	Redis       *redis.RedisOptions
	MySQLDSN    string
	PasswordRef *rhtasv1.SecretKeySelector
}

func Resolve(instance *rhtasv1.Rekor) (*Options, error) {
	if utils.OptionalBool(instance.Spec.SearchIndex.Create) {
		return &Options{
			Redis:       defaultSearchIndexDB(instance),
			PasswordRef: instance.Status.SearchIndex.DbPasswordRef,
		}, nil
	}
	switch instance.Spec.SearchIndex.Provider {
	case "redis":
		options, err := redis.Parse(instance.Spec.SearchIndex.URL)
		if err != nil {
			return nil, fmt.Errorf("can't parse redis searchIndex url: %w", err)
		}
		return &Options{Redis: options}, nil
	case "mysql":
		return &Options{MySQLDSN: instance.Spec.SearchIndex.URL}, nil
	default:
		return nil, fmt.Errorf("unsupported search_index provider %s", instance.Spec.SearchIndex.Provider)
	}
}

func defaultSearchIndexDB(instance *rhtasv1.Rekor) *redis.RedisOptions {
	options := &redis.RedisOptions{
		Host:       fmt.Sprintf("%s.%s.svc", actions.RedisDeploymentName, instance.Namespace),
		Port:       strconv.Itoa(actions.RedisDeploymentPort),
		TlsEnabled: instance.Status.SearchIndex.TLS.CertRef != nil,
	}
	if instance.Status.SearchIndex.DbPasswordRef != nil {
		// use env alias to avoid plain-text printing
		options.Password = fmt.Sprintf("$(%s)", RedisPasswordEnv)
	}
	return options
}
