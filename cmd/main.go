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
	"crypto/tls"
	"flag"
	"os"

	consolev1 "github.com/openshift/api/console/v1"
	v1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/securesign/operator/internal/clidownload"
	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/constants"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/ctlog"
	"github.com/securesign/operator/internal/controller/fulcio"
	"github.com/securesign/operator/internal/controller/rekor"
	"github.com/securesign/operator/internal/controller/securesign"
	"github.com/securesign/operator/internal/controller/trillian"
	"github.com/securesign/operator/internal/controller/tuf"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

//+kubebuilder:rbac:groups=console.openshift.io,resources=consoleclidownloads,verbs=create;get;list;watch;update;patch;delete
//+kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(monitoringv1.AddToScheme(scheme))
	utilruntime.Must(rhtasv1alpha1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	utilruntime.Must(v1.AddToScheme(scheme))
	utilruntime.Must(consolev1.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var (
		metricsAddr          string
		enableLeaderElection bool
		probeAddr            string
		pprofAddr            string
		secureMetrics        bool
		enableHTTP2          bool
	)

	flag.StringVar(&pprofAddr, "pprof-address", "", "The address to expose the pprof server. Default is empty string which disables the pprof server.")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", false,
		"If set the metrics endpoint is served securely")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.Int64Var(&constants.CreateTreeDeadline, "create-tree-deadline", constants.CreateTreeDeadline, "The time allowance (in seconds) for the create tree job to run before failing.")
	utils.StringFlagOrEnv(&constants.TrillianLogSignerImage, "trillian-log-signer-image", "TRILLIAN_LOG_SIGNER_IMAGE", constants.TrillianLogSignerImage, "The image used for trillian log signer.")
	utils.StringFlagOrEnv(&constants.TrillianServerImage, "trillian-log-server-image", "TRILLIAN_LOG_SERVER_IMAGE", constants.TrillianServerImage, "The image used for trillian log server.")
	utils.StringFlagOrEnv(&constants.TrillianDbImage, "trillian-db-image", "TRILLIAN_DB_IMAGE", constants.TrillianDbImage, "The image used for trillian's database.")
	utils.StringFlagOrEnv(&constants.TrillianNetcatImage, "trillian-netcat-image", "TRILLIAN_NETCAT_IMAGE", constants.TrillianNetcatImage, "The image used for trillian netcat.")
	utils.StringFlagOrEnv(&constants.FulcioServerImage, "fulcio-server-image", "FULCIO_SERVER_IMAGE", constants.FulcioServerImage, "The image used for the fulcio server.")
	utils.StringFlagOrEnv(&constants.RekorRedisImage, "rekor-redis-image", "REKOR_REDIS_IMAGE", constants.RekorRedisImage, "The image used for redis.")
	utils.StringFlagOrEnv(&constants.RekorServerImage, "rekor-server-image", "REKOR_SERVER_IMAGE", constants.RekorServerImage, "The image used for rekor server.")
	utils.StringFlagOrEnv(&constants.RekorSearchUiImage, "rekor-search-ui-image", "REKOR_SEARCH_UI_IMAGE", constants.RekorSearchUiImage, "The image used for rekor search ui.")
	utils.StringFlagOrEnv(&constants.BackfillRedisImage, "backfill-redis-image", "BACKFILL_REDIS_IMAGE", constants.BackfillRedisImage, "The image used for backfill redis.")
	utils.StringFlagOrEnv(&constants.TufImage, "tuf-image", "TUF_IMAGE", constants.TufImage, "The image used for TUF.")
	utils.StringFlagOrEnv(&constants.CTLogImage, "ctlog-image", "CTLOG_IMAGE", constants.CTLogImage, "The image used for ctlog.")
	utils.StringFlagOrEnv(&constants.ClientServerImage, "client-server-image", "CLIENT_SERVER_IMAGE", constants.ClientServerImage, "The image used to serve our cli binary's.")
	utils.StringFlagOrEnv(&constants.ClientServerImage_cg, "client-server-cg-image", "CLIENT_SERVER_CG_IMAGE", constants.ClientServerImage_cg, "The image used to serve cosign and gitsign.")
	utils.StringFlagOrEnv(&constants.ClientServerImage_re, "client-server-re-image", "CLIENT_SERVER_RE_IMAGE", constants.ClientServerImage_re, "The image used to serve rekor-cli and the ec binary.")
	utils.StringFlagOrEnv(&constants.SegmentBackupImage, "segment-backup-job-image", "SEGMENT_BACKUP_JOB_IMAGE", constants.SegmentBackupImage, "The image used for the segment backup job")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancelation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	tlsOpts := []func(*tls.Config){}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress:   metricsAddr,
			SecureServing: secureMetrics,
			TLSOpts:       tlsOpts,
		},
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		PprofBindAddress:       pprofAddr,
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

	if err = (&securesign.SecuresignReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Securesign")
		os.Exit(1)
	}
	if err = (&fulcio.FulcioReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("fulcio-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Fulcio")
		os.Exit(1)
	}
	if err = (&trillian.TrillianReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("trillian-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Trillian")
		os.Exit(1)
	}
	if err = (&rekor.RekorReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("rekor-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Rekor")
		os.Exit(1)
	}
	if err = (&tuf.TufReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("tuf-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Tuf")
		os.Exit(1)
	}
	if err = (&ctlog.CTlogReconciler{
		Client:   mgr.GetClient(),
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

	if err := mgr.Add(&clidownload.Component{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Log:    setupLog.WithName("clidownload"),
	}); err != nil {
		setupLog.Error(err, "unable to set up CLIDownload component")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
