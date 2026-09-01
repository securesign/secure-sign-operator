package actions

import (
	"context"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/ctlog/utils"
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
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Initialize
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

	for _, specLog := range instance.Spec.Logs {
		logStatus := rhtasv1.CTlogLogStatus{
			Prefix:   specLog.Prefix,
			Readonly: specLog.Readonly,
		}

		if utils.ActiveLog(instance.Spec.Logs) != nil && specLog.Active != nil && *specLog.Active {
			logStatus.LogId = instance.Status.TreeID
			logStatus.PrivateKeyRef = instance.Status.PrivateKeyRef
			logStatus.PrivateKeyPasswordRef = instance.Status.PrivateKeyPasswordRef
			logStatus.PublicKeyRef = instance.Status.PublicKeyRef
			logStatus.PublicKey = instance.Status.PublicKey
			logStatus.RootCertificates = instance.Status.RootCertificates
		} else {
			logStatus.LogId = specLog.LogId
			if specLog.RootCerts != nil {
				logStatus.RootCertificates = specLog.RootCerts.Roots
			}
			if specLog.Signer != nil && specLog.Signer.File != nil {
				logStatus.PrivateKeyRef = specLog.Signer.File.PrivateKeyRef
				logStatus.PublicKeyRef = specLog.Signer.File.PublicKeyRef
			}
		}

		logs = append(logs, logStatus)
	}

	return logs
}
