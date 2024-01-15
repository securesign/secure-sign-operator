package trillian

import (
	"context"
	"fmt"
	"github.com/securesign/operator/controllers/common/action"
	"net"
	"time"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/trillian/utils"
)

func NewInitializeAction() action.Action[rhtasv1alpha1.Trillian] {
	return &initializeAction{}
}

type initializeAction struct {
	action.BaseAction
}

func (i initializeAction) Name() string {
	return "initialize"
}

func (i initializeAction) CanHandle(trillian *rhtasv1alpha1.Trillian) bool {
	return trillian.Status.Phase == rhtasv1alpha1.PhaseInitialize
}

func (i initializeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) (*rhtasv1alpha1.Trillian, error) {
	url := instance.Status.Url
	inContainer, err := kubernetes.ContainerMode()
	if err == nil {
		if !inContainer {
			fmt.Println("Operator is running on localhost. You need to port-forward services.")
			for it := 0; it < 60; it++ {
				if rawConnect("localhost", "8091") {
					fmt.Println("Connection is open.")
					url = "localhost:8091"
					break
				} else {
					fmt.Println("Execute `oc port-forward service/trillian-logserver 8091 8091` in your namespace to continue.")
					time.Sleep(time.Duration(5) * time.Second)
				}
			}

		}
	} else {
		i.Logger.Info("Can't recognise operator mode - expecting in-container run")
	}

	tree, err := trillianUtils.CreateTrillianTree(ctx, url)
	if err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create Trillian tree: %w", err)
	}

	instance.Status.TreeID = tree.TreeId
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
