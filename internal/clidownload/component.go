package clidownload

import (
	"context"

	consolev1 "github.com/openshift/api/console/v1"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:rbac:groups=console.openshift.io,resources=consoleclidownloads,resourceNames=cosign;rekor-cli;gitsign;ec;fetch-tsa-certs;createtree;updatetree;tuftool,verbs=get;delete

var legacyNames = []string{
	"cosign",
	"rekor-cli",
	"gitsign",
	"ec",
	"fetch-tsa-certs",
	"createtree",
	"updatetree",
	"tuftool",
}

type MigrationComponent struct {
	Client client.Client
	Log    logr.Logger
}

func (c *MigrationComponent) Start(ctx context.Context) error {
	if !kubernetes.IsOpenShift() {
		return nil
	}

	c.Log.Info("cleaning up legacy ConsoleCLIDownload resources")

	for _, name := range legacyNames {
		obj := &consolev1.ConsoleCLIDownload{}
		if err := c.Client.Get(ctx, client.ObjectKey{Name: name}, obj); err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			c.Log.Error(err, "failed to get legacy ConsoleCLIDownload", "name", name)
			continue
		}

		if obj.Labels[labels.LabelAppPartOf] != constants.AppName {
			c.Log.Info("skipping ConsoleCLIDownload not owned by this operator", "name", name)
			continue
		}

		if err := c.Client.Delete(ctx, obj); err != nil {
			c.Log.Error(err, "failed to delete legacy ConsoleCLIDownload", "name", name)
			continue
		}
		c.Log.Info("deleted legacy ConsoleCLIDownload", "name", name)
	}
	return nil
}

func (c *MigrationComponent) NeedLeaderElection() bool {
	return true
}
