package actions

import (
	"context"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	"k8s.io/apimachinery/pkg/api/equality"
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
	desired := buildStatusLogs(instance)
	if equality.Semantic.DeepEqual(desired, instance.Status.Logs) {
		return a.Continue()
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
			Prefix:        specLog.Prefix,
			Readonly:      specLog.Readonly,
			NotAfterStart: specLog.NotAfterStart,
			NotAfterLimit: specLog.NotAfterLimit,
			FrozenSTH:     specLog.FrozenSTH,
		}

		// Preserve existing status fields if present
		if existing, ok := statusLogsMap[specLog.Prefix]; ok {
			logStatus.LogId = existing.LogId
			logStatus.PublicKey = existing.PublicKey
			logStatus.PrivateKeyRef = existing.PrivateKeyRef
			logStatus.PublicKeyRef = existing.PublicKeyRef
			logStatus.RootCertificates = existing.RootCertificates
		}

		// Override status with spec values when user explicitly configures them
		// (applies to both active and readonly logs)
		if specLog.LogId != nil {
			logStatus.LogId = specLog.LogId
		}
		if len(specLog.RootCerts) > 0 {
			logStatus.RootCertificates = specLog.RootCerts
		}
		if specLog.Signer != nil {
			logStatus.SignerType = specLog.Signer.Type
			// Only override key refs if explicitly provided in spec
			if specLog.Signer.File != nil {
				if specLog.Signer.File.PrivateKeyRef != nil {
					logStatus.PrivateKeyRef = specLog.Signer.File.PrivateKeyRef
				}
				if specLog.Signer.File.PublicKeyRef != nil {
					logStatus.PublicKeyRef = specLog.Signer.File.PublicKeyRef
				}
			}
			if specLog.Signer.PKCS11 != nil {
				if specLog.Signer.PKCS11.PublicKeyRef != nil {
					logStatus.PublicKeyRef = specLog.Signer.PKCS11.PublicKeyRef
				}
				if specLog.Signer.PKCS11.PinSecretRef != nil {
					logStatus.PinSecretRef = specLog.Signer.PKCS11.PinSecretRef
				}
				if specLog.Signer.PKCS11.TokenLabel != "" {
					logStatus.PKCS11TokenLabel = specLog.Signer.PKCS11.TokenLabel
				}
			}
		}

		if specLog.Active != nil && *specLog.Active {
			logStatus.Active = true
		}

		logs = append(logs, logStatus)
	}

	return logs
}
