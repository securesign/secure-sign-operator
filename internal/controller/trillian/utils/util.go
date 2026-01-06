package trillianUtils

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	v1 "k8s.io/api/core/v1"
)

const (
	Port             = 3306
	Host             = "trillian-mysql"
	User             = "mysql"
	databaseName     = "trillian"
	envMysqlUser     = "MYSQL_USER"
	envMysqlPassword = "MYSQL_PASSWORD"
	envMysqlHost     = "MYSQL_HOSTNAME"
	envMysqlPort     = "MYSQL_PORT"
	envMysqlDatabase = "MYSQL_DATABASE"
)

func EnsureDB(instance *rhtasv1alpha1.Trillian, mysqlOpts func(instance *rhtasv1alpha1.Trillian, options *MySQLOptions, container *v1.Container) error) func(*v1.Container) error {
	return func(container *v1.Container) error {
		var (
			options *MySQLOptions
			err     error
		)

		if utils.OptionalBool(instance.Spec.Db.Create) {
			// go with default mysql
			options = defaultMysqlDB(instance, container)
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

func defaultMysqlDB(instance *rhtasv1alpha1.Trillian, container *v1.Container) *MySQLOptions {
	options := &MySQLOptions{
		Host:       Host,
		Port:       strconv.Itoa(Port),
		Database:   databaseName,
		User:       User,
		TlsEnabled: instance.Status.Db.TLS.CertRef != nil,
	}
	if instance.Status.Db.DatabasePasswordSecretRef != nil {
		// use env alias to avoid plain-text printing
		options.Password = fmt.Sprintf("$(%s)", envMysqlPassword)

		hostEnv := kubernetes.FindEnvByNameOrCreate(container, "MYSQL_HOSTNAME")
		hostEnv.Value = options.Host

		userEnv := kubernetes.FindEnvByNameOrCreate(container, "MYSQL_USER")
		userEnv.Value = options.User

		portEnv := kubernetes.FindEnvByNameOrCreate(container, "MYSQL_PORT")
		portEnv.Value = options.Port

		dbEnv := kubernetes.FindEnvByNameOrCreate(container, "MYSQL_DATABASE")
		dbEnv.Value = options.Database
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

func EnsureEnvVar(container *v1.Container, targetEnv string, value string) (string, error) {

	// If value already references an env var, just ensure it exists
	if env, ok := ExtractEnvVarName(value); ok {
		_ = kubernetes.FindEnvByNameOrCreate(container, env)
		return fmt.Sprintf("$(%s)", env), nil
	}

	// if plain-text value, export it
	env := kubernetes.FindEnvByNameOrCreate(container, targetEnv)
	// if value contains ENV variables
	env.Value = envAsShellParams(value)

	return fmt.Sprintf("$(%s)", targetEnv), nil
}

func envAsShellParams(option string) string {
	// we must transfer ENV patterns from $(ENV) to $ENV to be correctly interpreted by shell
	re := regexp.MustCompile(`\$\((.*?)\)`)
	replacement := `$$$1` //$ + first matching group

	return re.ReplaceAllString(option, replacement)
}
