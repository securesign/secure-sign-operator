package rekor

import (
	"context"
	"fmt"
	"net"

	"github.com/securesign/operator/controllers/common/action"
	v12 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"

	"io"
	"net/http"
	"time"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	commonUtils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewWaitAction() action.Action[rhtasv1alpha1.Rekor] {
	return &waitAction{}
}

type waitAction struct {
	action.BaseAction
}

func (i waitAction) Name() string {
	return "wait"
}

func (i waitAction) CanHandle(Rekor *rhtasv1alpha1.Rekor) bool {
	return Rekor.Status.Phase == rhtasv1alpha1.PhaseInitialize
}

func (i waitAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) (*rhtasv1alpha1.Rekor, error) {
	var (
		ok  bool
		err error
	)
	labels := commonUtils.FilterCommonLabels(instance.Labels)
	labels[commonUtils.ComponentLabel] = ComponentName
	ok, err = commonUtils.DeploymentIsRunning(ctx, i.Client, instance.Namespace, labels)
	if err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, err
	}
	if !ok {
		return instance, nil
	}

	// find internal service URL (don't use the `.status.Url` because it can be external Ingress route with untrusted CA
	rekor, err := commonUtils.GetInternalUrl(ctx, i.Client, instance.Namespace, RekorDeploymentName)
	if err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, err
	}

	inContainer, err := commonUtils.ContainerMode()
	if err == nil {
		if !inContainer {
			fmt.Println("Operator is running on localhost. You need to port-forward services.")
			for it := 0; it < 60; it++ {
				if rawConnect("localhost", "8080") {
					fmt.Println("Connection is open.")
					rekor = "localhost:8080"
					break
				} else {
					fmt.Println("Execute `oc port-forward service/rekor 8091 8080` in your namespace to continue.")
					time.Sleep(time.Duration(5) * time.Second)
				}
			}

		}
	} else {
		i.Logger.Info("Can't recognise operator mode - expecting in-container run")
	}

	var pubKeyResponse *http.Response
	for retry := 1; retry < 5; retry++ {
		time.Sleep(time.Duration(retry) * time.Second)
		pubKeyResponse, err = http.Get("http://" + rekor + "/api/v1/log/publicKey")
		if err == nil && pubKeyResponse.StatusCode == http.StatusOK {
			continue
		}
		i.Logger.Info("retrying to get rekor public key")
	}

	if err != nil || pubKeyResponse.StatusCode != http.StatusOK {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, err
	}
	body, err := io.ReadAll(pubKeyResponse.Body)
	if err != nil || pubKeyResponse.StatusCode != http.StatusOK {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, err
	}
	secret := commonUtils.CreateSecret("rekor-public-key", instance.Namespace, map[string][]byte{"key": body}, labels)
	controllerutil.SetControllerReference(instance, secret, i.Client.Scheme())
	if err = i.Client.Create(ctx, secret); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create rekor-public-key secret: %w", err)
	}

	if instance.Spec.ExternalAccess.Enabled {
		protocol := "http://"
		ingress := &v12.Ingress{}
		err = i.Client.Get(ctx, types.NamespacedName{Name: RekorDeploymentName, Namespace: instance.Namespace}, ingress)
		if err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, err
		}
		if len(ingress.Spec.TLS) > 0 {
			protocol = "https://"
		}
		instance.Status.Url = protocol + ingress.Spec.Rules[0].Host

	}
	instance.Status.Phase = rhtasv1alpha1.PhaseReady
	return instance, nil
}

func rawConnect(host string, port string) bool {
	timeout := time.Second
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), timeout)
	if err != nil {
		return false
	}
	if conn != nil {
		defer conn.Close()
		return true
	}
	return false
}
