//go:build integration

package e2e

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/test/e2e/support"
	testSupportKubernetes "github.com/securesign/operator/test/e2e/support/kubernetes"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas/trillian"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	dbConnection = "postgresql-connection"
	dbName       = "my-postgres"
	appLabel     = "my-pg-db"
)

//go:embed resources/trillian_postgresql.sql
var schema string

var _ = Describe("Trillian install with byodb", Ordered, func() {
	cli, _ := support.CreateClient()

	var namespace *v1.Namespace
	var t *v1alpha1.Trillian

	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))

	BeforeAll(func(ctx SpecContext) {

		t = &v1alpha1.Trillian{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.Name,
				Name:      "postgresql-test",
			},
			Spec: v1alpha1.TrillianSpec{
				Auth: &v1alpha1.Auth{
					Env: []v1.EnvVar{
						{
							Name:  "POSTGRESQL_HOST",
							Value: fmt.Sprintf("my-postgres.%s.svc", namespace.Name),
						},
						{
							Name: "POSTGRESQL_USER",
							ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{Name: dbConnection},
								Key:                  "postgres-user",
							}},
						},
						{
							Name: "POSTGRESQL_PASSWORD",
							ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{Name: dbConnection},
								Key:                  "postgres-password",
							}},
						},
						{
							Name: "POSTGRESQL_DATABASE",
							ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{Name: dbConnection},
								Key:                  "postgres-database",
							}},
						},
					},
				},
				Db: v1alpha1.TrillianDB{
					Create:   ptr.To(false),
					Provider: "postgresql",
					Uri:      "postgresql:///$(POSTGRESQL_DATABASE)?host=$(POSTGRESQL_HOST)&user=$(POSTGRESQL_USER)&password=$(POSTGRESQL_PASSWORD)",
				},
			},
		}
	})

	Describe("Install with byodb", func() {
		BeforeAll(func(ctx SpecContext) {
			// create single mysql db for both (t & rekor search) to save CI resources
			Expect(createPostgresDB(ctx, cli, namespace.Name, dbConnection)).To(Succeed())

			Eventually(func(g Gomega) bool {
				p := &v1.Pod{}
				err := cli.Get(ctx, runtimeCli.ObjectKey{
					Name:      dbName,
					Namespace: namespace.Name,
				}, p)
				if errors.IsNotFound(err) {
					return false
				}

				for _, cond := range p.Status.Conditions {
					if cond.Type == v1.PodReady {
						return cond.Status == v1.ConditionTrue
					}
				}
				return false
			}).Should(BeTrue())

			// initialize trillian scheme
			Expect(testSupportKubernetes.ExecInPod(ctx, dbName, "postgresql", namespace.Name, "/bin/bash", "-c",
				fmt.Sprintf(
					`
psql -U ${POSTGRESQL_USER} -d trillian <<'EOF'
%s
EOF
`, schema))).To(Succeed())
			time.Sleep(time.Second * 5)

			Expect(cli.Create(ctx, t)).To(Succeed())
		})

		It("Trillian is running", func(ctx SpecContext) {
			trillian.Verify(ctx, cli, t.Namespace, t.Name, false)

			podList := &v1.PodList{}
			Expect(cli.List(ctx, podList, runtimeCli.InNamespace(namespace.Name), runtimeCli.MatchingLabels{
				"app.kubernetes.io/part-of": "trusted-artifact-signer",
			})).To(Succeed())
			Expect(podList.Items).To(HaveLen(2))

			for _, pod := range podList.Items {
				log, err := testSupportKubernetes.GetPodLogs(ctx, pod.Name, pod.Labels["app.kubernetes.io/component"], pod.Namespace)
				Expect(err).ToNot(HaveOccurred())
				Expect(strings.ToLower(log)).ToNot(ContainSubstring("error"))
				for _, c := range pod.Status.ContainerStatuses {
					Expect(c.RestartCount).To(BeNumerically("==", 0))
				}
			}
		})
	})
})

func createPostgresDB(ctx context.Context, cli runtimeCli.Client, ns string, secretRef string) error {

	pgService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      dbName,
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
				"app": appLabel,
			},
		},
	}
	if testSupportKubernetes.IsRemoteClusterOpenshift() {
		if pgService.Annotations == nil {
			pgService.Annotations = make(map[string]string)
		}
		pgService.Annotations[annotations.TLS] = "my-db-tls-secret"
	}

	if err := cli.Create(ctx, pgService); err != nil {
		return err
	}

	err := cli.Create(ctx, &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: secretRef},
		Data: map[string][]byte{
			"postgres-database": []byte("trillian"),
			"postgres-user":     []byte("testUser"),
			"postgres-password": []byte("password"),
			"postgres-host":     []byte(dbName),
			"postgres-port":     []byte("5432"),
		},
	})
	if err != nil {
		return err
	}

	var volumesMounts []v1.VolumeMount
	var volumes []v1.Volume
	args := []string{"run-postgresql"}
	if kubernetes.IsOpenShift() {
		volumesMounts = append(volumesMounts, v1.VolumeMount{
			Name:      "tls-cert",
			MountPath: "/etc/ssl/certs",
			ReadOnly:  true,
		})

		volumes = append(volumes,
			v1.Volume{
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
			Name:      dbName,
			Labels:    map[string]string{"app": appLabel},
		},
		Spec: v1.PodSpec{
			Volumes: volumes,
			Containers: []v1.Container{
				{
					Name:  "postgresql",
					Image: "registry.redhat.io/rhel9/postgresql-15:latest",
					Env: []v1.EnvVar{
						{
							Name: "POSTGRESQL_USER",
							ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{Name: secretRef},
								Key:                  "postgres-user",
							}},
						},
						{
							Name: "POSTGRESQL_PASSWORD",
							ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{Name: secretRef},
								Key:                  "postgres-password",
							}},
						},
						{
							Name: "POSTGRESQL_DATABASE",
							ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{Name: secretRef},
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
					VolumeMounts: volumesMounts,
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
