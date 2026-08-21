package pkcs11

import (
	"context"
	"fmt"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/securesign/operator/internal/utils/kubernetes"
	k8sSupport "github.com/securesign/operator/test/e2e/support/kubernetes"
)

const (
	// SoftHSMInitImage is the image containing SoftHSM2 + Fulcio createca tooling.
	SoftHSMInitImage = "quay.io/securesign/softhsm-init:1.0-test"

	// HSM PIN used for SoftHSM token initialization.
	hsmPIN = "testpin123"

	// PKCS#11 token label used by both Fulcio and CTLog ceremonies.
	tokenLabel = "PKCS11CA"

	// Resource names matching what WithPKCS11Signer references.
	configMapSoftHSM     = "softhsm-config"
	secretHSMCredentials = "hsm-credentials"
	pvcFulcioTokens      = "hsm-tokens-pvc"
	pvcCTLogTokens       = "ctlog-hsm-tokens-pvc"
	secretFulcioRootCA   = "fulcio-root-ca"
	secretFulcioPKCS11   = "fulcio-pkcs11-config"
	secretCTLogPublicKey = "ctlog-public-key"
	jobFulcioCeremony    = "fulcio-kc"
	jobCTLogCeremony     = "ctlog-kc"

	// Markers used by ceremony jobs to delimit output in logs.
	fulcioCertStart  = "===S==="
	fulcioCertEnd    = "===E==="
	ctlogPubKeyStart = "===PK==="
	ctlogPubKeyEnd   = "===PKE==="
)

// CreatePrerequisites creates all Kubernetes resources needed before deploying
// a SecureSign CR with PKCS#11 signer configuration. This includes:
//   - softhsm-config ConfigMap
//   - hsm-credentials Secret (PIN)
//   - PVCs for Fulcio and CTLog HSM token stores
//   - Key ceremony Jobs (Fulcio CA + CTLog key generation)
//   - Secrets derived from ceremony output (root CA cert, crypto11 config, public key)
//
// The function blocks until all Jobs complete and all Secrets are created.
func CreatePrerequisites(ctx context.Context, cli client.Client, namespace string) error {
	// Step 1: Create ConfigMap, Secret, and PVCs.
	resources := []client.Object{
		softHSMConfigMap(namespace),
		hsmCredentialsSecret(namespace),
		hsmTokensPVC(namespace, pvcFulcioTokens),
		hsmTokensPVC(namespace, pvcCTLogTokens),
	}
	for _, r := range resources {
		if err := cli.Create(ctx, r); err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("creating %T %s: %w", r, r.GetName(), err)
			}
		}
	}

	// Step 2: Run key ceremony Jobs.
	fulcioJob := fulcioKeyCeremonyJob(namespace)
	ctlogJob := ctlogKeyCeremonyJob(namespace)
	for _, job := range []*batchv1.Job{fulcioJob, ctlogJob} {
		if err := cli.Create(ctx, job); err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("creating job %s: %w", job.Name, err)
			}
		}
	}

	// Step 3: Wait for both Jobs to complete.
	if err := waitForJobCompletion(ctx, cli, namespace, jobFulcioCeremony); err != nil {
		return fmt.Errorf("waiting for fulcio ceremony: %w", err)
	}
	if err := waitForJobCompletion(ctx, cli, namespace, jobCTLogCeremony); err != nil {
		return fmt.Errorf("waiting for ctlog ceremony: %w", err)
	}

	// Step 4: Extract Fulcio root CA cert from Job logs.
	fulcioCertPEM, err := extractFromJobLogs(ctx, cli, namespace, jobFulcioCeremony, fulcioCertStart, fulcioCertEnd)
	if err != nil {
		return fmt.Errorf("extracting fulcio root CA: %w", err)
	}

	// Step 5: Extract CTLog public key from Job logs.
	ctlogPubKeyPEM, err := extractFromJobLogs(ctx, cli, namespace, jobCTLogCeremony, ctlogPubKeyStart, ctlogPubKeyEnd)
	if err != nil {
		return fmt.Errorf("extracting ctlog public key: %w", err)
	}

	// Step 6: Create Secrets from extracted data.
	secrets := []client.Object{
		fulcioRootCASecret(namespace, fulcioCertPEM),
		fulcioPKCS11ConfigSecret(namespace),
		ctlogPublicKeySecret(namespace, ctlogPubKeyPEM),
	}
	for _, s := range secrets {
		if err := cli.Create(ctx, s); err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("creating secret %s: %w", s.GetName(), err)
			}
		}
	}

	// Step 7: Clean up ceremony Jobs (they are no longer needed).
	for _, job := range []*batchv1.Job{fulcioJob, ctlogJob} {
		propagation := metav1.DeletePropagationBackground
		if err := cli.Delete(ctx, job, &client.DeleteOptions{
			PropagationPolicy: &propagation,
		}); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("deleting job %s: %w", job.Name, err)
		}
	}

	return nil
}

