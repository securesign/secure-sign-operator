package trillianUtils

import (
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"

	"github.com/go-sql-driver/mysql"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/controller/trillian/dbsecret"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	core "k8s.io/api/core/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

const WaitForDBInitContainer = "wait-for-trillian-db"

func EnsureDB(instance *rhtasv1.Trillian, containerName, caPath string) traits.Trait[*corev1ac.PodSpecApplyConfiguration] {
	return func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
		container := kubernetes.FindContainerByName(podSpec, containerName)
		switch instance.Spec.Db.Provider {
		case "mysql":
			uri, err := mysql.ParseDSN(instance.Spec.Db.Uri)
			if err != nil {
				return fmt.Errorf("can't parse db uri: %w", err)
			}
			host, port, err := net.SplitHostPort(uri.Addr)
			if err != nil {
				return err
			}
			if port == "" {
				port = "3306"
			}
			container.WithArgs("--mysql_uri", instance.Spec.Db.Uri, "--mysql_max_conns", "30", "--mysql_max_idle_conns", "10")
			if UseTLSDb(instance) {
				//DB secret is hostname created WITHOUT the .svc suffix (can't change for backward compat) that does not match TLS hostname
				if utils.OptionalBool(instance.Spec.Db.Create) && !strings.HasSuffix(host, fmt.Sprintf(".%s.svc", instance.Namespace)) {
					host = fmt.Sprintf("%s.%s.svc", host, instance.Namespace)
				}
				container.WithArgs("--mysql_tls_ca", caPath, "--mysql_server_name", host)
			}
			initContainer := corev1ac.Container().WithName(WaitForDBInitContainer).
				WithImage(images.Registry.Get(images.TrillianNetcat)).WithCommand("sh", "-c").
				WithArgs(`until nc -z -v -w30 $1 $2; do echo "Waiting for MySQL to start"; sleep 5; done;`, "inlineScript", host, port).
				WithSecurityContext(traits.SSAContainerSecurityContext())
			podSpec.WithInitContainers(initContainer)
		case "postgresql":
			container.WithArgs("--postgresql_uri", instance.Spec.Db.Uri)
			if UseTLSDb(instance) {
				container.WithArgs("--postgresql_tls_ca", caPath)
			}
		default:
			return errors.New("unsupported DB provider")
		}
		return nil
	}
}

// WithDbAuth adds database credentials to a copy of auth, replacing matching env
// vars by name. The root password is excluded.
func WithDbAuth(auth *rhtasv1.Auth, databaseSecretRef *rhtasv1.LocalObjectReference) *rhtasv1.Auth {
	merged := auth.DeepCopy()
	if databaseSecretRef == nil {
		return merged
	}
	if merged == nil {
		merged = &rhtasv1.Auth{}
	}
	for _, env := range dbsecret.ConnectionEnv(databaseSecretRef.Name) {
		index := slices.IndexFunc(merged.Env, func(existing core.EnvVar) bool {
			return existing.Name == env.Name
		})
		if index >= 0 {
			merged.Env[index] = env
		} else {
			merged.Env = append(merged.Env, env)
		}
	}
	return merged
}
