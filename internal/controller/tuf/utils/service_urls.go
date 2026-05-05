package utils

import (
	"context"

	"fmt"

	v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/apis"
	ctlog "github.com/securesign/operator/internal/controller/ctlog/actions"
	fulcio "github.com/securesign/operator/internal/controller/fulcio/actions"
	rekor "github.com/securesign/operator/internal/controller/rekor/actions"
	tsa "github.com/securesign/operator/internal/controller/tsa/actions"
)

type serviceEndpoint struct {
	Service             apis.TasService
	ServiceName, Suffix string
	Port                *int32
}

func ResolveServiceAddress(ctx context.Context, c client.Client, instance *rhtasv1alpha1.Tuf) error {
	var keyToService = map[string]serviceEndpoint{
		rekorKey:  {Service: &instance.Spec.Rekor, ServiceName: rekor.ServerDeploymentName, Suffix: ""},
		ctfeKey:   {Service: &instance.Spec.Ctlog, ServiceName: ctlog.DeploymentName, Suffix: ""},
		fulcioKey: {Service: &instance.Spec.Fulcio, ServiceName: fulcio.DeploymentName, Suffix: ""},
		tsaKey:    {Service: &instance.Spec.Tsa, ServiceName: tsa.DeploymentName, Suffix: tsa.TimestampPath, Port: ptr.To(int32(3000))},
	}
	useTlsClient := false // TODO: https://redhat.atlassian.net/browse/SECURESIGN-994: Use TLS client for internal mode
	// useTlsClient := tls.UseTlsClient(instance)

	for _, key := range instance.Spec.Keys {
		signingConfigURLMode := instance.Spec.SigningConfigURLMode
		service, ok := keyToService[key.Name]
		if !ok {
			return fmt.Errorf("unknown key %s", key.Name)
		}
		if key.Name == ctfeKey {
			// ctlog is never exposed externally, so we always use internal mode
			signingConfigURLMode = rhtasv1alpha1.SigningConfigURLInternal
		}
		if err := resolveServiceAddress(ctx, c, service, types.NamespacedName{Name: service.ServiceName, Namespace: instance.Namespace}, signingConfigURLMode, useTlsClient); err != nil {
			return err
		}
	}

	return nil
}

func resolveServiceAddress(ctx context.Context, c client.Client, serviceEndpoint serviceEndpoint, namespacedName types.NamespacedName, signingConfigURLMode rhtasv1alpha1.TufSigningConfigURLMode, useTlsClient bool) error {
	var (
		protocol string
	)
	switch {
	case serviceEndpoint.Service.GetAddress() != "":
		return nil // user config bypass the signingConfigURLMode and prefix
	case signingConfigURLMode == rhtasv1alpha1.SigningConfigURLInternal:

		if useTlsClient {
			protocol = "https"
		} else {
			protocol = "http"
		}

		serviceEndpoint.Service.SetAddress(fmt.Sprintf("%s://%s.%s.svc", protocol, namespacedName.Name, namespacedName.Namespace))
		if serviceEndpoint.Port != nil {
			serviceEndpoint.Service.SetPort(serviceEndpoint.Port)
		}
	default: // external mode
		if url, err := resolveURLFromIngress(ctx, c, namespacedName.Name, namespacedName.Namespace); err != nil {
			return err
		} else {
			serviceEndpoint.Service.SetAddress(url)
		}
	}
	if serviceEndpoint.Suffix != "" {
		serviceEndpoint.Service.SetAddress(serviceEndpoint.Service.GetAddress() + serviceEndpoint.Suffix)
	}
	return nil
}

func resolveURLFromIngress(ctx context.Context, c client.Client, ingressName, namespace string) (string, error) {
	ingress := &v1.Ingress{}
	if err := c.Get(ctx, types.NamespacedName{Name: ingressName, Namespace: namespace}, ingress); err != nil {
		return "", err
	}
	if len(ingress.Spec.Rules) == 0 || ingress.Spec.Rules[0].Host == "" {
		return "", fmt.Errorf("fail to resolve host name from ingress %s", ingress.Name)
	}
	protocol := "http"
	if len(ingress.Spec.TLS) > 0 {
		protocol = "https"
	}
	return fmt.Sprintf("%s://%s", protocol, ingress.Spec.Rules[0].Host), nil
}

func ResolveOIDCIssuers(ctx context.Context, c client.Client, namespace string) []string {
	fulcioList := &rhtasv1alpha1.FulcioList{}
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
