package searchIndex

import (
	"fmt"
	"strconv"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/controller/rekor/actions/searchIndex/redis"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	v1 "k8s.io/api/core/v1"
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

func EnsureSearchIndex(instance *rhtasv1.Rekor, redisOpts func(*redis.RedisOptions, *v1.Container), mysqlOpts func(string, *v1.Container)) func(*v1.Container) error {
	return func(container *v1.Container) error {
		options, err := Resolve(instance)
		if err != nil {
			return err
		}
		if ref := options.PasswordRef; ref != nil {
			passwordEnv := kubernetes.FindEnvByNameOrCreate(container, RedisPasswordEnv)
			passwordEnv.ValueFrom = &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{Name: ref.Name},
					Key:                  ref.Key,
				},
			}
		}
		if options.Redis != nil {
			redisOpts(options.Redis, container)
		} else {
			mysqlOpts(options.MySQLDSN, container)
		}
		return nil
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
