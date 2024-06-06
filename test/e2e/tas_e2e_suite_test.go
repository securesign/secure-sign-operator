package e2e

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	routev1 "github.com/openshift/api/route/v1"
	olm "github.com/operator-framework/api/pkg/operators/v1"
	olmAlpha "github.com/operator-framework/api/pkg/operators/v1alpha1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	log.SetLogger(GinkgoLogr)
	SetDefaultEventuallyTimeout(time.Duration(3) * time.Minute)
	RunSpecs(t, "Trusted Artifact Signer E2E Suite")

	// print whole stack in case of failure
	format.MaxLength = 0
}

func CreateClient() (runtimeCli.Client, error) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(monitoringv1.AddToScheme(scheme))
	utilruntime.Must(rhtasv1alpha1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	utilruntime.Must(olmAlpha.AddToScheme(scheme))
	utilruntime.Must(olm.AddToScheme(scheme))

	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	return runtimeCli.New(cfg, runtimeCli.Options{Scheme: scheme})

}
