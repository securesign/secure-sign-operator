package trillianUtils

import (
	"fmt"
	"strconv"
	"strings"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/utils"
	v1 "k8s.io/api/core/v1"
)

const (
	mysqlPasswordEnv = "MYSQL_PASSWORD"
	Port             = 3306
	Host             = "trillian-mysql"
	User             = "mysql"
	databaseName     = "trillian"
)

func EnsureDB(instance *rhtasv1alpha1.Trillian, mysqlOpts func(instance *rhtasv1alpha1.Trillian, options *MySQLOptions, container *v1.Container) error) func(*v1.Container) error {
	return func(container *v1.Container) error {
		var (
			options *MySQLOptions
			err     error
		)

		if utils.OptionalBool(instance.Spec.Db.Create) {
			// go with default mysql
			options = defaultMysqlDB(instance)
			if err := mysqlOpts(instance, options, container); err != nil {
				return fmt.Errorf("failed to configure default MySQL: %w", err)
			}
			return nil
		}

		switch instance.Spec.Db.Provider {
		case "mysql":
			options, err = ParseMySQL(instance.Spec.Db.Url)
			if err != nil {
				return fmt.Errorf("can't parse mysql url: %w", err)
			}
			if err := mysqlOpts(instance, options, container); err != nil {
				return fmt.Errorf("failed to configure MySQL: %w", err)
			}
		default:
			return fmt.Errorf("unsupported DB provider %s", instance.Spec.Db.Provider)
		}
		return nil
	}
}

func defaultMysqlDB(instance *rhtasv1alpha1.Trillian) *MySQLOptions {
	options := &MySQLOptions{
		Host:       Host,
		Port:       strconv.Itoa(Port),
		Database:   databaseName,
		User:       User,
		TlsEnabled: instance.Status.Db.TLS.CertRef != nil,
	}
	if instance.Status.Db.DatabaseSecretRef != nil {
		// use env alias to avoid plain-text printing
		options.Password = fmt.Sprintf("$(%s)", mysqlPasswordEnv)
	}
	return options
}

func ExtractEnvVarName(value string) (string, bool) {
	value = strings.TrimSpace(value)

	if value == "" {
		return "", false
	}

	// $(MYSQL_PASSWORD)
	if strings.HasPrefix(value, "$(") && strings.HasSuffix(value, ")") {
		env := strings.TrimSuffix(strings.TrimPrefix(value, "$("), ")")
		if env == "" {
			return "", false
		}
		return env, true
	}

	// $MYSQL_PASSWORD
	if strings.HasPrefix(value, "$") {
		env := strings.TrimPrefix(value, "$")
		if env == "" {
			return "", false
		}
		return env, true
	}

	return "", false
}
