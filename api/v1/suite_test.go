/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	testenvhelper "github.com/securesign/operator/internal/testing/envtest"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/test"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

func TestAPIs(t *testing.T) {
	fs := test.InitKlog(t)
	_ = fs.Set("v", "5")
	klog.SetOutput(GinkgoWriter)
	ctrl.SetLogger(klog.NewKlogr())

	RegisterFailHandler(Fail)
	RunSpecs(t, "v1 Suite")
}

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())

	By("bootstrapping test environment")

	Expect(SchemeBuilder.AddToScheme(scheme.Scheme)).To(Succeed())

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		BinaryAssetsDirectory: testenvhelper.FindBinaryAssetsDir(),
		Scheme:                scheme.Scheme,
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{filepath.Join("..", "..", "config", "webhook")},
		},
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	By("starting webhook server")

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    testEnv.WebhookInstallOptions.LocalServingHost,
			Port:    testEnv.WebhookInstallOptions.LocalServingPort,
			CertDir: testEnv.WebhookInstallOptions.LocalServingCertDir,
		}),
		Metrics: server.Options{BindAddress: "0"},
	})
	Expect(err).ToNot(HaveOccurred())

	Expect(SetupCTlogWebhookWithManager(mgr)).To(Succeed())
	Expect(SetupFulcioWebhookWithManager(mgr)).To(Succeed())
	Expect(SetupRekorWebhookWithManager(mgr)).To(Succeed())
	Expect(SetupSecuresignWebhookWithManager(mgr)).To(Succeed())
	Expect(SetupTimestampAuthorityWebhookWithManager(mgr)).To(Succeed())
	Expect(SetupTrillianWebhookWithManager(mgr)).To(Succeed())
	Expect(SetupTufWebhookWithManager(mgr)).To(Succeed())

	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(ctx)).To(Succeed())
	}()

	By("waiting for webhook server to be ready")
	dialer := &net.Dialer{Timeout: time.Second}
	addrPort := fmt.Sprintf("%s:%d",
		testEnv.WebhookInstallOptions.LocalServingHost,
		testEnv.WebhookInstallOptions.LocalServingPort)
	Eventually(func() error {
		conn, err := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true}) //nolint:gosec
		if err != nil {
			return err
		}
		return conn.Close()
	}).Should(Succeed())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	Expect(testEnv.Stop()).To(Succeed())
})
