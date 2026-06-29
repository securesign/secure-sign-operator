package postgresql

import (
	"context"
	_ "embed"
	"fmt"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/utils/kubernetes"
	k8ssupport "github.com/securesign/operator/test/e2e/support/kubernetes"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	PodName           = "my-postgres"
	AppLabel          = "my-pg-db"
	DefaultSecretName = "postgresql-connection"
	containerName     = "postgresql"
	image             = "registry.redhat.io/rhel9/postgresql-15:latest"

	ConnectionURI = "postgresql:///$(POSTGRESQL_DATABASE)?host=$(POSTGRESQL_HOST)&user=$(POSTGRESQL_USER)&password=$(POSTGRESQL_PASSWORD)"
	Provider      = "postgresql"
)

//go:embed trillian_postgresql.sql
var schema string

func AuthEnvVars(namespace, secretName string) []v1.EnvVar {
	return []v1.EnvVar{
		{
			Name:  "POSTGRESQL_HOST",
			Value: fmt.Sprintf("my-postgres.%s.svc", namespace),
		},
		{
			Name: "POSTGRESQL_USER",
			ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{
				LocalObjectReference: v1.LocalObjectReference{Name: secretName},
				Key:                  "postgres-user",
			}},
		},
		{
			Name: "POSTGRESQL_PASSWORD",
			ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{
				LocalObjectReference: v1.LocalObjectReference{Name: secretName},
				Key:                  "postgres-password",
			}},
		},
		{
			Name: "POSTGRESQL_DATABASE",
			ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{
				LocalObjectReference: v1.LocalObjectReference{Name: secretName},
				Key:                  "postgres-database",
			}},
		},
	}
}

func CreateDB(ctx context.Context, cli ctrlclient.Client, ns, secretName, password string) error {
	pgService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      PodName,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       "5432-tcp",
					Port:       5432,
					TargetPort: intstr.FromInt32(5432),
					Protocol:   "TCP",
				},
			},
			Selector: map[string]string{
				"app": AppLabel,
			},
		},
	}
	if k8ssupport.IsRemoteClusterOpenshift() {
		if pgService.Annotations == nil {
			pgService.Annotations = make(map[string]string)
		}
		pgService.Annotations[annotations.TLS] = "my-db-tls-secret"
	}

	if err := cli.Create(ctx, pgService); err != nil {
		return err
	}

	if err := cli.Create(ctx, &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: secretName},
		Data: map[string][]byte{
			"postgres-database": []byte("trillian"),
			"postgres-user":     []byte("testUser"),
			"postgres-password": []byte(password),
			"postgres-host":     []byte(PodName),
			"postgres-port":     []byte("5432"),
		},
	}); err != nil {
		return err
	}

	var volumeMounts []v1.VolumeMount
	var volumes []v1.Volume
	args := []string{"run-postgresql"}
	if kubernetes.IsOpenShift() {
		volumeMounts = append(volumeMounts, v1.VolumeMount{
			Name:      "tls-cert",
			MountPath: "/etc/ssl/certs",
			ReadOnly:  true,
		})

		volumes = append(volumes, v1.Volume{
			Name: "tls-cert",
			VolumeSource: v1.VolumeSource{
				Projected: &v1.ProjectedVolumeSource{
					DefaultMode: ptr.To(int32(0600)),
					Sources: []v1.VolumeProjection{
						{
							Secret: &v1.SecretProjection{
								LocalObjectReference: v1.LocalObjectReference{
									Name: "my-db-tls-secret",
								},
							},
						},
					},
				},
			},
		})

		args = append(args, "-c", "ssl=on", "-c", "ssl_cert_file=/etc/ssl/certs/tls.crt", "-c", "ssl_key_file=/etc/ssl/certs/tls.key")
	}

	return cli.Create(ctx, &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      PodName,
			Labels:    map[string]string{"app": AppLabel},
		},
		Spec: v1.PodSpec{
			Volumes: volumes,
			Containers: []v1.Container{
				{
					Name:  containerName,
					Image: image,
					Env: []v1.EnvVar{
						{
							Name: "POSTGRESQL_USER",
							ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{Name: secretName},
								Key:                  "postgres-user",
							}},
						},
						{
							Name: "POSTGRESQL_PASSWORD",
							ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{Name: secretName},
								Key:                  "postgres-password",
							}},
						},
						{
							Name: "POSTGRESQL_DATABASE",
							ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{Name: secretName},
								Key:                  "postgres-database",
							}},
						},
					},
					Args:  args,
					Ports: []v1.ContainerPort{{ContainerPort: 5432}},
					ReadinessProbe: &v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							Exec: &v1.ExecAction{
								Command: []string{"pg_isready", "-h", "localhost", "-U", "$(POSTGRESQL_USER)"},
							},
						},
						InitialDelaySeconds: 5,
						PeriodSeconds:       5,
					},
					VolumeMounts: volumeMounts,
					SecurityContext: &v1.SecurityContext{
						AllowPrivilegeEscalation: ptr.To(false),
						Capabilities:             &v1.Capabilities{Drop: []v1.Capability{"ALL"}},
						RunAsNonRoot:             ptr.To(true),
						SeccompProfile:           &v1.SeccompProfile{Type: v1.SeccompProfileTypeRuntimeDefault},
					},
				},
			},
		},
	})
}

func WaitAndLoadSchema(ctx context.Context, cli ctrlclient.Client, ns string) {
	Eventually(func(g Gomega, ctx context.Context) bool {
		p := &v1.Pod{}
		err := cli.Get(ctx, ctrlclient.ObjectKey{
			Name:      PodName,
			Namespace: ns,
		}, p)
		if errors.IsNotFound(err) {
			return false
		}
		g.Expect(err).NotTo(HaveOccurred())
		for _, cond := range p.Status.Conditions {
			if cond.Type == v1.PodReady {
				return cond.Status == v1.ConditionTrue
			}
		}
		return false
	}).WithContext(ctx).Should(BeTrue())

	Expect(k8ssupport.ExecInPod(ctx, PodName, containerName, ns, "/bin/bash", "-c",
		fmt.Sprintf(`
psql -U ${POSTGRESQL_USER} -d trillian <<'EOF'
%s
EOF
`, schema))).To(Succeed())
}
