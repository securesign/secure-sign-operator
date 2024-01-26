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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	consolev1 "github.com/openshift/api/console/v1"
	v1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	client2 "github.com/securesign/operator/client"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/ctlog"
	"github.com/securesign/operator/controllers/fulcio"
	"github.com/securesign/operator/controllers/rekor"
	"github.com/securesign/operator/controllers/trillian"
	"github.com/securesign/operator/controllers/tuf"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	cliv1 "github.com/openshift/api/console/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers"
	//+kubebuilder:scaffold:imports
)

const (
	cliServerNs        = "trusted-artifact-signer"
	cliServerName      = "cli-server"
	cliServerComponent = "client-server"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(monitoringv1.AddToScheme(scheme))
	utilruntime.Must(rhtasv1alpha1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	utilruntime.Must(v1.AddToScheme(scheme))
	utilruntime.Must(cliv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		// MetricsBindAddress:     metricsAddr,
		// Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "f81d37df.redhat.com",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	client, err := client2.NewClientWithScheme(scheme)
	if err != nil {
		setupLog.Error(err, "unable to initialize k8s client")
		os.Exit(1)
	}
	if err = (&controllers.SecuresignReconciler{
		Client: client,
		Scheme: client.Scheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Securesign")
		os.Exit(1)
	}
	if err = (&fulcio.FulcioReconciler{
		Client:   client,
		Scheme:   client.Scheme(),
		Recorder: mgr.GetEventRecorderFor("fulcio-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Fulcio")
		os.Exit(1)
	}
	if err = (&trillian.TrillianReconciler{
		Client: client,
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Trillian")
		os.Exit(1)
	}
	if err = (&rekor.RekorReconciler{
		Client:   client,
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("rekor-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Rekor")
		os.Exit(1)
	}
	if err = (&tuf.TufReconciler{
		Client:   client,
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("tuf-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Tuf")
		os.Exit(1)
	}
	if err = (&ctlog.CTlogReconciler{
		Client:   client,
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("ctlog-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CTlog")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	obj, err := createClientserver(ctx, client)
	if err != nil {
		setupLog.Error(err, "unable to create client-server resources")
	}
	defer func() {
		for _, o := range obj {
			if err := client.Delete(ctx, o); err != nil {
				setupLog.Error(err, "unable to cleanup client-server resources")
			}
		}
		cancel()
	}()

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func createClientserver(ctx context.Context, cli client2.Client) ([]client.Object, error) {
	var (
		err    error
		obj    []client.Object
		labels = map[string]string{
			"app.kubernetes.io/part-of": constants.AppName,
			kubernetes.ComponentLabel:   cliServerComponent,
		}
	)
	ns := &core.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: cliServerNs,
		},
	}

	obj = append(obj, ns)
	obj = append(obj, createClientserverDeployment(ns.Name, labels))
	svc := kubernetes.CreateService(ns.Name, cliServerName, 8080, labels)
	obj = append(obj, svc)
	ingress, err := kubernetes.CreateIngress(ctx, cli, *svc, rhtasv1alpha1.ExternalAccess{}, cliServerName, labels)
	if err != nil {
		return obj, err
	}
	obj = append(obj, ingress)

	if kubernetes.IsOpenShift(cli) {
		protocol := "http://"
		if len(ingress.Spec.TLS) > 0 {
			protocol = "https://"
		}
		for name, description := range map[string]string{
			"cosign":    "cosign is a CLI tool that allows you to manage sigstore artifacts.",
			"rekor-cli": "rekor-cli is a CLI tool that allows you to interact with rekor server.",
			"gitsign":   "gitsign is a CLI tool that allows you to digitally sign and verify git commits.",
		} {
			obj = append(obj, createConsoleCLIDownload(ns.Name, name, protocol+ingress.Spec.Rules[0].Host, description, labels))
		}
	}

	for _, o := range obj {
		err = cli.Create(ctx, o)
		if err != nil {
			return obj, err
		}
	}

	return obj, nil
}

func createClientserverDeployment(namespace string, labels map[string]string) *apps.Deployment {
	replicas := int32(1)

	return &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cliServerName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: apps.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Name:            cliServerName,
							Image:           constants.ClientServerImage,
							ImagePullPolicy: "IfNotPresent",
							Ports: []core.ContainerPort{
								{
									ContainerPort: 8080,
									Protocol:      "TCP",
								},
							},
						},
					},
				},
			},
		},
	}
}

func createConsoleCLIDownload(namespace, name, clientServerUrl, description string, labels map[string]string) *consolev1.ConsoleCLIDownload {
	return &consolev1.ConsoleCLIDownload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: consolev1.ConsoleCLIDownloadSpec{
			Description: description,
			DisplayName: fmt.Sprintf("%s - Command Line Interface (CLI)", name),
			Links: []consolev1.CLIDownloadLink{
				{
					Href: fmt.Sprintf("%s/clients/linux/%s-amd64.gz", clientServerUrl, name),
					Text: fmt.Sprintf("Download %s for Linux x86_64", name),
				},
				{
					Href: fmt.Sprintf("%s/clients/linux/%s-arm64.gz", clientServerUrl, name),
					Text: fmt.Sprintf("Download %s for Linux arm64", name),
				},
				{
					Href: fmt.Sprintf("%s/clients/linux/%s-ppc64le.gz", clientServerUrl, name),
					Text: fmt.Sprintf("Download %s for Linux ppc64le", name),
				},
				{
					Href: fmt.Sprintf("%s/clients/linux/%s-s390x.gz", clientServerUrl, name),
					Text: fmt.Sprintf("Download %s for Linux s390x", name),
				},
				{
					Href: fmt.Sprintf("%s/clients/darwin/%s-amd64.gz", clientServerUrl, name),
					Text: fmt.Sprintf("Download %s for Mac x86_64", name),
				},
				{
					Href: fmt.Sprintf("%s/clients/darwin/%s-arm64.gz", clientServerUrl, name),
					Text: fmt.Sprintf("Download %s for Mac arm64", name),
				},
				{
					Href: fmt.Sprintf("%s/clients/windows/%s-amd64.gz", clientServerUrl, name),
					Text: fmt.Sprintf("Download %s for Windows x86_64", name),
				},
			},
		},
	}
}
