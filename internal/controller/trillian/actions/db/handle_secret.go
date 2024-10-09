package db

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"k8s.io/apimachinery/pkg/api/equality"
	apierros "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/securesign/operator/internal/controller/common/utils"

	"github.com/securesign/operator/internal/controller/common"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	trillian "github.com/securesign/operator/internal/controller/trillian/actions"
	trillianUtils "github.com/securesign/operator/internal/controller/trillian/utils"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	port                   = 3306
	host                   = "trillian-mysql"
	user                   = "mysql"
	databaseName           = "trillian"
	dbConnectionResource   = "trillian-db-connection"
	dbConnectionSecretName = "trillian-db-connection-"

	annotationDatabase = constants.LabelNamespace + "/" + trillianUtils.SecretDatabaseName
	annotationUser     = constants.LabelNamespace + "/" + trillianUtils.SecretUser
	annotationPort     = constants.LabelNamespace + "/" + trillianUtils.SecretPort
	annotationHost     = constants.LabelNamespace + "/" + trillianUtils.SecretHost
)

var ErrMissingDBConfiguration = errors.New("expecting external DB configuration")

func NewHandleSecretAction() action.Action[*rhtasv1alpha1.Trillian] {
	return &handleSecretAction{}
}

type handleSecretAction struct {
	action.BaseAction
}

func (i handleSecretAction) Name() string {
	return "create db secret"
}

func (i handleSecretAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Trillian) bool {
	switch {
	case instance.Status.Db.DatabaseSecretRef == nil:
		return true
	case !equality.Semantic.DeepDerivative(instance.Spec.Db.DatabaseSecretRef, instance.Status.Db.DatabaseSecretRef):
		return true
	default:
		return !meta.IsStatusConditionTrue(instance.GetConditions(), trillian.DbCondition)
	}
}

func (i handleSecretAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {
	// external database
	if !utils.OptionalBool(instance.Spec.Db.Create) {
		if instance.Spec.Db.DatabaseSecretRef == nil {
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    trillian.DbCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: ErrMissingDBConfiguration.Error(),
			})
			return i.FailedWithStatusUpdate(ctx, ErrMissingDBConfiguration, instance)
		}

		if !equality.Semantic.DeepEqual(instance.Spec.Db.DatabaseSecretRef, instance.Status.Db.DatabaseSecretRef) {
			instance.Status.Db.DatabaseSecretRef = instance.Spec.Db.DatabaseSecretRef
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    trillian.DbCondition,
				Status:  metav1.ConditionTrue,
				Reason:  constants.Ready,
				Message: "Working with external DB",
			})
			return i.StatusUpdate(ctx, instance)
		}
		return i.Continue()
	}

	// managed database
	var (
		err error
	)
	if instance.Spec.Db.DatabaseSecretRef != nil {
		// skip if spec and status is equal
		if equality.Semantic.DeepEqual(instance.Spec.Db.DatabaseSecretRef, instance.Status.Db.DatabaseSecretRef) {
			return i.Continue()
		}

		// update database connection by spec
		instance.Status.Db.DatabaseSecretRef = instance.Spec.Db.DatabaseSecretRef
		return i.StatusUpdate(ctx, instance)
	}

	// skip if status exists
	if instance.Status.Db.DatabaseSecretRef != nil {
		return i.Continue()
	}

	dbLabels := constants.LabelsFor(trillian.DbComponentName, trillian.DbDeploymentName, instance.Name)
	dbLabels[constants.LabelResource] = dbConnectionResource

	partialSecrets, err := kubernetes.ListSecrets(ctx, i.Client, instance.Namespace, labels.SelectorFromSet(dbLabels).String())
	if err != nil {
		return i.Failed(fmt.Errorf("can't load secrets: %w", err))
	}

	for _, partialSecret := range partialSecrets.Items {
		// use first db-connection and remove all other
		if instance.Status.Db.DatabaseSecretRef == nil &&
			equality.Semantic.DeepDerivative(i.secretAnnotations(), partialSecret.GetAnnotations()) {
			instance.Status.Db.DatabaseSecretRef = &rhtasv1alpha1.LocalObjectReference{
				Name: partialSecret.Name,
			}
			continue
		}

		// delete unused secrets with db-connection
		err = i.Client.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Name:      partialSecret.GetName(),
			Namespace: partialSecret.GetNamespace(),
		}})
		if err != nil && !apierros.IsNotFound(err) {
			i.Logger.Error(err, "can't delete secret")
		}
	}

	if instance.Status.Db.DatabaseSecretRef != nil {
		return i.StatusUpdate(ctx, instance)
	}

	dbSecret := i.createDbSecret(instance.Namespace, dbLabels)
	if err = controllerutil.SetControllerReference(instance, dbSecret, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for secret: %w", err))
	}

	// no watch on secret - continue if no error
	if _, err = i.Ensure(ctx, dbSecret); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    trillian.DbCondition,
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

	instance.Status.Db.DatabaseSecretRef = &rhtasv1alpha1.LocalObjectReference{
		Name: dbSecret.Name,
	}
	return i.StatusUpdate(ctx, instance)
}
func (i handleSecretAction) createDbSecret(namespace string, labels map[string]string) *corev1.Secret {
	// Define a new Secret object
	var rootPass []byte
	var mysqlPass []byte
	rootPass = common.GeneratePassword(12)
	mysqlPass = common.GeneratePassword(12)
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: dbConnectionSecretName,
			Namespace:    namespace,
			Labels:       labels,
			Annotations:  i.secretAnnotations(),
		},
		Type: "Opaque",
		Data: map[string][]byte{
			trillianUtils.SecretRootPassword: rootPass,
			trillianUtils.SecretPassword:     mysqlPass,
			trillianUtils.SecretDatabaseName: []byte(databaseName),
			trillianUtils.SecretUser:         []byte(user),
			trillianUtils.SecretPort:         []byte(strconv.Itoa(port)),
			trillianUtils.SecretHost:         []byte(host),
		},
	}
}

func (i handleSecretAction) secretAnnotations() map[string]string {
	return map[string]string{
		annotationDatabase: databaseName,
		annotationUser:     user,
		annotationPort:     strconv.Itoa(port),
		annotationHost:     host,
	}
}
