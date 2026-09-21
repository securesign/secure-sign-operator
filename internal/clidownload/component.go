package clidownload

import (
	"context"

	consolev1 "github.com/openshift/api/console/v1"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"

	"github.com/go-logr/logr"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	cliServerNamespace = "trusted-artifact-signer"
	cliServerName      = "cli-server"
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

	c.deleteLegacyCLIDownloads(ctx)
	c.deleteLegacyCLIServer(ctx)

	return nil
}

func (c *MigrationComponent) deleteLegacyCLIDownloads(ctx context.Context) {
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
}

func (c *MigrationComponent) deleteLegacyCLIServer(ctx context.Context) {
	c.Log.Info("cleaning up legacy cli-server resources")

	cliServerResources := []client.Object{
		&apps.Deployment{ObjectMeta: metav1.ObjectMeta{Name: cliServerName, Namespace: cliServerNamespace}},
		&core.Service{ObjectMeta: metav1.ObjectMeta{Name: cliServerName, Namespace: cliServerNamespace}},
		&networking.Ingress{ObjectMeta: metav1.ObjectMeta{Name: cliServerName, Namespace: cliServerNamespace}},
	}

	for _, obj := range cliServerResources {
		if err := c.Client.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			c.Log.Error(err, "failed to get legacy cli-server resource", "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
			continue
		}

		if obj.GetLabels()[labels.LabelAppPartOf] != constants.AppName {
			c.Log.Info("skipping resource not owned by this operator", "name", obj.GetName())
			continue
		}

		if err := c.Client.Delete(ctx, obj); err != nil {
			c.Log.Error(err, "failed to delete legacy cli-server resource", "name", obj.GetName())
			continue
		}
		c.Log.Info("deleted legacy cli-server resource", "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
	}

}

func (c *MigrationComponent) NeedLeaderElection() bool {
	return true
}
