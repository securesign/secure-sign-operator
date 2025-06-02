//go:build integration

package byo

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/test/e2e/support"
	testSupportKubernetes "github.com/securesign/operator/test/e2e/support/kubernetes"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
)

const dbAuth = "db-auth"

var _ = Describe("Securesign install with byodb", Ordered, func() {
	cli, _ := support.CreateClient()

	var targetImageName string
	var namespace *v1.Namespace
	var s *v1alpha1.Securesign

	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))

	BeforeAll(func(ctx SpecContext) {
		dsn := "$(MYSQL_USER):$(MYSQL_PASSWORD)@tcp(my-mysql.$(NAMESPACE).svc:3300)/$(MYSQL_DB)"
		if testSupportKubernetes.IsRemoteClusterOpenshift() {
			dsn += "?tls=true"
		}

		s = securesign.Create(namespace.Name, "test",
			securesign.WithDefaults(),
			securesign.WithExternalDatabase(dbAuth),
			func(v *v1alpha1.Securesign) {
				v.Spec.Rekor.Auth = &v1alpha1.Auth{
					Env: []v1.EnvVar{
						{
							Name: "MYSQL_USER",
							ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{
									Name: dbAuth,
								},
								Key: "mysql-user",
							}},
						},
						{
							Name: "MYSQL_PASSWORD",
							ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{
									Name: dbAuth,
								},
								Key: "mysql-password",
							}},
						},
						{
							Name: "MYSQL_DB",
							ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{
									Name: dbAuth,
								},
								Key: "mysql-database",
							}},
						},
						{
							Name: "NAMESPACE",
							ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{
								APIVersion: "v1",
								FieldPath:  "metadata.namespace",
							}},
						},
					},
				}
				v.Spec.Rekor.BackFillRedis = v1alpha1.BackFillRedis{
					Enabled:  ptr.To(true),
					Schedule: "* * * * *",
				}

				v.Spec.Rekor.SearchIndex = v1alpha1.SearchIndex{
					Create:   ptr.To(false),
					Provider: "mysql",
					Url:      dsn,
				}
			},
		)
	})

	BeforeAll(func(ctx SpecContext) {
		targetImageName = support.PrepareImage(ctx)
	})

	Describe("Install with byodb", func() {
		BeforeAll(func(ctx SpecContext) {
			// create single mysql db for both (trillian & rekor search) to save CI resources
			Expect(createDB(ctx, cli, namespace.Name, dbAuth)).To(Succeed())
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("All components are running", func(ctx SpecContext) {
			tas.VerifyAllComponents(ctx, cli, s, false)
		})

		It("No other DB is created", func(ctx SpecContext) {
			list := &v1.PodList{}
			Expect(cli.List(ctx, list, runtimeCli.InNamespace(namespace.Name), runtimeCli.MatchingLabels{labels.LabelAppName: "trillian-db"})).To(Succeed())
			Expect(list.Items).To(BeEmpty(), "Trillian DB is not created")

			Expect(cli.List(ctx, list, runtimeCli.InNamespace(namespace.Name), runtimeCli.MatchingLabels{labels.LabelAppName: actions.RedisDeploymentName})).To(Succeed())
			Expect(list.Items).To(BeEmpty(), "Redis DB is not created")
		})

		It("Use cosign cli", func(ctx SpecContext) {
			tas.VerifyByCosign(ctx, cli, s, targetImageName)
		})

		It("Verify backfill cron job", func(ctx SpecContext) {
			Eventually(func(g Gomega) []string {
				logs := make([]string, 0)
				jobPods := &v1.PodList{}
				g.Expect(cli.List(ctx, jobPods, runtimeCli.InNamespace(namespace.Name), runtimeCli.HasLabels{"job-name"})).To(Succeed())
				for _, pod := range jobPods.Items {
					if pod.Status.Phase != v1.PodSucceeded {
						continue
					}
					if strings.Contains(pod.Labels["job-name"], actions.BackfillRedisCronJobName) {
						l, e := testSupportKubernetes.GetPodLogs(ctx, pod.Name, actions.BackfillRedisCronJobName, namespace.Name)
						Expect(e).NotTo(HaveOccurred())

						logs = append(logs, l)
					}
				}
				return logs
			}).WithTimeout(2*time.Minute + 10*time.Second).WithPolling(1 * time.Minute).Should(ContainElement(ContainSubstring("Completed log index")))
		})
	})
})

