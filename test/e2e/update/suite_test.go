//go:build integration

package update

import (
	"context"
	"testing"
	"time"

	_ "github.com/securesign/operator/test/e2e/support/kubernetes"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestUpdateComponents(t *testing.T) {
	RegisterFailHandler(Fail)
	log.SetLogger(GinkgoLogr)
	SetDefaultEventuallyTimeout(time.Duration(1) * time.Minute)
	EnforceDefaultTimeoutsWhenUsingContexts()
	RunSpecs(t, "Update components Suite")

	// print whole stack in case of failure
	format.MaxLength = 0
}

func securesignResource(namespace *v1.Namespace) *rhtasv1alpha1.Securesign {
	return securesign.Create(namespace.Name, "test",
		securesign.WithDefaults(),
		securesign.WithoutSearchUI(),
	)
}

func getDeploymentGeneration(ctx context.Context, cli runtimeCli.Client, nn types.NamespacedName) int64 {
	deployment := appsv1.Deployment{}
	if err := cli.Get(ctx, nn, &deployment); err != nil {
		return -1
	}
	return deployment.Status.ObservedGeneration
}
