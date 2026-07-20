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

package testonly

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller"
	testenvhelper "github.com/securesign/operator/internal/testing/envtest"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	//+kubebuilder:scaffold:imports
)

type controllerSuite struct {
	supplier controller.Constructor

	cfg       *rest.Config
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
	k8sClient client.Client // You'll be using this client in your tests.
}

func (s *controllerSuite) Client() client.Client {
	return s.k8sClient
}

func ControllerSuite(supplier controller.Constructor) controllerSuite {
	return controllerSuite{
		supplier: supplier,
	}
}

func (t *controllerSuite) BeforeSuite() {
	t.ctx, t.cancel = context.WithCancel(context.Background())

	By("bootstrapping test environment")
	t.testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: testenvhelper.FindBinaryAssetsDir(),
		Scheme:                scheme.Scheme,
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{filepath.Join("..", "..", "..", "config", "webhook")},
		},
	}

	var err error
	// cfg is defined in this file globally.
	t.cfg, err = t.testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(t.cfg).NotTo(BeNil())

	err = rhtasv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	t.k8sClient, err = client.New(t.cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(t.k8sClient).NotTo(BeNil())

	// start controller
	k8sManager, err := ctrl.NewManager(t.cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    t.testEnv.WebhookInstallOptions.LocalServingHost,
			Port:    t.testEnv.WebhookInstallOptions.LocalServingPort,
			CertDir: t.testEnv.WebhookInstallOptions.LocalServingCertDir,
		}),
		Metrics: metricsserver.Options{
			// turnoff metrics server
			BindAddress: "0",
		},
		HealthProbeBindAddress: "0",
	})
	Expect(err).ToNot(HaveOccurred())

	Expect(err).NotTo(HaveOccurred())
	Expect(t.k8sClient).NotTo(BeNil())

	Expect(rhtasv1.SetupCTlogWebhookWithManager(k8sManager)).To(Succeed())
	Expect(rhtasv1.SetupFulcioWebhookWithManager(k8sManager)).To(Succeed())
	Expect(rhtasv1.SetupRekorWebhookWithManager(k8sManager)).To(Succeed())
	Expect(rhtasv1.SetupSecuresignWebhookWithManager(k8sManager)).To(Succeed())
	Expect(rhtasv1.SetupTimestampAuthorityWebhookWithManager(k8sManager)).To(Succeed())
	Expect(rhtasv1.SetupTrillianWebhookWithManager(k8sManager)).To(Succeed())
	Expect(rhtasv1.SetupTufWebhookWithManager(k8sManager)).To(Succeed())

	recorder := events.NewFakeRecorder(1000)

	err = t.supplier(
		k8sManager.GetClient(),
		k8sManager.GetScheme(),
		recorder,
	).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		elog := logf.FromContext(context.Background()).WithName("Event")
		for msg := range recorder.Events {
			elog.Info(msg)
		}
	}()

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(t.ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	By("waiting for webhook server to be ready")
	dialer := &net.Dialer{Timeout: time.Second}
	addrPort := fmt.Sprintf("%s:%d",
		t.testEnv.WebhookInstallOptions.LocalServingHost,
		t.testEnv.WebhookInstallOptions.LocalServingPort)
	Eventually(func() error {
		conn, err := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true}) //nolint:gosec
		if err != nil {
			return err
		}
		return conn.Close()
	}).Should(Succeed())
}

func (t *controllerSuite) AfterSuite() {
	t.cancel()
	By("tearing down the test environment")
	err := t.testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
}