// softHSMConfigMap creates the ConfigMap that tells SoftHSM where to store tokens.
func softHSMConfigMap(namespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapSoftHSM,
			Namespace: namespace,
		},
		Data: map[string]string{
			"softhsm2.conf": "directories.tokendir = /var/run/hsm-tokens\nobjectstore.backend = file\nlog.level = INFO\n",
		},
	}
}

// hsmCredentialsSecret creates the Secret containing the HSM PIN.
func hsmCredentialsSecret(namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretHSMCredentials,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"pin": []byte(hsmPIN),
		},
	}
}

// hsmTokensPVC creates a PVC for HSM token persistence.
func hsmTokensPVC(namespace, name string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("100Mi"),
				},
			},
		},
	}
}

// fulcioKeyCeremonyJob creates a Job that initializes the SoftHSM token,
// generates a P-256 ECDSA key pair, and creates a self-signed root CA
// certificate using Fulcio's createca command.
// The root CA cert PEM is emitted between ===S=== and ===E=== markers in stdout.
func fulcioKeyCeremonyJob(namespace string) *batchv1.Job {
	script := `
softhsm2-util --init-token --free --label PKCS11CA \
  --pin $HSM_PIN --so-pin $HSM_PIN
pkcs11-tool --module /usr/lib64/pkcs11/libsofthsm2.so \
  --login --pin $HSM_PIN --token-label PKCS11CA \
  --keypairgen --key-type EC:prime256v1 \
  --id 63 --label PKCS11CA
cat > /tmp/c.conf <<C
{"Path":"/usr/lib64/pkcs11/libsofthsm2.so","TokenLabel":"PKCS11CA","Pin":"$HSM_PIN"}
C
fulcio createca --org RHTAS --country US --province NC \
  --locality Raleigh --hsm-caroot-id 99 \
  --pkcs11-config-path /tmp/c.conf --out /tmp/r.pem
echo ===S=== && cat /tmp/r.pem && echo ===E===
`
	return keyCeremonyJob(namespace, jobFulcioCeremony, "ca", script, pvcFulcioTokens)
}

// ctlogKeyCeremonyJob creates a Job that initializes the SoftHSM token,
// generates a P-256 ECDSA key pair, and exports the public key in PEM format.
// The public key PEM is emitted between ===PK=== and ===PKE=== markers in stdout.
func ctlogKeyCeremonyJob(namespace string) *batchv1.Job {
	script := `
softhsm2-util --init-token --free --label PKCS11CA \
  --pin $HSM_PIN --so-pin $HSM_PIN
pkcs11-tool --module /usr/lib64/pkcs11/libsofthsm2.so \
  --login --pin $HSM_PIN --token-label PKCS11CA \
  --keypairgen --key-type EC:prime256v1 \
  --id 63 --label PKCS11CA
pkcs11-tool --module /usr/lib64/pkcs11/libsofthsm2.so \
  --login --pin $HSM_PIN --token-label PKCS11CA \
  --read-object --type pubkey --label PKCS11CA \
  -o /tmp/pk.der
openssl ec -inform DER -pubin -in /tmp/pk.der \
  -outform PEM -out /tmp/pk.pem
echo ===PK=== && cat /tmp/pk.pem && echo ===PKE===
`
	return keyCeremonyJob(namespace, jobCTLogCeremony, "kc", script, pvcCTLogTokens)
}

