//go:build integration

package e2e

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
	"github.com/securesign/operator/test/e2e/support/tas"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
)

const dbAuth = "db-auth"

var _ = Describe("Securesign install with byodb", Ordered, func() {
	cli, _ := support.CreateClient()
	ctx := context.TODO()

	var targetImageName string
	var namespace *v1.Namespace
	var s *v1alpha1.Securesign

	AfterEach(func() {
		if CurrentSpecReport().Failed() && support.IsCIEnvironment() {
			support.DumpNamespace(ctx, cli, namespace.Name)
		}
	})

	BeforeAll(func() {
		namespace = support.CreateTestNamespace(ctx, cli)
		DeferCleanup(func() {
			_ = cli.Delete(ctx, namespace)
		})

		dsn := "$(MYSQL_USER):$(MYSQL_PASSWORD)@tcp(my-mysql.$(NAMESPACE).svc:3300)/$(MYSQL_DB)"
		if testSupportKubernetes.IsRemoteClusterOpenshift() {
			dsn += "?tls=true"
		}

		s = &v1alpha1.Securesign{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.Name,
				Name:      "test",
				Annotations: map[string]string{
					"rhtas.redhat.com/metrics": "false",
				},
			},
			Spec: v1alpha1.SecuresignSpec{
				Rekor: v1alpha1.RekorSpec{
					Auth: &v1alpha1.Auth{
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
					},
					ExternalAccess: v1alpha1.ExternalAccess{
						Enabled: true,
					},
					BackFillRedis: v1alpha1.BackFillRedis{
						Enabled:  ptr.To(true),
						Schedule: "* * * * *",
					},
					SearchIndex: v1alpha1.SearchIndex{
						Create:   ptr.To(false),
						Provider: "mysql",
						Url:      dsn,
					},
				},
				Fulcio: v1alpha1.FulcioSpec{
					ExternalAccess: v1alpha1.ExternalAccess{
						Enabled: true,
					},
					Config: v1alpha1.FulcioConfig{
						OIDCIssuers: []v1alpha1.OIDCIssuer{
							{
								ClientID:  support.OidcClientID(),
								IssuerURL: support.OidcIssuerUrl(),
								Issuer:    support.OidcIssuerUrl(),
								Type:      "email",
							},
						}},
					Certificate: v1alpha1.FulcioCert{
						OrganizationName:  "MyOrg",
						OrganizationEmail: "my@email.org",
						CommonName:        "fulcio",
					},
				},
				Tuf: v1alpha1.TufSpec{
					ExternalAccess: v1alpha1.ExternalAccess{
						Enabled: true,
					},
				},
				Ctlog: v1alpha1.CTlogSpec{},
				Trillian: v1alpha1.TrillianSpec{Db: v1alpha1.TrillianDB{
					Create: new(bool),
					DatabaseSecretRef: &v1alpha1.LocalObjectReference{
						Name: dbAuth,
					},
				}},
				TimestampAuthority: &v1alpha1.TimestampAuthoritySpec{
					ExternalAccess: v1alpha1.ExternalAccess{
						Enabled: true,
					},
					Signer: v1alpha1.TimestampAuthoritySigner{
						CertificateChain: v1alpha1.CertificateChain{
							RootCA: &v1alpha1.TsaCertificateAuthority{
								OrganizationName:  "MyOrg",
								OrganizationEmail: "my@email.org",
								CommonName:        "tsa.hostname",
							},
							IntermediateCA: []*v1alpha1.TsaCertificateAuthority{
								{
									OrganizationName:  "MyOrg",
									OrganizationEmail: "my@email.org",
									CommonName:        "tsa.hostname",
								},
							},
							LeafCA: &v1alpha1.TsaCertificateAuthority{
								OrganizationName:  "MyOrg",
								OrganizationEmail: "my@email.org",
								CommonName:        "tsa.hostname",
							},
						},
					},
					NTPMonitoring: v1alpha1.NTPMonitoring{
						Enabled: true,
						Config: &v1alpha1.NtpMonitoringConfig{
							RequestAttempts: 3,
							RequestTimeout:  5,
							NumServers:      4,
							ServerThreshold: 3,
							MaxTimeDelta:    6,
							Period:          60,
							Servers:         []string{"time.apple.com", "time.google.com", "time-a-b.nist.gov", "time-b-b.nist.gov", "gbg1.ntp.se"},
						},
					},
				},
			},
		}
	})

	BeforeAll(func() {
		targetImageName = support.PrepareImage(ctx)
	})

	Describe("Install with byodb", func() {
		BeforeAll(func() {
			// create single mysql db for both (trillian & rekor search) to save CI resources
			Expect(createDB(ctx, cli, namespace.Name, dbAuth)).To(Succeed())
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("All components are running", func() {
			tas.VerifyAllComponents(ctx, cli, s, false)
		})

		It("No other DB is created", func() {
			list := &v1.PodList{}
			Expect(cli.List(ctx, list, runtimeCli.InNamespace(namespace.Name), runtimeCli.MatchingLabels{labels.LabelAppName: "trillian-db"})).To(Succeed())
			Expect(list.Items).To(BeEmpty(), "Trillian DB is not created")

			Expect(cli.List(ctx, list, runtimeCli.InNamespace(namespace.Name), runtimeCli.MatchingLabels{labels.LabelAppName: actions.RedisDeploymentName})).To(Succeed())
			Expect(list.Items).To(BeEmpty(), "Redis DB is not created")
		})

		It("Use cosign cli", func() {
			tas.VerifyByCosign(ctx, cli, s, targetImageName)
		})

		It("Verify backfill cron job", func() {
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
				},
			},
		},
	})
	if err != nil {
		return err
	}
	return err
}
