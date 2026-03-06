package db

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	utils2 "github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"k8s.io/apimachinery/pkg/api/equality"
	apierros "k8s.io/apimachinery/pkg/api/errors"
	apilabels "k8s.io/apimachinery/pkg/labels"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	console "github.com/securesign/operator/internal/controller/console/actions"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	port                   = 3306
	host                   = "console-db"
	user                   = "mysql"
	databaseName           = "console"
	dbConnectionResource   = "console-db-connection"
	dbConnectionSecretName = "console-db-connection-"

	annotationDatabase = labels.LabelNamespace + "/" + console.SecretDatabaseName
	annotationUser     = labels.LabelNamespace + "/" + console.SecretUser
	annotationPort     = labels.LabelNamespace + "/" + console.SecretPort
	annotationHost     = labels.LabelNamespace + "/" + console.SecretHost
)

var managedAnnotations = []string{annotationDatabase, annotationUser, annotationPort, annotationHost}

var ErrMissingDBConfiguration = errors.New("expecting external DB configuration")

func NewHandleSecretAction() action.Action[*rhtasv1alpha1.Console] {
	return &handleSecretAction{}
}

type handleSecretAction struct {
	action.BaseAction
}

func (i handleSecretAction) Name() string {
	return "create db secret"
}

func (i handleSecretAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Console) bool {
	switch {
	case instance.Status.DatabaseSecretRef == nil:
		return true
	default:
		return !meta.IsStatusConditionTrue(instance.GetConditions(), console.DbCondition)
	}
}

func (i handleSecretAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Console) *action.Result {

	// managed database
	var (
		err error
	)

	// skip if status exists
	if instance.Status.DatabaseSecretRef != nil {
		return i.Continue()
	}

	dbLabels := labels.For(console.DbComponentName, console.DbDeploymentName, instance.Name)
	dbLabels[labels.LabelResource] = dbConnectionResource

	partialSecrets, err := kubernetes.ListSecrets(ctx, i.Client, instance.Namespace, apilabels.SelectorFromSet(dbLabels).String())
	if err != nil {
		return i.Error(ctx, fmt.Errorf("can't load secrets: %w", err), instance)
	}

	for _, partialSecret := range partialSecrets.Items {
		// use first db-connection and remove all other
		if instance.Status.DatabaseSecretRef == nil &&
			equality.Semantic.DeepDerivative(i.secretAnnotations(), partialSecret.GetAnnotations()) {
			instance.Status.DatabaseSecretRef = &rhtasv1alpha1.LocalObjectReference{
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

	if instance.Status.DatabaseSecretRef != nil {
		return i.StatusUpdate(ctx, instance)
	}

	dbSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: dbConnectionSecretName,
			Namespace:    instance.Namespace,
		},
	}
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		dbSecret,
		ensure.Labels[*corev1.Secret](slices.Collect(maps.Keys(dbLabels)), dbLabels),
		ensure.Annotations[*corev1.Secret](managedAnnotations, i.secretAnnotations()),
		kubernetes.EnsureSecretData(true, i.defaultDBData()),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("can't generate certificate secret: %w", err), instance,
			metav1.Condition{
				Type:    console.DbCondition,
				Status:  metav1.ConditionFalse,
				Reason:  state.Failure.String(),
				Message: err.Error(),
			})
	}

	instance.Status.DatabaseSecretRef = &rhtasv1alpha1.LocalObjectReference{
		Name: dbSecret.Name,
	}
	return i.StatusUpdate(ctx, instance)
}
func (i handleSecretAction) defaultDBData() map[string][]byte {
	// Define a new Secret object
	var rootPass []byte
	var mysqlPass []byte
	rootPass = utils2.GeneratePassword(12)
	mysqlPass = utils2.GeneratePassword(12)
	dsn := fmt.Sprintf("mysql:%s@tcp(%s:%d)/%s", mysqlPass, host, port, databaseName)
	return map[string][]byte{
		console.SecretRootPassword: rootPass,
		console.SecretPassword:     mysqlPass,
		console.SecretDatabaseName: []byte(databaseName),
		console.SecretUser:         []byte(user),
		console.SecretPort:         []byte(strconv.Itoa(port)),
		console.SecretHost:         []byte(host),
		console.SecretDsn:          []byte(dsn),
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
