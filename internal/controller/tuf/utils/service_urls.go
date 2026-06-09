package utils

import (
	"context"

	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	tsa "github.com/securesign/operator/internal/controller/tsa/actions"
	"k8s.io/apimachinery/pkg/api/meta"
)

type AddressableConditionAware interface {
	apis.Addressable
	apis.ConditionsAwareObject
}

type serviceEndpoint struct {
	Service       apis.TasService
	Suffix        string
	ComponentList client.ObjectList
}

func ResolveServiceAddress(ctx context.Context, c client.Client, instance *rhtasv1.Tuf) error {
	var keyToService = map[string]serviceEndpoint{
		rekorKey:  {Service: &instance.Spec.Rekor, ComponentList: &rhtasv1.RekorList{}, Suffix: ""},
		ctfeKey:   {Service: &instance.Spec.Ctlog, ComponentList: &rhtasv1.CTlogList{}, Suffix: ""},
		fulcioKey: {Service: &instance.Spec.Fulcio, ComponentList: &rhtasv1.FulcioList{}, Suffix: ""},
		tsaKey:    {Service: &instance.Spec.Tsa, ComponentList: &rhtasv1.TimestampAuthorityList{}, Suffix: tsa.TimestampPath},
	}

	for _, key := range instance.Spec.Keys {
		serviceEndpoint, ok := keyToService[key.Name]
		if !ok {
			return fmt.Errorf("unknown key %s", key.Name)
		}
		switch {
		case serviceEndpoint.Service.GetAddress() != "":
			continue // user specified address

		default:
			if url, err := resolveURLFromService(ctx, c, serviceEndpoint.ComponentList, instance.Namespace); err != nil {
				return err
			} else {
				serviceEndpoint.Service.SetAddress(url)
			}
			if serviceEndpoint.Suffix != "" {
				serviceEndpoint.Service.SetAddress(serviceEndpoint.Service.GetAddress() + serviceEndpoint.Suffix)
			}
		}
	}

	return nil
}

func resolveURLFromService(ctx context.Context, c client.Client, list client.ObjectList, namespace string) (string, error) {
	list = list.DeepCopyObject().(client.ObjectList)
	if err := c.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return "", err
	}

	l, err := meta.ExtractList(list)
	if err != nil {
		return "", err
	}
	switch {
	case len(l) == 0:
		return "", fmt.Errorf("no items found in %T", list)
	case len(l) > 1:
		return "", fmt.Errorf("multiple items found in %T", list)
	default:
		instance, ok := l[0].(AddressableConditionAware)
		if !ok {
			return "", fmt.Errorf("service %T is not addressable or not a condition aware object", l[0])
		}
		if !meta.IsStatusConditionTrue(instance.GetConditions(), constants.ReadyCondition) {
			return "", fmt.Errorf("service is not ready Kind: %T, Name: %s", instance, instance.GetName())
		}
		url := instance.GetServiceURL()
		if url == "" {
			return "", fmt.Errorf("service %T url is empty", instance)
		}
		return url, nil
	}
}

func ResolveOIDCIssuers(ctx context.Context, c client.Client, namespace string) []string {
	fulcioList := &rhtasv1.FulcioList{}
	if err := c.List(ctx, fulcioList, client.InNamespace(namespace)); err != nil {
		return nil
	}
	if len(fulcioList.Items) == 0 {
		return nil
	}

	fulcioInstance := &fulcioList.Items[0]
	var issuers []string
	for _, oidc := range fulcioInstance.Spec.Config.OIDCIssuers {
		if oidc.IssuerURL != "" {
			issuers = append(issuers, oidc.IssuerURL)
		} else if oidc.Issuer != "" {
			issuers = append(issuers, oidc.Issuer)
		}
	}
	return issuers
}
