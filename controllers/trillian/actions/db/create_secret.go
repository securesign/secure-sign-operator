package db

import (
	"context"
	"fmt"
	"strconv"

	"github.com/securesign/operator/controllers/common"
	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/constants"
	actions2 "github.com/securesign/operator/controllers/trillian/actions"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	port = 3306
	host = "trillian-mysql"
)

func NewCreateSecretAction() action.Action[rhtasv1alpha1.Trillian] {
	return &createSecretAction{}
}

type createSecretAction struct {
	action.BaseAction
}

func (i createSecretAction) Name() string {
	return "create db secret"
}

func (i createSecretAction) CanHandle(instance *rhtasv1alpha1.Trillian) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating && instance.Spec.Db.Create && instance.Spec.Db.DatabaseSecretRef == nil
}

func (i createSecretAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {

	var (
		err error
	)
	dbLabels := constants.LabelsFor(actions2.DbComponentName, actions2.DbDeploymentName, instance.Name)

	dbSecret := i.createDbSecret(instance.Namespace, dbLabels)
	if err = controllerutil.SetControllerReference(instance, dbSecret, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for secret: %w", err))
	}

	// no watch on secret - continue if no error
	if _, err = i.Ensure(ctx, dbSecret); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions2.DbCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create DB secret: %w", err), instance)
	}

	instance.Spec.Db.DatabaseSecretRef = &corev1.LocalObjectReference{
		Name: dbSecret.Name,
	}
	return i.Update(ctx, instance)
}
func (i createSecretAction) createDbSecret(namespace string, labels map[string]string) *corev1.Secret {
	// Define a new Secret object
	var rootPass []byte
	var mysqlPass []byte
	rootPass = common.GeneratePassword(12)
	mysqlPass = common.GeneratePassword(12)
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "rhtas",
			Namespace:    namespace,
			Labels:       labels,
		},
		Type: "Opaque",
		Data: map[string][]byte{
			"mysql-root-password": rootPass,
			"mysql-password":      mysqlPass,
			"mysql-database":      []byte("trillian"),
			"mysql-user":          []byte("mysql"),
			"mysql-port":          []byte(strconv.Itoa(port)),
			"mysql-host":          []byte(host),
		},
	}
}
