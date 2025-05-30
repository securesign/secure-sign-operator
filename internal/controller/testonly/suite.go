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
	"fmt"
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
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
	t.ctx, t.cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	t.testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,

		// The BinaryAssetsDirectory is only required if you want to run the tests directly
		// without call the makefile target test. If not informed it will look for the
		// default path defined in controller-runtime which is /usr/local/kubebuilder/.
		// Note that you must have the required binaries setup under the bin directory to perform
		// the tests directly. When we run make test it will be setup and used automatically.
		BinaryAssetsDirectory: filepath.Join("..", "..", "..", "bin", "k8s",
			fmt.Sprintf("1.29.1-%s-%s", runtime.GOOS, runtime.GOARCH)),
	}

	var err error
	// cfg is defined in this file globally.
	t.cfg, err = t.testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(t.cfg).NotTo(BeNil())

	err = rhtasv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	t.k8sClient, err = client.New(t.cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(t.k8sClient).NotTo(BeNil())

	// start controller
	k8sManager, err := ctrl.NewManager(t.cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	Expect(err).NotTo(HaveOccurred())
	Expect(t.k8sClient).NotTo(BeNil())

	recorder := record.NewFakeRecorder(1000)

	err = t.supplier(
		k8sManager.GetClient(),
		k8sManager.GetScheme(),
		recorder,
	).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		elog := logf.FromContext(context.TODO()).WithName("Event")
		for msg := range recorder.Events {
			elog.Info(msg)
		}
	}()

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(t.ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
}

func (t *controllerSuite) AfterSuite() {
	t.cancel()
	By("tearing down the test environment")
	err := t.testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
}
