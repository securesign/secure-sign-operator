package db

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	trillianUtils "github.com/securesign/operator/internal/controller/trillian/utils"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure/deployment"
	"github.com/securesign/operator/internal/utils/tls"

	"github.com/securesign/operator/internal/controller/trillian/actions"
	v2 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

const (
	livenessCommand  = "mariadb-admin -u ${MYSQL_USER} -p${MYSQL_PASSWORD} ping"
	readinessCommand = "mariadb -u ${MYSQL_USER} -p${MYSQL_PASSWORD} -e \"SELECT 1;\""
)

func NewDeployAction() action.Action[*rhtasv1alpha1.Trillian] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Trillian) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return (c.Reason == constants.Ready || c.Reason == constants.Creating) && enabled(instance)
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {
	var (
		err    error
		result controllerutil.OperationResult
	)

	labels := labels.For(actions.DbComponentName, actions.DbDeploymentName, instance.Name)

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v2.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.DbDeploymentName,
				Namespace: instance.Namespace,
			},
		},
		i.ensureDbDeployment(instance, actions.RBACDbName, labels),
		deployment.PodSecurityContext(),
		ensure.ControllerReference[*v2.Deployment](instance, i.Client),
		ensure.Labels[*v2.Deployment](slices.Collect(maps.Keys(labels)), labels),
		ensure.Optional(trillianUtils.UseTLSDb(instance), i.ensureTLS(statusTLS(instance))),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create Trillian DB: %w", err), instance, metav1.Condition{
			Type:    actions.DbCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.DbCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Database deployment created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}

func (i deployAction) ensureDbDeployment(instance *rhtasv1alpha1.Trillian, sa string, labels map[string]string) func(deployment *v2.Deployment) error {
	return func(dp *v2.Deployment) error {
		switch {
		case instance.Status.Db.DatabaseSecretRef == nil:
			{
				return errors.New("reference to database secret is not set")
			}
		case instance.Status.Db.Pvc.Name == "":
			{
				return errors.New("reference to database pvc is not set")
			}
		}

		var volumeName = "storage"

		spec := &dp.Spec
		spec.Replicas = utils.Pointer[int32](1)
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}
		spec.Strategy = v2.DeploymentStrategy{
			Type: "Recreate",
		}

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = sa

		volume := kubernetes.FindVolumeByNameOrCreate(&template.Spec, volumeName)
		if volume.PersistentVolumeClaim == nil {
			volume.PersistentVolumeClaim = &v1.PersistentVolumeClaimVolumeSource{}
		}
		volume.PersistentVolumeClaim.ClaimName = instance.Status.Db.Pvc.Name

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, actions.DbDeploymentName)
		container.Image = images.Registry.Get(images.TrillianDb)
		container.Command = []string{
			"run-mysqld",
		}

		port := kubernetes.FindPortByNameOrCreate(container, "3306-tcp")
		port.ContainerPort = 3306
		port.Protocol = v1.ProtocolTCP

		volumeMount := kubernetes.FindVolumeMountByNameOrCreate(container, volumeName)
		volumeMount.MountPath = "/var/lib/mysql"

		// Env variables from secret trillian-mysql
		userEnv := kubernetes.FindEnvByNameOrCreate(container, "MYSQL_USER")
		userEnv.ValueFrom = &v1.EnvVarSource{
			SecretKeyRef: &v1.SecretKeySelector{
				Key: actions.SecretUser,
				LocalObjectReference: v1.LocalObjectReference{
					Name: instance.Status.Db.DatabaseSecretRef.Name,
				},
			},
		}

		passwordEnv := kubernetes.FindEnvByNameOrCreate(container, "MYSQL_PASSWORD")
		passwordEnv.ValueFrom = &v1.EnvVarSource{
			SecretKeyRef: &v1.SecretKeySelector{
				Key: actions.SecretPassword,
				LocalObjectReference: v1.LocalObjectReference{
					Name: instance.Status.Db.DatabaseSecretRef.Name,
				},
			},
		}

		rootPasswordEnv := kubernetes.FindEnvByNameOrCreate(container, "MYSQL_ROOT_PASSWORD")
		rootPasswordEnv.ValueFrom = &v1.EnvVarSource{
			SecretKeyRef: &v1.SecretKeySelector{
				Key: actions.SecretRootPassword,
				LocalObjectReference: v1.LocalObjectReference{
					Name: instance.Status.Db.DatabaseSecretRef.Name,
				},
			},
		}

		portEnv := kubernetes.FindEnvByNameOrCreate(container, "MYSQL_PORT")
		portEnv.ValueFrom = &v1.EnvVarSource{
			SecretKeyRef: &v1.SecretKeySelector{
				Key: actions.SecretPort,
				LocalObjectReference: v1.LocalObjectReference{
					Name: instance.Status.Db.DatabaseSecretRef.Name,
				},
			},
		}

		dbEnv := kubernetes.FindEnvByNameOrCreate(container, "MYSQL_DATABASE")
		dbEnv.ValueFrom = &v1.EnvVarSource{
			SecretKeyRef: &v1.SecretKeySelector{
				Key: actions.SecretDatabaseName,
				LocalObjectReference: v1.LocalObjectReference{
					Name: instance.Status.Db.DatabaseSecretRef.Name,
				},
			},
		}

		if container.ReadinessProbe == nil {
			container.ReadinessProbe = &v1.Probe{}
		}
		if container.ReadinessProbe.Exec == nil {
			container.ReadinessProbe.Exec = &v1.ExecAction{}
		}

		container.ReadinessProbe.Exec.Command = []string{"bash", "-c", readinessCommand}
		container.ReadinessProbe.InitialDelaySeconds = 10

		if container.LivenessProbe == nil {
			container.LivenessProbe = &v1.Probe{}
		}
		if container.LivenessProbe.Exec == nil {
			container.LivenessProbe.Exec = &v1.ExecAction{}
		}

		container.LivenessProbe.Exec.Command = []string{"bash", "-c", livenessCommand}
		container.LivenessProbe.InitialDelaySeconds = 30
		return nil
	}
}

