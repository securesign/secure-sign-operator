package actions

import (
	"context"
	"errors"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewAlignStatusLogsAction() action.Action[*rhtasv1.CTlog] {
	return &alignStatusLogs{}
}

type alignStatusLogs struct {
	action.BaseAction
}

func (a alignStatusLogs) Name() string {
	return "align-status-logs"
}

func (a alignStatusLogs) CanHandle(_ context.Context, instance *rhtasv1.CTlog) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (a alignStatusLogs) Handle(ctx context.Context, instance *rhtasv1.CTlog) *action.Result {
	if len(instance.Spec.Logs) == 0 {
		return a.Error(ctx, errors.New("at least one log is required"), instance)
	}

	desired := buildStatusLogs(instance)

	// Validate unique logIds to prevent secret data corruption
	logIds := make(map[int64]string)
	for _, log := range desired {
		if log.LogId != nil {
			if prefix, exists := logIds[*log.LogId]; exists {
				err := fmt.Errorf("duplicate logIds in spec - logs %q and %q both use logId %d (would cause secret data corruption)", prefix, log.Prefix, *log.LogId)
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:    constants.ReadyCondition,
					Status:  metav1.ConditionFalse,
					Reason:  "InvalidLogConfiguration",
					Message: err.Error(),
				})
				return a.Error(ctx, err, instance)
			}
			logIds[*log.LogId] = log.Prefix
		}
	}

	if equality.Semantic.DeepEqual(desired, instance.Status.Logs) {
		return a.Continue()
	}
	// Warn if any logs are being removed from status
	for _, statusLog := range instance.Status.Logs {
		found := false
		for _, desiredLog := range desired {
			if desiredLog.Prefix == statusLog.Prefix {
				found = true
				break
			}
		}
		if !found {
			a.Logger.Info("Log removed from status (was present but not in spec)", "prefix", statusLog.Prefix)
		}
	}
	instance.Status.Logs = desired
	return a.ReturnOnChange(a.PersistStatus)(ctx, instance)
}

func buildStatusLogs(instance *rhtasv1.CTlog) []rhtasv1.CTlogLogStatus {
	logs := make([]rhtasv1.CTlogLogStatus, 0, len(instance.Spec.Logs))
	statusLogsMap := make(map[string]*rhtasv1.CTlogLogStatus)
	for i := range instance.Status.Logs {
		statusLogsMap[instance.Status.Logs[i].Prefix] = &instance.Status.Logs[i]
	}

	for _, specLog := range instance.Spec.Logs {
		logStatus := rhtasv1.CTlogLogStatus{
			Prefix: specLog.Prefix,
		}

		// Preserve existing status fields if present
		if existing, ok := statusLogsMap[specLog.Prefix]; ok {
			logStatus.LogId = existing.LogId
			logStatus.PublicKey = existing.PublicKey
			logStatus.PrivateKeyRef = existing.PrivateKeyRef
			logStatus.PublicKeyRef = existing.PublicKeyRef
			logStatus.RootCertificates = existing.RootCertificates
			logStatus.SignerType = existing.SignerType
			logStatus.PrivateKeyPasswordRef = existing.PrivateKeyPasswordRef
		}

		// Sync Active status from spec (false if not explicitly marked active)
		logStatus.Active = specLog.Active != nil && *specLog.Active

		// Apply spec overrides for all logs (active and non-active).
		// User-configured spec values always take priority over status.
		if specLog.LogId != nil {
			logStatus.LogId = specLog.LogId
		}
		if len(specLog.RootCerts) > 0 {
			logStatus.RootCertificates = specLog.RootCerts
		}
		if specLog.Signer != nil {
			logStatus.SignerType = specLog.Signer.Type
			if specLog.Signer.File != nil {
				// Track if private key reference is actually changing
				oldPrivateKeyRef := logStatus.PrivateKeyRef
				if specLog.Signer.File.PrivateKeyRef != nil {
					logStatus.PrivateKeyRef = specLog.Signer.File.PrivateKeyRef
					// Private key changed: clear password ref since it won't match new key
					if oldPrivateKeyRef == nil ||
						oldPrivateKeyRef.Name != specLog.Signer.File.PrivateKeyRef.Name ||
						oldPrivateKeyRef.Key != specLog.Signer.File.PrivateKeyRef.Key {
						logStatus.PrivateKeyPasswordRef = nil
					}
				}
				if specLog.Signer.File.PublicKeyRef != nil {
					logStatus.PublicKeyRef = specLog.Signer.File.PublicKeyRef
				} else if logStatus.PrivateKeyRef != nil {
					// For file-backed logs without explicit public key, derive it from the private key
					// (same convention as generate_signer.go for the active log).
					// This ensures non-active file-backed shards can serialize correctly.
					logStatus.PublicKeyRef = &rhtasv1.SecretKeySelector{
						LocalObjectReference: logStatus.PrivateKeyRef.LocalObjectReference,
						Key:                  "public",
					}
				}
			}
			if specLog.Signer.PKCS11 != nil {
				if specLog.Signer.PKCS11.PublicKeyRef != nil {
					logStatus.PublicKeyRef = &specLog.Signer.PKCS11.PublicKeyRef
				}
				// PKCS#11 doesn't use passwords
				logStatus.PrivateKeyPasswordRef = nil
			}
		}

		logs = append(logs, logStatus)
	}

	return logs
}
