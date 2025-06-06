package utils

import (
	"errors"
	"fmt"
	"testing"

	"github.com/onsi/gomega"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"k8s.io/utils/ptr"
)

func TestSearchIndexParams(t *testing.T) {
	gomega.RegisterTestingT(t)
	tests := []struct {
		name        string
		searchIndex rhtasv1alpha1.SearchIndex
		output      []string
		err         error
	}{
		{
			name:        "default value",
			searchIndex: rhtasv1alpha1.SearchIndex{Create: ptr.To(true)},
			output: []string{
				"--redis_server.address=rekor-redis",
				"--redis_server.port=6379",
			},
		},

		{
			name: "external redis",
			searchIndex: rhtasv1alpha1.SearchIndex{
				Create:   ptr.To(false),
				Provider: "redis",
				Url:      "redis://:password@my-redis-server:9999",
			},
			output: []string{
				"--redis_server.address=my-redis-server",
				"--redis_server.port=9999",
				"--redis_server.password=password",
			},
		},
		{
			name: "external mysql",
			searchIndex: rhtasv1alpha1.SearchIndex{
				Create:   ptr.To(false),
				Url:      "mysql://user:password@tcp(mysql:3306)/test",
				Provider: "mysql",
			},
			output: []string{
				"--search_index.mysql.dsn=mysql://user:password@tcp(mysql:3306)/test",
			},
		},
		{
			name: "unsupported provider",
			searchIndex: rhtasv1alpha1.SearchIndex{
				Create:   ptr.To(false),
				Provider: "fake",
				Url:      "fake://:password@my-redis-server:9999",
			},
			err: fmt.Errorf("unsupported search_index provider %s", "fake"),
		},
		{
			name: "missing host",
			searchIndex: rhtasv1alpha1.SearchIndex{
				Create:   ptr.To(false),
				Provider: "redis",
				Url:      "redis://:password@:9999",
			},
			err: errors.New("searchIndex url host is empty"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := rhtasv1alpha1.Rekor{
				Spec: rhtasv1alpha1.RekorSpec{
					SearchIndex: tt.searchIndex,
				},
			}

			got, err := SearchIndexParams(instance, NewSearchIndexParameterMap("redis_server.address", "redis_server.port", "redis_server.password", "search_index.mysql.dsn"))
			if tt.err != nil {
				gomega.Expect(err).To(gomega.MatchError(tt.err))
			}
			gomega.Expect(got).To(gomega.Equal(tt.output))
		})
	}
}