func (i deployAction) ensureTLS(tlsConfig rhtasv1alpha1.TLS) func(deployment *v2.Deployment) error {
	return func(dp *v2.Deployment) error {
		if err := deployment.TLS(tlsConfig, actions.DbDeploymentName)(dp); err != nil {
			return err
		}

		container := kubernetes.FindContainerByNameOrCreate(&dp.Spec.Template.Spec, actions.DbDeploymentName)

		if container.ReadinessProbe == nil {
			container.ReadinessProbe = &v1.Probe{}
		}
		if container.ReadinessProbe.Exec == nil {
			container.ReadinessProbe.Exec = &v1.ExecAction{}
		}

		container.ReadinessProbe.Exec.Command = []string{"bash", "-c", readinessCommand + " --ssl"}

		if container.LivenessProbe == nil {
			container.LivenessProbe = &v1.Probe{}
		}
		if container.LivenessProbe.Exec == nil {
			container.LivenessProbe.Exec = &v1.ExecAction{}
		}

		container.LivenessProbe.Exec.Command = []string{"bash", "-c", livenessCommand + " --ssl"}

		if i := slices.Index(container.Args, "--ssl-cert"); i == -1 {
			container.Args = append(container.Args, "--ssl-cert", tls.TLSCertPath)
		} else {
			if len(container.Args)-1 < i+1 {
				container.Args = append(container.Args, tls.TLSCertPath)
			}
			container.Args[i+1] = tls.TLSCertPath
		}

		if i := slices.Index(container.Args, "--ssl-key"); i == -1 {
			container.Args = append(container.Args, "--ssl-key", tls.TLSKeyPath)
		} else {
			if len(container.Args)-1 < i+1 {
				container.Args = append(container.Args, tls.TLSKeyPath)
			}
			container.Args[i+1] = tls.TLSKeyPath
		}
		return nil
	}
}
