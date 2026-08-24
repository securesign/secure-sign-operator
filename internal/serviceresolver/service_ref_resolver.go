package serviceresolver

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

	v1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrGetServiceFailed    = fmt.Errorf("failed to get service")
	ErrAutodiscoveryFailed = fmt.Errorf("failed to autodiscovery service")
	ErrServiceNotReady     = fmt.Errorf("service is not ready")
)

// portRe matches a trailing :port, anchored to end-of-string so it can't match a
// port embedded in the resolver authority (always followed by "/" before the target).
var portRe = regexp.MustCompile(`:(\d+)$`)

func ResolveInternalServiceUrl(ctx context.Context, cl client.Client, serviceRef apis.ServiceReferencer, instanceNamespace string, instance client.Object) (string, error) {
	ref := serviceRef.GetServiceRef()
	users, done, err := parseUserURL(ref)
	if err != nil {
		return "", err
	}
	if done {
		return users.String(), nil
	}

	if err := serviceRefOrAutoload(ctx, cl, ref, instanceNamespace, instance); err != nil {
		return "", err
	}
	resolvedService, err := Resolve(instance)
	if err != nil {
		return "", err
	}
	return mergeURLs(users, resolvedService)
}

func ResolveExternalServiceUrl(ctx context.Context, cl client.Client, serviceRef apis.ServiceReferencer, instanceNamespace string, instance apis.AddressableConditionAware) (string, error) {
	ref := serviceRef.GetServiceRef()
	users, done, err := parseUserURL(ref)
	if err != nil {
		return "", err
	}
	if done {
		return users.String(), nil
	}

	if err := serviceRefOrAutoload(ctx, cl, ref, instanceNamespace, instance); err != nil {
		return "", err
	}
	if !meta.IsStatusConditionTrue(instance.GetConditions(), constants.ReadyCondition) {
		return "", fmt.Errorf("%w: %T %s", ErrServiceNotReady, instance, instance.GetName())
	}
	serviceURL := instance.GetServiceURL()
	if serviceURL == "" {
		return "", fmt.Errorf("%T %s: service url is empty", instance, instance.GetName())
	}
	return mergeURLs(users, serviceURL)
}

// parseUserURL extracts user overrides from a ServiceReference URL.
// Returns done=true when the URL already has a hostname (no autodiscovery needed).
func parseUserURL(ref v1.ServiceReference) (users *url.URL, done bool, err error) {
	users = &url.URL{}
	if ref.URL == "" {
		return users, false, nil
	}
	users, err = url.Parse(ref.URL)
	if err != nil {
		return nil, false, err
	}
	return users, users.Hostname() != "", nil
}

// mergeURLs combines a user's port/path overrides with an autodiscovered URL.
func mergeURLs(users *url.URL, resolvedRaw string) (string, error) {
	resolved, err := url.Parse(resolvedRaw)
	if err != nil {
		return "", err
	}
	users.Scheme = resolved.Scheme
	if users.Port() == "" {
		users.Host = resolved.Host
	} else {
		users.Host = net.JoinHostPort(resolved.Hostname(), users.Port())
	}
	userPath := strings.TrimLeft(users.Path, "/")
	if userPath == "" {
		users.Path = resolved.Path
	} else {
		users.Path = "/" + userPath
	}
	return users.String(), nil
}

// PopulateInstance resolves serviceRef (explicit ref, or autodiscovery-by-listing)
// and populates instance with the found object's data.
func PopulateInstance(ctx context.Context, cl client.Client, serviceRef apis.ServiceReferencer, instanceNamespace string, instance client.Object) error {
	return serviceRefOrAutoload(ctx, cl, serviceRef.GetServiceRef(), instanceNamespace, instance)
}

func ResolveInternalGrpcService(ctx context.Context, cl client.Client, serviceRef apis.ServiceReferencer, instanceNamespace string, instance client.Object) (address string, port string, err error) {
	ref := serviceRef.GetServiceRef()

	var userAddress, userPort string
	if ref.URL != "" {
		userAddress, userPort = splitGrpcAddressPort(ref.URL)
		if userAddress != "" && userAddress != "//" {
			address = userAddress
			port = userPort
			return
		}
	}

	if err = serviceRefOrAutoload(ctx, cl, ref, instanceNamespace, instance); err != nil {
		return
	}
	var resolved string
	resolved, err = Resolve(instance)
	if err != nil {
		return
	}
	address, port = splitGrpcAddressPort(resolved)

	if userPort != "" {
		port = userPort
	}
	return
}

func splitGrpcAddressPort(raw string) (address, port string) {
	matches := portRe.FindAllStringSubmatchIndex(raw, -1)
	if len(matches) == 0 {
		return raw, ""
	}
	m := matches[len(matches)-1]
	return raw[:m[0]], raw[m[2]:m[3]]
}

func serviceRefOrAutoload(ctx context.Context, cl client.Client, serviceRef v1.ServiceReference, instanceNamespace string, instance client.Object) error {
	if serviceRef.Ref != nil && serviceRef.Ref.Name != "" {
		if err := cl.Get(ctx, types.NamespacedName{Namespace: serviceRef.Ref.Namespace, Name: serviceRef.Ref.Name}, instance); err != nil {
			return fmt.Errorf("%w: %w", ErrGetServiceFailed, err)
		}
		return nil
	}

	// Autoload service from list of objects (backwards compatibility)
	listObject, err := objectAsList(cl, instance)
	if err != nil {
		return err
	}
	found, err := autoloadService(ctx, cl, instanceNamespace, listObject)
	if err != nil {
		return err
	}
	return cl.Scheme().Convert(found, instance, nil)
}

func autoloadService(ctx context.Context, cl client.Client, namespace string, list client.ObjectList) (client.Object, error) {
	if err := cl.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAutodiscoveryFailed, err)
	}
	items, err := meta.ExtractList(list)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAutodiscoveryFailed, err)
	}
	switch len(items) {
	case 0:
		return nil, fmt.Errorf("%w: no %T found in namespace %s", ErrAutodiscoveryFailed, list, namespace)
	case 1:
		obj, ok := items[0].(client.Object)
		if !ok {
			return nil, fmt.Errorf("%w: %T does not implement client.Object", ErrAutodiscoveryFailed, items[0])
		}
		return obj, nil
	default:
		return nil, fmt.Errorf("%w: found %d instances in namespace %s", ErrAutodiscoveryFailed, len(items), namespace)
	}
}

func objectAsList(cl client.Client, instance client.Object) (client.ObjectList, error) {
	gvks, _, err := cl.Scheme().ObjectKinds(instance)
	if err != nil {
		return nil, fmt.Errorf("resolving object kind: %w", err)
	}
	if len(gvks) == 0 {
		return nil, fmt.Errorf("no GVK registered for %T", instance)
	}
	listGVK := gvks[0]
	listGVK.Kind += "List"
	obj, err := cl.Scheme().New(listGVK)
	if err != nil {
		return nil, fmt.Errorf("creating list for %s: %w", listGVK.Kind, err)
	}
	list, ok := obj.(client.ObjectList)
	if !ok {
		return nil, fmt.Errorf("%s does not implement client.ObjectList", listGVK.Kind)
	}
	return list, nil
}