func keyCeremonyPodSecurityContext() *corev1.PodSecurityContext {
	sc := &corev1.PodSecurityContext{
		RunAsNonRoot: ptr.To(true),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
	if !kubernetes.IsOpenShift() {
		sc.RunAsUser = ptr.To(int64(1001))
		sc.RunAsGroup = ptr.To(int64(1001))
		sc.FSGroup = ptr.To(int64(1001))
	}
	return sc
}

// keyCeremonyJob creates a batch Job with the SoftHSM init image.
func keyCeremonyJob(namespace, name, containerName, script, pvcName string) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr.To[int32](0),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:   corev1.RestartPolicyNever,
					SecurityContext: keyCeremonyPodSecurityContext(),
					Containers: []corev1.Container{
						{
							Name:            containerName,
							Image:           SoftHSMInitImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"/bin/bash", "-c"},
							Args:            []string{script},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
							Env: []corev1.EnvVar{
								{
									Name: "HSM_PIN",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: secretHSMCredentials,
											},
											Key: "pin",
										},
									},
								},
								{
									Name:  "SOFTHSM2_CONF",
									Value: "/etc/softhsm/softhsm2.conf",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "sc",
									MountPath: "/etc/softhsm",
									ReadOnly:  true,
								},
								{
									Name:      "ht",
									MountPath: "/var/run/hsm-tokens",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "sc",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: configMapSoftHSM,
									},
								},
							},
						},
						{
							Name: "ht",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvcName,
								},
							},
						},
					},
				},
			},
		},
	}
}

// fulcioRootCASecret creates the Secret containing the Fulcio root CA certificate.
func fulcioRootCASecret(namespace, certPEM string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretFulcioRootCA,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"cert.pem": []byte(certPEM),
		},
	}
}

// fulcioPKCS11ConfigSecret creates the Secret containing the crypto11 JSON config.
// The Path points to /var/run/hsm-lib/libsofthsm2.so where the init container
// copies the PKCS#11 module.
func fulcioPKCS11ConfigSecret(namespace string) *corev1.Secret {
	config := fmt.Sprintf(`{
  "Path": "/var/run/hsm-lib/libsofthsm2.so",
  "TokenLabel": "%s",
  "Pin": "%s"
}`, tokenLabel, hsmPIN)

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretFulcioPKCS11,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"crypto11.conf": []byte(config),
		},
	}
}

// ctlogPublicKeySecret creates the Secret containing the CTLog public key.
func ctlogPublicKeySecret(namespace, pubKeyPEM string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretCTLogPublicKey,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"public.pem": []byte(pubKeyPEM),
		},
	}
}

// waitForJobCompletion polls the Job until it reaches Complete or Failed status.
func waitForJobCompletion(ctx context.Context, cli client.Client, namespace, name string) error {
	job := &batchv1.Job{}
	key := client.ObjectKey{Namespace: namespace, Name: name}

	deadline := time.After(5 * time.Minute)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timeout waiting for job %s to complete", name)
		case <-ticker.C:
			if err := cli.Get(ctx, key, job); err != nil {
				return fmt.Errorf("getting job %s: %w", name, err)
			}
			for _, c := range job.Status.Conditions {
				if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
					return nil
				}
				if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
					return fmt.Errorf("job %s failed: %s", name, c.Message)
				}
			}
		}
	}
}

// extractFromJobLogs reads the logs of the first Pod owned by the given Job
// and extracts the content between the specified start and end markers.
func extractFromJobLogs(ctx context.Context, cli client.Client, namespace, jobName, startMarker, endMarker string) (string, error) {
	// Find the Pod created by the Job.
	podList := &corev1.PodList{}
	if err := cli.List(ctx, podList,
		client.InNamespace(namespace),
		client.MatchingLabels{"job-name": jobName},
	); err != nil {
		return "", fmt.Errorf("listing pods for job %s: %w", jobName, err)
	}
	if len(podList.Items) == 0 {
		return "", fmt.Errorf("no pods found for job %s", jobName)
	}

	pod := podList.Items[0]
	containerName := pod.Spec.Containers[0].Name

	// Use the existing pod log helper.
	logs, err := k8sSupport.GetPodLogs(ctx, pod.Name, containerName, namespace)
	if err != nil {
		return "", fmt.Errorf("reading logs for pod %s: %w", pod.Name, err)
	}

	// Extract content between markers.
	startIdx := strings.Index(logs, startMarker)
	if startIdx == -1 {
		return "", fmt.Errorf("start marker %q not found in logs of pod %s", startMarker, pod.Name)
	}
	startIdx += len(startMarker)

	endIdx := strings.Index(logs[startIdx:], endMarker)
	if endIdx == -1 {
		return "", fmt.Errorf("end marker %q not found in logs of pod %s", endMarker, pod.Name)
	}

	extracted := strings.TrimSpace(logs[startIdx : startIdx+endIdx])
	if extracted == "" {
		return "", fmt.Errorf("empty content between markers in logs of pod %s", pod.Name)
	}

	return extracted, nil
}
