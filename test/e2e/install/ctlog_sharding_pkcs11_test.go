//go:build integration

package install

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/certificate-transparency-go/trillian/ctfe/configpb"
	"github.com/google/trillian/crypto/keyspb"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	ctlogAction "github.com/securesign/operator/internal/controller/ctlog/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	testKubernetes "github.com/securesign/operator/test/e2e/support/kubernetes"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/postgresql"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/ctlog"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CTlog sharding with PKCS#11", Ordered, func() {
	SetDefaultEventuallyTimeout(time.Duration(10) * time.Minute)
	cli, _ := support.CreateClient()

	var namespace *v1.Namespace
	var s *rhtasv1.Securesign
	var fipsEnabled bool
	var runningTimestamp time.Time

	BeforeAll(steps.DetectAndConfigureFIPS(cli, func(enabled bool) {
		fipsEnabled = enabled
	}))

	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))

	BeforeAll(func(ctx SpecContext) {
		if fipsEnabled {
			Expect(postgresql.CreateDB(ctx, cli, namespace.Name, postgresql.DefaultSecretName, "fips-password")).To(Succeed())
			postgresql.WaitAndLoadSchema(ctx, cli, namespace.Name)
		}
	})

	BeforeAll(func(ctx SpecContext) {
		s = securesign.Create(namespace.Name, "test",
			securesign.ChooseDefaults(fipsEnabled, namespace.Name),
		)
		// Configure PKCS#11 signer
		s.Spec.Ctlog.Signer.Type = "pkcs11"
		s.Spec.Ctlog.Signer.PKCS11 = &rhtasv1.CTlogPKCS11Config{
			PKCS11Config: rhtasv1.PKCS11Config{
				ModulePath:  "/usr/lib64/pkcs11/libsofthsm2.so",
				TokenLabel:  "PKCS11CA",
				PinSecretRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{
						Name: "hsm-credentials",
					},
					Key: "pin",
				},
			},
			PublicKeyRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{
					Name: "ctlog-hsm-public-key",
				},
				Key: "public.pem",
			},
		}
		s.Spec.Ctlog.Signer.File = nil
	})

	Describe("Install with PKCS#11 signer", func() {
		BeforeAll(func(ctx SpecContext) {
			// Create HSM credentials secret
			hsmSecret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hsm-credentials",
					Namespace: namespace.Name,
				},
				Data: map[string][]byte{
					"pin": []byte("1234"),
				},
			}
			Expect(cli.Create(ctx, hsmSecret)).To(Succeed())

			// Create public key secret for PKCS#11
			pubKeySecret := ctlog.CreateSecret(namespace.Name, "ctlog-hsm-public-key", !fipsEnabled)
			Expect(cli.Create(ctx, pubKeySecret)).To(Succeed())

			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("All other components are running", func(ctx SpecContext) {
			tas.VerifyAllComponents(ctx, cli, s, !fipsEnabled, true)
			runningTimestamp = time.Now()
		})
	})

	Describe("Configure CTlog sharding with PKCS#11", func() {
		var oldTreeId *int64
		var oldConfig *v1.Secret
		var oldPublicKey []byte

		It("Get current ctlog tree information", func(ctx SpecContext) {
			c := ctlog.Get(ctx, cli, namespace.Name, s.Name)
			Expect(c).ToNot(BeNil())
			oldTreeId = c.Status.TreeID
			Expect(oldTreeId).ToNot(BeNil())

			var err error
			oldConfig, err = kubernetes.GetSecret(ctx, cli, namespace.Name, c.Status.ServerConfigRef.Name)
			Expect(err).ToNot(HaveOccurred())
			Expect(oldConfig).ToNot(BeNil())

			oldPublicKey = oldConfig.Data["public"]
			Expect(oldPublicKey).ToNot(BeEmpty())
		})

		It("Freeze the current tree", func(ctx SpecContext) {
			drainingPod := updateTree(namespace.Name, oldTreeId, "DRAINING")
			Expect(cli.Create(ctx, drainingPod)).To(Succeed())
			Eventually(func(gomega Gomega) bool {
				gomega.Expect(cli.Get(ctx, runtimeCli.ObjectKeyFromObject(drainingPod), drainingPod)).To(Succeed())
				return drainingPod.Status.Phase == v1.PodSucceeded
			}).Should(BeTrue())

			freezePod := updateTree(namespace.Name, oldTreeId, "FROZEN")
			Expect(cli.Create(ctx, freezePod)).To(Succeed())
			Eventually(func(gomega Gomega) bool {
				gomega.Expect(cli.Get(ctx, runtimeCli.ObjectKeyFromObject(freezePod), freezePod)).To(Succeed())
				return freezePod.Status.Phase == v1.PodSucceeded
			}).Should(BeTrue())
		})

		It("Create new tree for active log", func(ctx SpecContext) {
			newCreatePod := createTree(namespace.Name, "ctlog-pkcs11-sharding-tree")
			Expect(cli.Create(ctx, newCreatePod)).To(Succeed())
			Eventually(func(gomega Gomega) bool {
				gomega.Expect(cli.Get(ctx, runtimeCli.ObjectKeyFromObject(newCreatePod), newCreatePod)).To(Succeed())
				return newCreatePod.Status.Phase == v1.PodSucceeded
			}).Should(BeTrue())
		})

		It("Configure sharding with new tree and PKCS#11", func(ctx SpecContext) {
			Eventually(func() error {
				s := securesign.Get(ctx, cli, namespace.Name, "test")

				// Get new tree ID from the latest created pod logs
				podList := &v1.PodList{}
				Expect(cli.List(ctx, podList, runtimeCli.InNamespace(namespace.Name))).To(Succeed())

				var newTreeId int64
				for _, pod := range podList.Items {
					if strings.HasPrefix(pod.Name, "create-tree-") && pod.Status.Phase == v1.PodSucceeded {
						createTreeLog, err := testKubernetes.GetPodLogs(ctx, pod.Name, "createtree", namespace.Name)
						if err == nil {
							lines := strings.Split(strings.TrimSpace(createTreeLog), "\n")
							if len(lines) > 0 {
								newTreeId, _ = strconv.ParseInt(lines[len(lines)-1], 10, 64)
								break
							}
						}
					}
				}

				if newTreeId == 0 {
					return fmt.Errorf("failed to get new tree ID")
				}

				secretName := "ctlog-pkcs11-sharding-config"
				newCtlogSecret := ctlog.CreateSecret(namespace.Name, secretName, !fipsEnabled)

				// Prepare config with sharding
				cfg := &configpb.LogMultiConfig{}
				now := time.Now()
				timestamp := &timestamppb.Timestamp{
					Seconds: now.Unix(), Nanos: int32(now.Nanosecond()),
				}
				Expect(prototext.Unmarshal(oldConfig.Data["config"], cfg)).To(Succeed())

				// First config entry is the frozen shard
				frozen := cfg.LogConfigs.Config[0]
				Expect(frozen).ToNot(BeNil())

				// Create a copy for the active shard
				cfg.LogConfigs.Config = append(cfg.LogConfigs.Config, proto.Clone(frozen).(*configpb.LogConfig))
				active := cfg.LogConfigs.Config[1]

				// Configure the frozen shard
				frozen.NotAfterLimit = timestamp
				frozen.Prefix = "trusted-artifact-signer-shard-0"
				frozenKey := &keyspb.PEMKeyFile{}
				Expect(anypb.UnmarshalTo(frozen.PrivateKey, frozenKey, proto.UnmarshalOptions{})).To(Succeed())
				frozenKey.Path = "/ctfe-keys/private-0"
				frozenAnyKey, err := anypb.New(frozenKey)
				Expect(err).ToNot(HaveOccurred())
				frozen.PrivateKey = frozenAnyKey

				// Configure the active shard with PKCS#11
				active.LogId = newTreeId
				active.Prefix = "trusted-artifact-signer"
				pkcs11Key := &keyspb.PKCS11Config{
					TokenLabel: "PKCS11CA",
					Pin:        string(newCtlogSecret.Data["pin"]),
					PublicKey:  string(oldPublicKey), // This will be replaced with new key in real HSM scenario
				}
				activeAnyKey, err := anypb.New(pkcs11Key)
				Expect(err).ToNot(HaveOccurred())
				active.PrivateKey = activeAnyKey
				active.PublicKey = nil
				active.NotAfterStart = timestamp

				configdata, err := prototext.Marshal(cfg)
				Expect(err).ToNot(HaveOccurred())
				newCtlogSecret.Data["config"] = configdata
				newCtlogSecret.Data["fulcio"] = oldConfig.Data["fulcio"]
				newCtlogSecret.Data["private-0"] = oldConfig.Data["private"]
				newCtlogSecret.Data["public-0"] = oldConfig.Data["public"]

				Expect(cli.Create(ctx, newCtlogSecret)).To(Succeed())

				// Update securesign resource with new tree and sharding config (PKCS#11 only)
				Expect(cli.Get(ctx, runtimeCli.ObjectKeyFromObject(s), s)).To(Succeed())
				s.Spec.Ctlog.ServerConfigRef = &rhtasv1.LocalObjectReference{
					Name: secretName,
				}

				s.Spec.Ctlog.TreeID = ptr.To(newTreeId)

				// Add sharding configuration with PKCS#11
				// Note: PKCS#11 shards only need PublicKeyRef, no PrivateKeyRef
				s.Spec.Ctlog.Sharding = []rhtasv1.CTlogLogRange{
					{
						TreeID:     *oldTreeId,
						TreeLength: 0,
						Type:       "file", // Old shard was file-based
						PublicKeyRef: rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{
								Name: secretName,
							},
							Key: "public-0",
						},
						PrivateKeyRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{
								Name: secretName,
							},
							Key: "private-0",
						},
					},
					{
						TreeID:     newTreeId,
						TreeLength: 0,
						Type:       "pkcs11", // New shard is PKCS#11
						PublicKeyRef: rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{
								Name: "ctlog-hsm-public-key",
							},
							Key: "public.pem",
						},
						// Note: No PrivateKeyRef for PKCS#11, key stays in HSM
					},
				}

				return cli.Update(ctx, s)
			}).Should(Succeed())
		})

		It("Verify CTlog deployment is updated with PKCS#11 sharding config", func(ctx SpecContext) {
			Eventually(func(g Gomega) bool {
				list := &v1.PodList{}
				g.Expect(cli.List(ctx, list, runtimeCli.InNamespace(s.Namespace), runtimeCli.MatchingLabels{labels.LabelAppComponent: ctlogAction.ComponentName})).To(Succeed())
				for _, p := range list.Items {
					if p.CreationTimestamp.After(runningTimestamp) {
						return true
					}
				}
				return false
			}).Should(BeTrue())
		})

		It("Verify ctlog is in Ready state", func(ctx SpecContext) {
			Eventually(func(g Gomega) bool {
				c := ctlog.Get(ctx, cli, namespace.Name, s.Name)
				if c == nil {
					return false
				}
				return c.Status.TreeID != nil && *c.Status.TreeID != *oldTreeId
			}, time.Duration(5)*time.Minute).Should(BeTrue())
		})

		It("Verify PKCS#11 sharding config is applied", func(ctx SpecContext) {
			c := ctlog.Get(ctx, cli, namespace.Name, s.Name)
			Expect(c).ToNot(BeNil())

			configSecret, err := kubernetes.GetSecret(ctx, cli, namespace.Name, c.Status.ServerConfigRef.Name)
			Expect(err).ToNot(HaveOccurred())

			cfg := &configpb.LogMultiConfig{}
			Expect(prototext.Unmarshal(configSecret.Data["config"], cfg)).To(Succeed())

			// Verify we have two log configs (frozen file-based + active PKCS#11)
			Expect(cfg.LogConfigs.Config).To(HaveLen(2))

			// Verify frozen shard has the old tree ID (file-based)
			frozenCfg := cfg.LogConfigs.Config[0]
			Expect(frozenCfg.LogId).To(Equal(*oldTreeId))
			Expect(frozenCfg.NotAfterLimit).ToNot(BeNil())

			// Verify active shard has the new tree ID (PKCS#11-based)
			activeCfg := cfg.LogConfigs.Config[1]
			Expect(activeCfg.LogId).To(Equal(*c.Status.TreeID))
			Expect(activeCfg.NotAfterStart).ToNot(BeNil())

			// Verify active shard is configured with PKCS#11
			pkcs11Cfg := &keyspb.PKCS11Config{}
			Expect(anypb.UnmarshalTo(activeCfg.PrivateKey, pkcs11Cfg, proto.UnmarshalOptions{})).To(Succeed())
			Expect(pkcs11Cfg.TokenLabel).To(Equal("PKCS11CA"))
		})
	})
})
