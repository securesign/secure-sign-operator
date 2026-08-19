package db

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	trillian "github.com/securesign/operator/internal/controller/trillian/actions"
	"github.com/securesign/operator/internal/controller/trillian/dbsecret"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	utils2 "github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierros "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apilabels "k8s.io/apimachinery/pkg/labels"
)

const (
	port                   = 3306
	host                   = "trillian-mysql"
	user                   = "mysql"
	databaseName           = "trillian"
	dbConnectionResource   = "trillian-db-connection"
	dbConnectionSecretName = "trillian-db-connection-"

	annotationDatabase = labels.LabelNamespace + "/" + dbsecret.SecretDatabaseName
	annotationUser     = labels.LabelNamespace + "/" + dbsecret.SecretUser
	annotationPort     = labels.LabelNamespace + "/" + dbsecret.SecretPort
	annotationHost     = labels.LabelNamespace + "/" + dbsecret.SecretHost
)

var managedAnnotations = []string{annotationDatabase, annotationUser, annotationPort, annotationHost}

func NewHandleSecretAction() action.Action[*rhtasv1.Trillian] {
	return &handleSecretAction{}
}

type handleSecretAction struct {
	action.BaseAction
}

func (i handleSecretAction) Name() string {
	return "create db secret"
}

func (i handleSecretAction) CanHandle(_ context.Context, instance *rhtasv1.Trillian) bool {
	switch {
	case utils2.OptionalBool(instance.Spec.Db.Create) && instance.Status.Db.DatabaseSecretRef == nil:
		return true
	default:
		return !meta.IsStatusConditionTrue(instance.GetConditions(), trillian.DbCondition)
	}
}

func (i handleSecretAction) Handle(ctx context.Context, instance *rhtasv1.Trillian) *action.Result {
	// external database
	if !utils2.OptionalBool(instance.Spec.Db.Create) {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    trillian.DbCondition,
			Status:  metav1.ConditionTrue,
			Reason:  state.Ready.String(),
			Message: "Working with external DB",
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}

	// managed database
	var (
		err error
	)

	// skip if status exists
	if instance.Status.Db.DatabaseSecretRef != nil {
		return i.Continue()
	}

	dbLabels := labels.For(trillian.DbComponentName, trillian.DbDeploymentName, instance.Name)
	dbLabels[labels.LabelResource] = dbConnectionResource

	partialSecrets, err := kubernetes.ListSecrets(ctx, i.Client, instance.Namespace, apilabels.SelectorFromSet(dbLabels).String())
	if err != nil {
		return i.Error(ctx, fmt.Errorf("can't load secrets: %w", err), instance)
	}

	for _, partialSecret := range partialSecrets.Items {
		// use first db-connection and remove all other
		if instance.Status.Db.DatabaseSecretRef == nil &&
			equality.Semantic.DeepDerivative(i.secretAnnotations(), partialSecret.GetAnnotations()) {
			instance.Status.Db.DatabaseSecretRef = &rhtasv1.LocalObjectReference{
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
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}

	dbSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: dbConnectionSecretName,
			Namespace:    instance.Namespace,
		},
	}
	if err = kubernetes.Create(ctx, i.Client,
		dbSecret,
		ensure.Labels[*corev1.Secret](slices.Collect(maps.Keys(dbLabels)), dbLabels),
		ensure.Annotations[*corev1.Secret](managedAnnotations, i.secretAnnotations()),
		kubernetes.EnsureSecretData(true, i.defaultDBData()),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("can't generate certificate secret: %w", err), instance,
			metav1.Condition{
				Type:    trillian.DbCondition,
				Status:  metav1.ConditionFalse,
				Reason:  state.Failure.String(),
				Message: err.Error(),
			})
	}

	instance.Status.Db.DatabaseSecretRef = &rhtasv1.LocalObjectReference{
		Name: dbSecret.Name,
	}
	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}
func (i handleSecretAction) defaultDBData() map[string][]byte {
	// Define a new Secret object
	var rootPass []byte
	var mysqlPass []byte
	rootPass = utils2.GeneratePassword(12)
	mysqlPass = utils2.GeneratePassword(12)
	return map[string][]byte{
		dbsecret.SecretRootPassword: rootPass,
		dbsecret.SecretPassword:     mysqlPass,
		dbsecret.SecretDatabaseName: []byte(databaseName),
		dbsecret.SecretUser:         []byte(user),
		dbsecret.SecretPort:         []byte(strconv.Itoa(port)),
		dbsecret.SecretHost:         []byte(host),
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
