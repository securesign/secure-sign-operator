package tuf

import (
	"context"
	"fmt"
	"reflect"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	tufutils "github.com/securesign/operator/controllers/tuf/utils"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	ComponentName      = "tuf"
	DeploymentName     = "tuf"
	ServiceAccountName = "tuf"
)

func NewReconcileAction() action.Action[rhtasv1alpha1.Tuf] {
	return &reconcileAction{}
}

type reconcileAction struct {
	action.BaseAction
}

func (i reconcileAction) Name() string {
	return "reconcile"
}

func (i reconcileAction) CanHandle(tuf *rhtasv1alpha1.Tuf) bool {
	return tuf.Status.Phase == rhtasv1alpha1.PhaseCreating || tuf.Status.Phase == rhtasv1alpha1.PhaseReady
}

func (i reconcileAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Tuf) (*rhtasv1alpha1.Tuf, error) {
	var err error

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: string(rhtasv1alpha1.PhaseReady),
		Status: metav1.ConditionTrue, Reason: string(rhtasv1alpha1.PhaseCreating)})

	if err = i.ensureSA(ctx, instance); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, err
	}

	if err = i.ensureDeployment(ctx, instance); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, err
	}

	var service *v1.Service
	if service, err = i.ensureService(ctx, instance); err != nil && service != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, err
	}

	if err = i.ensureIngress(ctx, instance, service); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, err
	}

	if instance.Status.Phase == rhtasv1alpha1.PhaseCreating {
		instance.Status.Phase = rhtasv1alpha1.PhaseInitialize
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: string(rhtasv1alpha1.PhaseReady),
			Status: metav1.ConditionTrue, Reason: string(rhtasv1alpha1.PhaseInitialize)})
	}
	return instance, nil
}

func (i reconcileAction) ensureSA(ctx context.Context, instance *rhtasv1alpha1.Tuf) error {
	var err error
	labels := constants.LabelsFor(ComponentName, ComponentName, instance.Name)
	ok := types.NamespacedName{Name: DeploymentName, Namespace: instance.Namespace}

	sa := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceAccountName,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
	}

	if err = ctrl.SetControllerReference(instance, sa, i.Client.Scheme()); err != nil {
		return fmt.Errorf("could not set controll reference for SA: %w", err)
	}
	if err = i.ensure(ctx, ok, sa); err != nil {
		return fmt.Errorf("could not create SA: %w", err)
	}

	role := kubernetes.CreateRole(instance.Namespace, DeploymentName, labels, []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"configmaps"},
			Verbs:     []string{"create", "get", "update"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"secrets"},
			Verbs:     []string{"create", "get", "update"},
		},
	})

	if err = ctrl.SetControllerReference(instance, role, i.Client.Scheme()); err != nil {
		return fmt.Errorf("could not set controll reference for Role: %w", err)
	}
	if err = i.ensure(ctx, ok, role); err != nil {
		return fmt.Errorf("could not create Role: %w", err)
	}

	rb := kubernetes.CreateRoleBinding(instance.Namespace, DeploymentName, labels, rbacv1.RoleRef{
		APIGroup: v1.SchemeGroupVersion.Group,
		Kind:     "Role",
		Name:     DeploymentName,
	},
		[]rbacv1.Subject{
			{Kind: "ServiceAccount", Name: DeploymentName, Namespace: instance.Namespace},
		})

	if err = ctrl.SetControllerReference(instance, rb, i.Client.Scheme()); err != nil {
		return fmt.Errorf("could not set controll reference for RoleBinding: %w", err)
	}
	if err = i.ensure(ctx, ok, rb); err != nil {
		return fmt.Errorf("could not create RoleBinding: %w", err)
	}
	return nil
}

func (i reconcileAction) ensureDeployment(ctx context.Context, instance *rhtasv1alpha1.Tuf) error {
	var err error

	ok := types.NamespacedName{Name: DeploymentName, Namespace: instance.Namespace}
	labels := constants.LabelsFor(ComponentName, ComponentName, instance.Name)

	db := tufutils.CreateTufDeployment(instance, DeploymentName, labels, ServiceAccountName)

	if err = controllerutil.SetControllerReference(instance, db, i.Client.Scheme()); err != nil {
		return fmt.Errorf("could not set controller reference for Deployment: %w", err)
	}
	if err = i.ensure(ctx, ok, db); err != nil {
		return fmt.Errorf("could not create TUF: %w", err)
	}

	return nil
}

func (i reconcileAction) ensureService(ctx context.Context, instance *rhtasv1alpha1.Tuf) (*v1.Service, error) {
	var err error

	ok := types.NamespacedName{Name: DeploymentName, Namespace: instance.Namespace}
	labels := constants.LabelsFor(ComponentName, ComponentName, instance.Name)

	svc := kubernetes.CreateService(instance.Namespace, DeploymentName, 8080, labels)
	//patch the pregenerated service
	svc.Spec.Ports[0].Port = instance.Spec.Port
	if err = controllerutil.SetControllerReference(instance, svc, i.Client.Scheme()); err != nil {
		return svc, fmt.Errorf("could not set controller reference for Service: %w", err)
	}
	if err = i.ensure(ctx, ok, svc); err != nil {
		return svc, fmt.Errorf("could not create service: %w", err)
	}

	return svc, nil
}

func (i reconcileAction) ensureIngress(ctx context.Context, instance *rhtasv1alpha1.Tuf, service *v1.Service) error {
	var err error
	ok := client.ObjectKey{Name: service.Name, Namespace: service.Namespace}
	labels := constants.LabelsFor(ComponentName, ComponentName, instance.Name)

	ingress, err := kubernetes.CreateIngress(ctx, i.Client, *service, instance.Spec.ExternalAccess, "tuf", labels)
	if err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return fmt.Errorf("could not create ingress: %w", err)
	}

	if err = controllerutil.SetControllerReference(instance, ingress, i.Client.Scheme()); err != nil {
		return fmt.Errorf("could not set controller reference for Route: %w", err)
	}

	if instance.Spec.ExternalAccess.Enabled {
		if err = i.ensure(ctx, ok, ingress); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return fmt.Errorf("could not create route: %w", err)
		}
	} else {
		if err = i.Client.Delete(ctx, ingress); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

func (i reconcileAction) ensure(ctx context.Context, key types.NamespacedName, obj client.Object) error {
	expectedSpec := reflect.ValueOf(obj.DeepCopyObject()).Elem().FieldByName("Spec")

	if err := i.Client.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			i.Logger.Info("Creating object",
				"kind", reflect.TypeOf(obj).Elem().Name(), "name", key.Name)
			if err = i.Client.Create(ctx, obj); err != nil {
				i.Logger.Error(err, "Failed to create new object",
					"kind", reflect.TypeOf(obj).Elem().Name(), "Namespace", key.Namespace, "Name", key.Name)
				return err
			}
			return nil
		}
		return err
	}

	currentSpec := reflect.ValueOf(obj).Elem().FieldByName("Spec")
	if !expectedSpec.IsValid() || !currentSpec.IsValid() {
		return nil
	}

	if equality.Semantic.DeepDerivative(expectedSpec.Interface(), currentSpec.Interface()) {
		return nil
	}

	i.Logger.Info("Updating object",
		"kind", reflect.TypeOf(obj).Elem().Name(), "Namespace", key.Namespace, "Name", key.Name)
	if err := i.Client.Update(ctx, obj); err != nil {
		i.Logger.Error(err, "Failed to update object",
			"kind", reflect.TypeOf(obj).Elem().Name(), "Namespace", key.Namespace, "Name", key.Name)
		return err
	}
	return nil
}
