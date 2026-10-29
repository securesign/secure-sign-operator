package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/controller/trillian/dbsecret"
	trillianUtils "github.com/securesign/operator/internal/controller/trillian/utils"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/tls"

	"github.com/securesign/operator/internal/controller/trillian/actions"
	v2 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"

	rhtasv1 "github.com/securesign/operator/api/v1"
)

const (
	livenessCommand  = "mariadb-admin -u ${MYSQL_USER} -p${MYSQL_PASSWORD} ping"
	readinessCommand = "mariadb -u ${MYSQL_USER} -p${MYSQL_PASSWORD} -e \"SELECT 1;\""
)

func NewDeployAction() action.Action[*rhtasv1.Trillian] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(ctx context.Context, instance *rhtasv1.Trillian) bool {
	return enabled(instance) && state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1.Trillian) *action.Result {

	dp, err := traits.Apply(
		appsv1ac.Deployment(actions.DbDeploymentName, instance.Namespace),
		traits.ControllerReference[*appsv1ac.DeploymentApplyConfiguration](instance, i.Client),
		traits.Labels[*appsv1ac.DeploymentApplyConfiguration](labels.For(actions.DbComponentName, actions.DbDeploymentName, instance.Name)),
		i.ensureDbDeployment(instance),
		traits.WithPodSpecTraits(
			traits.SecurityContexts(actions.DbDeploymentName),
			traits.GODEBUG(instance.GetAnnotations(), actions.DbDeploymentName),
			traits.ServiceAccount(actions.RBACDbName),
			traits.Optional(trillianUtils.UseTLSDb(instance), i.ensureTLS(statusTLS(instance))),
		),
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not build Trillian DB deployment: %w", err), instance, metav1.Condition{
			Type: actions.DbCondition, Status: metav1.ConditionFalse,
			Reason: state.Failure.String(), Message: err.Error(),
		})
	}
	changed, err := kubernetes.Apply(ctx, i.Client, dp, &v2.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: actions.DbDeploymentName, Namespace: instance.Namespace},
	})
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not create Trillian DB: %w", err), instance, metav1.Condition{
			Type:    actions.DbCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Failure.String(),
			Message: err.Error(),
		})
	}

	if changed {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.DbCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Creating.String(),
			Message: "Database deployment created",
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	} else {
		return i.Continue()
	}
}

func (i deployAction) ensureDbDeployment(instance *rhtasv1.Trillian) traits.Trait[*appsv1ac.DeploymentApplyConfiguration] {
	return func(dp *appsv1ac.DeploymentApplyConfiguration) error {
		switch {
		case instance.Status.Db.DatabaseSecretRef == nil:
			return errors.New("reference to database secret is not set")
		case instance.Status.Db.PvcName == "":
			return errors.New("reference to database pvc is not set")
		}
		const volumeName = "storage"
		execProbe := func(command string) *corev1ac.ProbeApplyConfiguration {
			return corev1ac.Probe().WithExec(corev1ac.ExecAction().WithCommand("bash", "-c", command))
		}
		container := corev1ac.Container().
			WithName(actions.DbDeploymentName).
			WithImage(images.Registry.Get(images.TrillianDb)).
			WithCommand("run-mysqld").
			WithPorts(corev1ac.ContainerPort().
				WithName("3306-tcp").
				WithContainerPort(3306).
				WithProtocol(v1.ProtocolTCP)).
			WithVolumeMounts(corev1ac.VolumeMount().
				WithName(volumeName).
				WithMountPath("/var/lib/mysql")).
			WithReadinessProbe(execProbe(readinessCommand).
				WithPeriodSeconds(10).
				WithTimeoutSeconds(1).
				WithFailureThreshold(3)).
			WithLivenessProbe(execProbe(livenessCommand).
				WithPeriodSeconds(10).
				WithTimeoutSeconds(1).
				WithFailureThreshold(3)).
			WithStartupProbe(execProbe(readinessCommand).
				WithPeriodSeconds(5).
				WithTimeoutSeconds(5).
				WithFailureThreshold(24))

		for _, key := range []string{
			dbsecret.SecretRootPassword,
			dbsecret.SecretUser,
			dbsecret.SecretPassword,
			dbsecret.SecretPort,
			dbsecret.SecretDatabaseName,
		} {
			container.WithEnv(corev1ac.EnvVar().
				WithName(dbsecret.EnvName(key)).
				WithValueFrom(corev1ac.EnvVarSource().
					WithSecretKeyRef(corev1ac.SecretKeySelector().
						WithName(instance.Status.Db.DatabaseSecretRef.Name).
						WithKey(key))))
		}
		dp.WithSpec(appsv1ac.DeploymentSpec().
			WithSelector(metav1ac.LabelSelector().WithMatchLabels(dp.Labels)).
			WithStrategy(appsv1ac.DeploymentStrategy().WithType(v2.RecreateDeploymentStrategyType)).
			WithReplicas(1).
			WithTemplate(corev1ac.PodTemplateSpec().
				WithLabels(dp.Labels).
				WithSpec(corev1ac.PodSpec().
					WithContainers(container).
					WithVolumes(corev1ac.Volume().
						WithName(volumeName).
						WithPersistentVolumeClaim(corev1ac.PersistentVolumeClaimVolumeSource().
							WithClaimName(instance.Status.Db.PvcName))))))
		return nil
	}
}

func (i deployAction) ensureTLS(tlsConfig rhtasv1.TLS) traits.Trait[*corev1ac.PodSpecApplyConfiguration] {
	return func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
		if err := traits.TLS(tlsConfig, actions.DbDeploymentName)(podSpec); err != nil {
			return err
		}
		execProbeWithSSL := func(command string) *corev1ac.ExecActionApplyConfiguration {
			return corev1ac.ExecAction().WithCommand("bash", "-c", command+" --ssl")
		}
		container := kubernetes.FindContainerByName(podSpec, actions.DbDeploymentName)
		container.ReadinessProbe.Exec = execProbeWithSSL(readinessCommand)
		container.LivenessProbe.Exec = execProbeWithSSL(livenessCommand)
		container.StartupProbe.Exec = execProbeWithSSL(readinessCommand)
		container.WithArgs("--ssl-cert", tls.TLSCertPath, "--ssl-key", tls.TLSKeyPath)
		return nil
	}
}
