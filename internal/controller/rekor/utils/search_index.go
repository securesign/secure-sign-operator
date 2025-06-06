package utils

import (
	"fmt"
	"net/url"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	utils2 "github.com/securesign/operator/internal/utils"
)

type parameterMap struct {
	redisHost, redisPort, redisPassword, mysqlDsn string
}

func NewSearchIndexParameterMap(redisHost, redisPort, redisPassword, mysqlDsn string) parameterMap {
	return parameterMap{
		redisHost:     redisHost,
		redisPort:     redisPort,
		redisPassword: redisPassword,
		mysqlDsn:      mysqlDsn,
	}
}

func SearchIndexParams(instance rhtasv1alpha1.Rekor, parameters parameterMap) ([]string, error) {
	if utils2.OptionalBool(instance.Spec.SearchIndex.Create) {
		// go with defaults
		return []string{
			fmt.Sprintf("--%s=rekor-redis", parameters.redisHost),
			fmt.Sprintf("--%s=6379", parameters.redisPort),
		}, nil
	}

	switch instance.Spec.SearchIndex.Provider {
	case "redis":
		searchIndexUrl, err := url.Parse(instance.Spec.SearchIndex.Url)
		if err != nil {
			return nil, fmt.Errorf("can't parse searchIndex url: %w", err)
		}
		if searchIndexUrl.Hostname() == "" {
			return nil, fmt.Errorf("searchIndex url host is empty")
		}
		args := []string{
			fmt.Sprintf("--%s=%s", parameters.redisHost, searchIndexUrl.Hostname()),
		}

		if searchIndexUrl.Port() != "" {
			args = append(args, fmt.Sprintf("--%s=%s", parameters.redisPort, searchIndexUrl.Port()))
		}

		if searchIndexUrl.User != nil {
			if p, ok := searchIndexUrl.User.Password(); ok {
				args = append(args, fmt.Sprintf("--%s=%s", parameters.redisPassword, p))
			}
		}
		return args, nil
	case "mysql":
		return []string{
			fmt.Sprintf("--%s=%s", parameters.mysqlDsn, instance.Spec.SearchIndex.Url),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported search_index provider %s", instance.Spec.SearchIndex.Provider)
	}
}
