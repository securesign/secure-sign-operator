package support

import (
	"context"

	olm "github.com/operator-framework/api/pkg/operators/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func CreateOrUpdateCatalogSource(ctx context.Context, cli client.Client, ns, name, image string) error {
	catalogSource := &olm.CatalogSource{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
	}

	_, err := ctrlutil.CreateOrUpdate(ctx, cli, catalogSource, func() error {
		catalogSource.Spec = olm.CatalogSourceSpec{
			Image:       image,
			SourceType:  "grpc",
			DisplayName: "OLM upgrade test Catalog",
			Publisher:   "grpc",
		}
		return nil
	})

	return err
}
