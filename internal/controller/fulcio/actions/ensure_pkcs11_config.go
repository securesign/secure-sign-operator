package actions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	pkcs11helpers "github.com/securesign/operator/internal/controller/common/pkcs11"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewEnsurePKCS11ConfigAction() action.Action[*rhtasv1.Fulcio] {
	return &ensurePKCS11Config{}
}

type ensurePKCS11Config struct {
	action.BaseAction
}

func (a ensurePKCS11Config) Name() string {
	return "ensure PKCS#11 config"
}

func (a ensurePKCS11Config) CanHandle(_ context.Context, instance *rhtasv1.Fulcio) bool {
	if instance.Spec.Signer.Type != rhtasv1.SignerTypePKCS11 {
		return false
	}
	if state.FromInstance(instance, constants.ReadyCondition) < state.Creating {
		return false
	}

	cond := meta.FindStatusCondition(instance.Status.Conditions, PKCS11Condition)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		return true
	}

	// Drift detection: compare stored hash with computed hash
	currentHash := computePKCS11Hash(instance)
	return !strings.Contains(cond.Message, "[hash="+currentHash+"]")
}

func (a ensurePKCS11Config) Handle(ctx context.Context, instance *rhtasv1.Fulcio) *action.Result {
	if instance.Spec.Signer.PKCS11 == nil {
		return a.Error(ctx,
			reconcile.TerminalError(fmt.Errorf("PKCS#11 configuration is required when signer type is pkcs11")),
			instance,
		)
	}

	pkcs11Cfg := instance.Spec.Signer.PKCS11

	if pkcs11Cfg.ConfigRef == nil {
		return a.Error(ctx,
			reconcile.TerminalError(fmt.Errorf("spec.signer.pkcs11.configRef is required")),
			instance,
		)
	}

	// Validate ConfigRef secret exists
	if _, err := kubernetes.GetSecretData(ctx, a.Client, instance.Namespace, pkcs11Cfg.ConfigRef); err != nil {
		return a.Error(ctx, fmt.Errorf("PKCS#11 config secret not available: %w", err), instance,
			metav1.Condition{
				Type:               PKCS11Condition,
				Status:             metav1.ConditionFalse,
				Reason:             state.Creating.String(),
				Message:            fmt.Sprintf("Waiting for PKCS#11 config secret: %v", err),
				ObservedGeneration: instance.Generation,
			},
		)
	}

	// Set Status.Certificate.CARef from CertificateChain.CertificateChainRef
	certChainRef := instance.Spec.Signer.CertificateChain.CertificateChainRef
	if certChainRef == nil {
		return a.Error(ctx,
			reconcile.TerminalError(fmt.Errorf("certificateChain.certificateChainRef is required for PKCS#11 signer")),
			instance,
		)
	}

	// Validate that the CA certificate secret exists and contains valid data.
	if _, err := kubernetes.GetSecretData(ctx, a.Client, instance.Namespace, certChainRef); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               CertCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            fmt.Sprintf("Waiting for CA certificate secret: %v", err),
			ObservedGeneration: instance.Generation,
		})
		if _, persistErr := a.PersistStatus(ctx, instance); persistErr != nil {
			return a.Error(ctx, persistErr, instance)
		}
		return a.RequeueAfter(5 * time.Second)
	}

	if instance.Status.Certificate == nil {
		instance.Status.Certificate = &rhtasv1.FulcioCertStatus{}
	}
	instance.Status.Certificate.CARef = certChainRef.DeepCopy()
	// Note: Status.CertificateChain is intentionally NOT pre-populated here.
	// ResolvePubKey fetches it from the running Fulcio API and applies
	// ParseTrustBundle normalization (TrimSpace). Pre-populating with the raw
	// secret PEM (which may contain trailing whitespace) would cause a
	// false drift detection when the normalized API response differs,
	// permanently blocking the Ready transition.

	// Set CertCondition = True since the CA cert is resolved via certificateChainRef.
	// In file mode, generate_signer sets this; in PKCS#11 mode we set it here.
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               CertCondition,
		Status:             metav1.ConditionTrue,
		Reason:             state.Ready.String(),
		Message:            "CA certificate resolved from certificateChainRef",
		ObservedGeneration: instance.Generation,
	})

	// Set PKCS11Condition = True with content hash for drift detection
	contentHash := computePKCS11Hash(instance)
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               PKCS11Condition,
		Status:             metav1.ConditionTrue,
		Reason:             state.Ready.String(),
		Message:            fmt.Sprintf("PKCS#11 config resolved [hash=%s]", contentHash),
		ObservedGeneration: instance.Generation,
	})

	return a.ReturnOnChange(a.PersistStatus)(ctx, instance)
}

// computePKCS11Hash computes a deterministic SHA-256 hash of the PKCS#11 configuration
// fields used for drift detection. This hash is stored in the PKCS11Condition message
// and compared on subsequent reconciles to detect configuration changes without
// reacting to unrelated generation bumps (e.g., monitoring toggle).
func computePKCS11Hash(instance *rhtasv1.Fulcio) string {
	h := sha256.New()
	if instance.Spec.Signer.PKCS11 != nil {
		cfg := instance.Spec.Signer.PKCS11
		pkcs11helpers.HashCoreConfig(h, &cfg.PKCS11Config)
		if cfg.ConfigRef != nil {
			fmt.Fprintf(h, "configRef:%s/%s\n", cfg.ConfigRef.Name, cfg.ConfigRef.Key) //nolint:errcheck // hash.Hash.Write never returns an error
		}
	}
	if ref := instance.Spec.Signer.CertificateChain.CertificateChainRef; ref != nil {
		fmt.Fprintf(h, "certChainRef:%s/%s\n", ref.Name, ref.Key) //nolint:errcheck
	}
	return hex.EncodeToString(h.Sum(nil))
}
