package olm

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	olm "github.com/operator-framework/api/pkg/operators/v1"
	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// olm types
type subscriptionExtension struct {
	v1alpha1.Subscription
}

func (e *subscriptionExtension) Unwrap() client.Object {
	return &e.Subscription
}

func (e *subscriptionExtension) IsReady(ctx context.Context, cli client.Client) bool {
	s := e.getInstalledCSV(ctx, cli)
	return s != nil
}

func (e *subscriptionExtension) GetVersion(ctx context.Context, cli client.Client) string {
	s := e.getInstalledCSV(ctx, cli)
	if s == nil {
		return ""
	}
	return s.Spec.Version.String()

}

func (e *subscriptionExtension) getInstalledCSV(ctx context.Context, cli client.Client) *v1alpha1.ClusterServiceVersion {
	lst := v1alpha1.ClusterServiceVersionList{}
	if err := cli.List(ctx, &lst, client.InNamespace(e.Namespace)); err != nil {
		return nil
	}
	for _, s := range lst.Items {
		if strings.Contains(s.Name, e.Name) && s.Status.Phase == v1alpha1.CSVPhaseSucceeded {
			return &s

		}
	}
	return nil
}

type catalogSourceWrapper struct {
	v1alpha1.CatalogSource
}

func (c *catalogSourceWrapper) Unwrap() client.Object {
	return &c.CatalogSource
}

func (c *catalogSourceWrapper) IsReady(ctx context.Context, cli client.Client) bool {
	if c.Status.GRPCConnectionState == nil {
		return false
	}
	return c.Status.GRPCConnectionState.LastObservedState == "READY"
}

func (c *catalogSourceWrapper) UpdateSourceImage(s string) {
	c.Spec.Image = s
}

func OlmInstaller(ctx context.Context, cli client.Client, catalogImage, ns, packageName, channel string, ocp bool) (Extension, ExtensionSource, error) {
	scheme := cli.Scheme()
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	utilruntime.Must(olm.AddToScheme(scheme))

	og := &olm.OperatorGroup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      packageName,
		},
		Spec: olm.OperatorGroupSpec{
			TargetNamespaces: []string{},
		},
	}

	catalog := &catalogSourceWrapper{

		v1alpha1.CatalogSource{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      fmt.Sprintf("%s-catalog", packageName),
			},
			Spec: v1alpha1.CatalogSourceSpec{
				Image:       catalogImage,
				SourceType:  "grpc",
				DisplayName: "OLM upgrade test Catalog",
				Publisher:   "grpc",
			},
		},
	}

	subscription := &subscriptionExtension{
		v1alpha1.Subscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:      packageName,
				Namespace: ns,
			},
			Spec: &v1alpha1.SubscriptionSpec{
				CatalogSource:          catalog.Name,
				CatalogSourceNamespace: catalog.Namespace,
				Package:                packageName,
				Channel:                channel,
				Config: &v1alpha1.SubscriptionConfig{
					Env: []coreV1.EnvVar{
						{
							Name:  "OPENSHIFT",
							Value: strconv.FormatBool(ocp),
						},
					},
				},
			},
		},
	}

	for _, obj := range []client.Object{catalog.Unwrap(), subscription.Unwrap(), og} {
		if err := cli.Create(ctx, obj); client.IgnoreAlreadyExists(err) != nil {
			return nil, nil, err
		}
	}

	return subscription, catalog, nil
}
