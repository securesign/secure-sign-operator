package openbao

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/securesign/operator/internal/utils/kubernetes"
	k8sSupport "github.com/securesign/operator/test/e2e/support/kubernetes"
)

const (
	// Image is the OpenBao dev-mode server image.
	Image = "quay.io/openbao/openbao:2.1.0"

	// PodName/ServiceName of the in-cluster OpenBao dev server.
	PodName     = "openbao"
	ServiceName = "openbao"
	appLabel    = "openbao"
	port        = 8200

	containerName = "openbao"

	// RootToken is the dev-mode root token (VAULT_TOKEN).
	RootToken = "root"

	// RekorKeyName is the transit key backing the Rekor KMS signer.
	RekorKeyName = "rekor-kms"
	// FulcioKeyName is the transit key backing the Fulcio KMS signer.
	FulcioKeyName = "fulcio-kms"
	// TsaKeyName is the transit key backing the TSA KMS signer.
	TsaKeyName = "tsa-kms"
)

// transitKeyNames lists every transit key CreatePrerequisites provisions.
var transitKeyNames = []string{RekorKeyName, FulcioKeyName, TsaKeyName}

// Addr returns the in-cluster VAULT_ADDR pointing at the OpenBao service.
func Addr(namespace string) string {
	return fmt.Sprintf("http://%s.%s.svc:%d", ServiceName, namespace, port)
}

// CreatePrerequisites deploys an OpenBao dev-mode server in the given namespace,
// waits for it to become ready, and enables the transit secrets engine with the
// rekor-kms, fulcio-kms and tsa-kms signing keys used by WithKMSOpenBaoSigner.
func CreatePrerequisites(ctx context.Context, cli client.Client, namespace string) error {
	resources := []client.Object{
		service(namespace),
		pod(namespace),
	}
	for _, r := range resources {
		if err := cli.Create(ctx, r); err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("creating %T %s: %w", r, r.GetName(), err)
			}
		}
	}

	if err := waitForPodReady(ctx, cli, namespace, PodName); err != nil {
		return fmt.Errorf("waiting for openbao pod: %w", err)
	}

	var keyWrites strings.Builder
	for _, keyName := range transitKeyNames {
		fmt.Fprintf(&keyWrites, "bao write transit/keys/%s type=ecdsa-p256\n", keyName)
	}

	script := fmt.Sprintf(`
export VAULT_ADDR=http://127.0.0.1:%d
export VAULT_TOKEN=%s
bao secrets enable transit
%s`, port, RootToken, keyWrites.String())

	if err := k8sSupport.ExecInPod(ctx, PodName, containerName, namespace, "/bin/sh", "-c", script); err != nil {
		return fmt.Errorf("configuring openbao transit engine: %w", err)
	}

	return nil
}

func service(namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceName,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": appLabel},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       port,
					TargetPort: intstr.FromInt32(port),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}

func podSecurityContext() *corev1.PodSecurityContext {
	sc := &corev1.PodSecurityContext{
		RunAsNonRoot: ptr.To(true),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
	if !kubernetes.IsOpenShift() {
		sc.RunAsUser = ptr.To(int64(100))
		sc.RunAsGroup = ptr.To(int64(1000))
		sc.FSGroup = ptr.To(int64(1000))
	}
	return sc
}

func pod(namespace string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PodName,
			Namespace: namespace,
			Labels:    map[string]string{"app": appLabel},
		},
		Spec: corev1.PodSpec{
			SecurityContext: podSecurityContext(),
			Containers: []corev1.Container{
				{
					Name:            containerName,
					Image:           Image,
					ImagePullPolicy: corev1.PullIfNotPresent,
					Command:         []string{"bao"},
					Args: []string{
						"server", "-dev",
						"-dev-root-token-id=" + RootToken,
						fmt.Sprintf("-dev-listen-address=0.0.0.0:%d", port),
					},
					Env: []corev1.EnvVar{
						// Dev mode writes a convenience CLI token file to $HOME/.vault-token.
						// Without HOME set, it defaults to "/", which the non-root user can't
						// write to; that failure makes OpenBao revoke the just-generated root
						// token as a safety measure, breaking VAULT_TOKEN=root auth.
						{Name: "HOME", Value: "/tmp"},
					},
					Ports: []corev1.ContainerPort{
						{ContainerPort: port},
					},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: ptr.To(false),
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
					},
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/v1/sys/health",
								Port: intstr.FromInt32(port),
							},
						},
						InitialDelaySeconds: 2,
						PeriodSeconds:       3,
					},
				},
			},
		},
	}
}

// transitKeyResponse models the subset of `bao read -format=json transit/keys/<name>`
// needed to extract the active version's public key.
type transitKeyResponse struct {
	Data struct {
		LatestVersion int `json:"latest_version"`
		Keys          map[string]struct {
			PublicKey string `json:"public_key"`
		} `json:"keys"`
	} `json:"data"`
}

// TransitPublicKeyPEM reads the PEM-encoded public key of the given transit key's
// active version directly from OpenBao, for cross-checking against the public key
// a KMS-backed signer (e.g. Rekor) reports using the same key resource.
func TransitPublicKeyPEM(ctx context.Context, namespace, keyName string) (string, error) {
	out, err := k8sSupport.ExecInPodWithOutput(ctx, PodName, containerName, namespace, "/bin/sh", "-c",
		fmt.Sprintf(`export VAULT_ADDR=http://127.0.0.1:%d VAULT_TOKEN=%s; bao read -format=json transit/keys/%s`, port, RootToken, keyName))
	if err != nil {
		return "", fmt.Errorf("reading transit key %s: %w", keyName, err)
	}

	var resp transitKeyResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("parsing transit key %s response: %w", keyName, err)
	}

	key, ok := resp.Data.Keys[fmt.Sprintf("%d", resp.Data.LatestVersion)]
	if !ok {
		return "", fmt.Errorf("transit key %s: no data for latest version %d", keyName, resp.Data.LatestVersion)
	}
	return key.PublicKey, nil
}

// waitForPodReady polls the Pod until its Ready condition is true.
func waitForPodReady(ctx context.Context, cli client.Client, namespace, name string) error {
	deadline := time.After(5 * time.Minute)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timeout waiting for pod %s to become ready", name)
		case <-ticker.C:
			p := &corev1.Pod{}
			if err := cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, p); err != nil {
				if errors.IsNotFound(err) {
					continue
				}
				return fmt.Errorf("getting pod %s: %w", name, err)
			}
			for _, cond := range p.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					return nil
				}
			}
		}
	}
}