func createDB(ctx context.Context, cli runtimeCli.Client, ns string, secretRef string) error {

	mysql := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      "my-mysql",
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       "3306-tcp",
					Port:       3300,
					TargetPort: intstr.IntOrString{IntVal: 3306},
					Protocol:   "TCP",
				},
			},
			Selector: map[string]string{
				labels.LabelAppName: "my-db",
			},
		},
	}

	if testSupportKubernetes.IsRemoteClusterOpenshift() {
		if mysql.Annotations == nil {
			mysql.Annotations = make(map[string]string)
		}
		mysql.Annotations[annotations.TLS] = "my-db-tls-secret"
	}
	err := cli.Create(ctx, mysql)
	if err != nil {
		return err
	}

	err = cli.Create(ctx, &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: secretRef},
		Data: map[string][]byte{
			"mysql-database":      []byte("my_db"),
			"mysql-host":          []byte("my-mysql"),
			"mysql-password":      []byte("password"),
			"mysql-port":          []byte("3300"),
			"mysql-root-password": []byte("password"),
			"mysql-user":          []byte("mysql"),
		},
	})
	if err != nil {
		return err
	}
	volumesMounts := []v1.VolumeMount{
		{
			Name:      "storage",
			MountPath: "/var/lib/mysql",
		},
	}
	volumes := []v1.Volume{
		{
			Name: "storage",
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{},
			},
		},
	}
	args := []string{}

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

		args = append(args, "--ssl-cert", "/etc/ssl/certs/tls.crt")
		args = append(args, "--ssl-key", "/etc/ssl/certs/tls.key")
	}

	err = cli.Create(ctx, &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      "my-db",
			Labels:    map[string]string{labels.LabelAppName: "my-db"},
		},
		Spec: v1.PodSpec{
			Volumes: volumes,
			Containers: []v1.Container{
				{
					Name:  "mysql",
					Image: "registry.redhat.io/rhtas-tech-preview/trillian-database-rhel9@sha256:fe4758ff57a9a6943a4655b21af63fb579384dc51838af85d0089c04290b4957",
					Command: []string{
						"run-mysqld",
					},
					Args: args,
					Env: []v1.EnvVar{
						{
							Name: "MYSQL_ROOT_PASSWORD",
							ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{
									Name: secretRef,
								},
								Key: "mysql-root-password",
							}},
						},
						{
							Name: "MYSQL_USER",
							ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{
									Name: secretRef,
								},
								Key: "mysql-user",
							}},
						},
						{
							Name: "MYSQL_PASSWORD",
							ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{
									Name: secretRef,
								},
								Key: "mysql-password",
							}},
						},
						{
							Name: "MYSQL_DATABASE",
							ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{
									Name: secretRef,
								},
								Key: "mysql-database",
							}},
						},
					},
					Ports: []v1.ContainerPort{
						{
							ContainerPort: 3306,
							Protocol:      "TCP",
						},
					},
					ReadinessProbe: &v1.Probe{
						ProbeHandler: v1.ProbeHandler{
							Exec: &v1.ExecAction{
								Command: []string{"bash", "-c", "mysqladmin ping -h localhost -u $MYSQL_USER -p$MYSQL_PASSWORD"},
							},
						},
						InitialDelaySeconds: 3,
						TimeoutSeconds:      1,
						PeriodSeconds:       10,
						SuccessThreshold:    1,
						FailureThreshold:    3,
					},
					VolumeMounts: volumesMounts,
					SecurityContext: &v1.SecurityContext{
						AllowPrivilegeEscalation: ptr.To(false),
						Capabilities: &v1.Capabilities{
							Drop: []v1.Capability{"ALL"},
						},
						RunAsNonRoot: ptr.To(true),
						SeccompProfile: &v1.SeccompProfile{
							Type: v1.SeccompProfileTypeRuntimeDefault,
						},
					},
				},
			},
		},
	})
	if err != nil {
		return err
	}
	return err
}
