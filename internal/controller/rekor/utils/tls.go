package utils

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Mock used in tests
var MockUseTrillianTLS func(ctx context.Context, serviceAddr string, tlsCACertFile string) (bool, error)

// checks if trillian-logserver service supports TLS
func UseTrillianTLS(ctx context.Context, serviceAddr string, tlsCACertFile string) (bool, error) {

	if MockUseTrillianTLS != nil {
		return MockUseTrillianTLS(ctx, serviceAddr, "")
	}

	if kubernetes.IsOpenShift() {
		return true, nil
	}

	timeout := 5 * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	hostname := serviceAddr
	if idx := strings.Index(serviceAddr, ":"); idx != -1 {
		hostname = serviceAddr[:idx]
	}

	var creds credentials.TransportCredentials
	if tlsCACertFile != "" {
		tlsCaCert, err := os.ReadFile(filepath.Clean(tlsCACertFile))
		if err != nil {
			return false, fmt.Errorf("failed to load tls ca cert: %v", err)
		}
		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(tlsCaCert) {
			return false, fmt.Errorf("failed to append CA certificate to pool")
		}
		creds = credentials.NewTLS(&tls.Config{
			ServerName: hostname,
			RootCAs:    certPool,
			MinVersion: tls.VersionTLS12,
		})
	}

	conn, err := grpc.DialContext(ctx, serviceAddr, grpc.WithTransportCredentials(creds), grpc.WithBlock())
	if err != nil {
		fmt.Printf("gRPC service at %s is not TLS secured: %v\n", serviceAddr, err)
		return false, nil
	}
	if err := conn.Close(); err != nil {
		return false, fmt.Errorf("failed to close connection: %v", err)
	}

	return true, nil
}

func CAPath(ctx context.Context, cli client.Client, instance *rhtasv1alpha1.Rekor) (string, error) {
	if instance.Spec.TrustedCA != nil {
		cfgTrust, err := kubernetes.GetConfigMap(ctx, cli, instance.Namespace, instance.Spec.TrustedCA.Name)
		if err != nil {
			return "", err
		}
		if len(cfgTrust.Data) != 1 {
			err = fmt.Errorf("%s ConfigMap can contain only 1 record", instance.Spec.TrustedCA.Name)
			return "", err
		}
		for key := range cfgTrust.Data {
			return "/var/run/configs/tas/ca-trust/" + key, nil
		}
	}

	if instance.Spec.TrustedCA == nil && kubernetes.IsOpenShift() {
		return "/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt", nil
	}

	return "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", nil
}
