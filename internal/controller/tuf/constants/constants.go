package constants

const (
	ComponentName       = "tuf"
	DeploymentName      = "tuf"
	RBACName            = "tuf"
	PortName            = "http"
	Port                = 8080
	InitJobName         = "tuf-repository-init"
	MigrationJobName    = "tuf-repository-migration"
	RBACInitJobName     = "tuf-repository-init"
	ContainerName       = "tuf-server"
	VolumeName          = "repository"
	RepositoryCondition = "repository"

	RepositoryVersionAnnotation = "rhtas.redhat.com/tuf-version"
	TufVersionV1                = "v1"
)
