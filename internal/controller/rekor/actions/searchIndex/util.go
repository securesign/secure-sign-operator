package searchIndex

import (
	"fmt"
	"strconv"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/controller/rekor/actions/searchIndex/redis"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	v1 "k8s.io/api/core/v1"
)

const redisPasswordEnv = "REDIS_PASSWORD"

func EnsureSearchIndex(instance *rhtasv1alpha1.Rekor, redisOpts func(options *redis.RedisOptions, container *v1.Container), mysqlOpts func(url string, container *v1.Container)) func(*v1.Container) error {
	return func(container *v1.Container) error {
		var (
			options *redis.RedisOptions
			err     error
		)

		if utils.OptionalBool(instance.Spec.SearchIndex.Create) {
			// go with default redis
			options = defaultSearchIndexDB(instance, container)
			redisOpts(options, container)
			return nil
		}

		switch instance.Spec.SearchIndex.Provider {
		case "redis":
			options, err = redis.Parse(instance.Spec.SearchIndex.Url)
			if err != nil {
				return fmt.Errorf("can't parse redis searchIndex url: %w", err)
			}
			redisOpts(options, container)
		case "mysql":
			mysqlOpts(instance.Spec.SearchIndex.Url, container)
		default:
			return fmt.Errorf("unsupported search_index provider %s", instance.Spec.SearchIndex.Provider)
		}
		return nil
	}
}

func defaultSearchIndexDB(instance *rhtasv1alpha1.Rekor, container *v1.Container) *redis.RedisOptions {
	options := &redis.RedisOptions{
		Host:       fmt.Sprintf("%s.%s.svc", actions.RedisDeploymentName, instance.Namespace),
		Port:       strconv.Itoa(actions.RedisDeploymentPort),
		TlsEnabled: instance.Status.SearchIndex.TLS.CertRef != nil,
	}
	if instance.Status.SearchIndex.DbPasswordRef != nil {
		// use env alias to avoid plain-text printing
		options.Password = fmt.Sprintf("$(%s)", redisPasswordEnv)

		passwordEnv := kubernetes.FindEnvByNameOrCreate(container, redisPasswordEnv)
		if passwordEnv.ValueFrom == nil {
			passwordEnv.ValueFrom = &v1.EnvVarSource{}
		}
		if passwordEnv.ValueFrom.SecretKeyRef == nil {
			passwordEnv.ValueFrom.SecretKeyRef = &v1.SecretKeySelector{}
		}
		passwordEnv.ValueFrom.SecretKeyRef.Key = instance.Status.SearchIndex.DbPasswordRef.Key
		passwordEnv.ValueFrom.SecretKeyRef.Name = instance.Status.SearchIndex.DbPasswordRef.Name
	}
	return options
}
