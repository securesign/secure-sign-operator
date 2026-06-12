package api

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/go-sql-driver/mysql"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/console/actions"
	consoleUtils "github.com/securesign/operator/internal/controller/console/utils"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
)

const initContainerName = "wait-for-console-db-tuf"

func ensureDbAuth(instance *rhtasv1.Console, containerName string) []func(dp *apps.Deployment) error {
	return []func(dp *apps.Deployment) error{
		// ensure user auth
		func(deploy *apps.Deployment) error {
			ref := &deploy.Spec.Template.Spec
			err := ensure.ContainerAuth(kubernetes.FindContainerByNameOrCreate(ref, containerName), instance.Spec.Auth)(ref)
			return err
		},

		// ensure dbSecret auth
		ensure.Optional(instance.Status.Db.DatabaseSecretRef != nil,
			func(deploy *apps.Deployment) error {
				ref := &deploy.Spec.Template.Spec
				err := ensure.ContainerAuth(kubernetes.FindContainerByNameOrCreate(ref, containerName), dbSecretToAuth(instance.Status.Db.DatabaseSecretRef))(ref)
				return err
			}),
	}
}

func dbSecretToAuth(databaseSecretRef *rhtasv1.LocalObjectReference) *rhtasv1.Auth {
	auth := rhtasv1.Auth{}
	keys := []string{actions.SecretUser, actions.SecretPassword, actions.SecretHost, actions.SecretPort, actions.SecretDatabaseName}

	for _, v := range keys {
		temp := strings.ReplaceAll(v, "-", "_")
		temp = strings.ToUpper(temp)

		auth.Env = append(auth.Env, core.EnvVar{
			Name: temp,
			ValueFrom: &core.EnvVarSource{
				SecretKeyRef: &core.SecretKeySelector{
					Key: v,
					LocalObjectReference: core.LocalObjectReference{
						Name: databaseSecretRef.Name,
					},
				},
			},
		})
	}
	return &auth
}

func ensureDB(instance *rhtasv1.Console, containerName, caPath, tufURL string) []func(*apps.Deployment) error {
	return append(ensureDbAuth(instance, containerName), ensureDbParams(instance, containerName, consoleUtils.UseTLSDb(instance), caPath, tufURL))
}

func ensureDbParams(instance *rhtasv1.Console, containerName string, useTls bool, caPath, tufURL string) func(dp *apps.Deployment) error {
	return func(dp *apps.Deployment) error {
		container := kubernetes.FindContainerByNameOrCreate(&dp.Spec.Template.Spec, containerName)

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
				// mysql default port
				port = "3306"
			}

			container.Args = append(container.Args, "--mysql-uri", instance.Spec.Db.Uri)

			if useTls {
				if utils.OptionalBool(instance.Spec.Db.Create) && !strings.HasSuffix(host, fmt.Sprintf(".%s.svc", instance.Namespace)) {
					host = fmt.Sprintf("%s.%s.svc", host, instance.Namespace)
				}

				container.Args = append(container.Args, "--db-tls-ca", caPath, "--db-tls-server-name", host)
			}

			initContainer := kubernetes.FindInitContainerByNameOrCreate(&dp.Spec.Template.Spec, initContainerName)
			initContainer.Image = images.Registry.Get(images.TrillianNetcat)
			initContainer.Command = []string{"sh", "-c"}
			initContainer.Args = []string{
				fmt.Sprintf(`
                    echo "Waiting for console database...";
                    until nc -z -v -w30 $1 $2; do
                        echo "Waiting for MySQL to start";
                        sleep 5;
                    done;
                    echo "Waiting for TUF server...";
                    until curl %s > /dev/null 2>&1; do
                        echo "TUF server not ready...";
                        sleep 5;
                    done;
                    echo "tuf-init completed."
                `, tufURL),
				"inlineScript",
				host,
				port,
			}

			ref := &dp.Spec.Template.Spec
			err = ensure.ContainerAuth(initContainer, instance.Spec.Auth)(ref)
			if err != nil {
				return err
			}
			// ensure dbSecret auth
			if instance.Status.Db.DatabaseSecretRef != nil {
				err = ensure.ContainerAuth(initContainer, dbSecretToAuth(instance.Status.Db.DatabaseSecretRef))(ref)
				if err != nil {
					return err
				}
			}

		case "postgresql":
			container.Args = append(container.Args, "--postgresql-uri", instance.Spec.Db.Uri)

			if useTls {
				container.Args = append(container.Args, "--db-tls-ca", caPath)
			}

		default:
			return errors.New("unsupported DB provider")
		}

		return nil
	}
}
