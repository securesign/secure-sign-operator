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
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"
	ostls "github.com/openshift/controller-runtime-common/pkg/tls"
	appconfig "github.com/securesign/operator/internal/config"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/fips"
	"github.com/securesign/operator/internal/utils/kubernetes"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/utils/ptr"

	"sigs.k8s.io/controller-runtime/pkg/config"

	configv1 "github.com/openshift/api/config/v1"
	consolev1 "github.com/openshift/api/console/v1"
	v1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/securesign/operator/internal/clidownload"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	rhtasv1 "github.com/securesign/operator/api/v1"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/console"
	"github.com/securesign/operator/internal/controller/ctlog"
	"github.com/securesign/operator/internal/controller/fulcio"
	"github.com/securesign/operator/internal/controller/rekor"
	"github.com/securesign/operator/internal/controller/securesign"
	"github.com/securesign/operator/internal/controller/trillian"
	"github.com/securesign/operator/internal/controller/tsa"
	"github.com/securesign/operator/internal/controller/tuf"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(rhtasv1alpha1.AddToScheme(scheme))
	utilruntime.Must(rhtasv1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	utilruntime.Must(v1.AddToScheme(scheme))
	utilruntime.Must(configv1.AddToScheme(scheme))
	utilruntime.Must(consolev1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	// Create a cancellable context from the signal handler. The TLS profile watcher
	// uses cancel() to trigger a graceful restart when the cluster policy changes.
	ctx, cancel := context.WithCancel(ctrl.SetupSignalHandler())
	defer cancel()

	var (
		metricsAddr          string
		enableLeaderElection bool
		probeAddr            string
		pprofAddr            string
		secureMetrics        bool
		enableHTTP2          bool
		metricsCertDir       string
	)

	flag.StringVar(&pprofAddr, "pprof-address", "", "The address to expose the pprof server. Default is empty string which disables the pprof server.")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8443", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set the metrics endpoint is served securely via HTTPS with authentication and authorization")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.StringVar(&metricsCertDir, "metrics-tls-cert-dir", "",
		"Directory containing tls.crt and tls.key for metrics server TLS. If empty, a self-signed certificate is generated.")
	flag.Int64Var(&appconfig.CreateTreeDeadline, "create-tree-deadline", appconfig.CreateTreeDeadline, "The time allowance (in seconds) for the create tree job to run before failing.")
	utils.BoolFlagOrEnv(&appconfig.Openshift, "openshift", "OPENSHIFT", false, "Enable to ensures the operator applies OpenShift specific configurations.")
	utils.StringFlagOrEnv(&appconfig.OpenshiftAPIServerName, "openshift-apiserver-name", "OPENSHIFT_APISERVER_NAME", "openshift-apiserver", "The OpenShift API Server name.")
	utils.DurationFlagOrEnv(&appconfig.APIServerTimeout, "apiserver-timeout", "APISERVER_TIMEOUT", 30*time.Second, "The initial timeout for contacting the API Server, defaults to 30 seconds.")
	utils.BoolFlagOrEnv(&appconfig.DisableClusterTLSProfile, "disable-cluster-tls-profile", "DISABLE_CLUSTER_TLS_PROFILE", false,
		"Disable reading the cluster-wide TLS security profile from configv1.APIServer. "+
			"When set, the operator uses Intermediate TLS profile defaults (TLS 1.2 minimum). "+
			"Use this as an escape hatch if the cluster profile causes compatibility issues.")
	utils.StringFlagOrEnv(&appconfig.IngressHostTemplate, "ingress-host-template", "INGRESS_HOST_TEMPLATE", appconfig.IngressHostTemplate,
		"Default hostname template for non-OpenShift Ingress resources when ExternalAccess.Host is not set. "+
			"Uses Go fmt.Sprintf with %[1]s=service name, %[2]s=namespace. Ignored on OpenShift.")
	utils.RelatedImageFlag("trillian-log-signer-image", images.TrillianLogSigner, "The image used for trillian log signer.")
	utils.RelatedImageFlag("trillian-log-server-image", images.TrillianServer, "The image used for trillian log server.")
	utils.RelatedImageFlag("trillian-db-image", images.TrillianDb, "The image used for trillian's database.")
	utils.RelatedImageFlag("trillian-netcat-image", images.TrillianNetcat, "The image used for trillian netcat.")
	utils.RelatedImageFlag("createtree-image", images.TrillianCreateTree, "The image used to create a trillian tree.")
	utils.RelatedImageFlag("fulcio-server-image", images.FulcioServer, "The image used for the fulcio server.")
	utils.RelatedImageFlag("rekor-redis-image", images.RekorRedis, "The image used for redis.")
	utils.RelatedImageFlag("rekor-server-image", images.RekorServer, "The image used for rekor server.")
	utils.RelatedImageFlag("rekor-search-ui-image", images.RekorSearchUi, "The image used for rekor search ui.")
	utils.RelatedImageFlag("backfill-redis-image", images.BackfillRedis, "The image used for backfill redis.")
	utils.RelatedImageFlag("tuf-image", images.Tuf, "The image used for TUF.")
	utils.RelatedImageFlag("ctlog-image", images.CTLog, "The image used for ctlog.")
	utils.RelatedImageFlag("http-server-image", images.HttpServer, "The image used to serve our cli binary's.")
	utils.RelatedImageFlag("client-server-image", images.ClientServer, "The image used to serve cosign and gitsign.")
	utils.RelatedImageFlag("timestamp-authority-image", images.TimestampAuthority, "The image used for Timestamp Authority")
	utils.RelatedImageFlag("rekor-monitor-image", images.RekorMonitor, "The image used for rekor monitor.")
	utils.RelatedImageFlag("ctlog-monitor-image", images.CTLogMonitor, "The image used for ctlog monitor.")
	utils.RelatedImageFlag("console-api-image", images.ConsoleApi, "The image used for the console backend (API).")
	utils.RelatedImageFlag("console-ui-image", images.ConsoleUI, "The image used for the console UI.")
	flag.StringVar(&clidownload.CliHostName, "cli-server-hostname", "", "The hostname for the cli server")

	klog.InitFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(klog.NewKlogr())

	setupLog.Info("FIPS mode", "enabled", fips.Enabled())

	if !utils.IsFlagProvided("openshift", "OPENSHIFT") {
		openshift, err := kubernetes.DetectOpenShiftPlatform(setupLog, appconfig.OpenshiftAPIServerName, appconfig.APIServerTimeout)
		if err != nil {
			setupLog.Error(err, "Platform auto-detection failed, exiting")
			os.Exit(1)
		}
		appconfig.Openshift = openshift
		setupLog.Info("Platform auto-detected", "openshift", appconfig.Openshift)
	} else {
		setupLog.Info("Platform explicitly configured via flag/env", "openshift", appconfig.Openshift)
	}

	// Resolve the cluster TLS security profile once at startup, before the webhook and metrics
	// servers are configured. A dedicated bootstrap client is used because the manager has not
	// started yet. On vanilla Kubernetes (no configv1.APIServer) or when the flag is set,
	// this falls back to the Intermediate profile.
	var bootstrapClient client.Client
	if appconfig.Openshift && !appconfig.DisableClusterTLSProfile {
		var bootErr error
		bootstrapClient, bootErr = client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
		if bootErr != nil {
			setupLog.Error(bootErr, "unable to create bootstrap client for TLS profile resolution")
			os.Exit(1)
		}
	}

	// Bound the profile resolution by the same timeout used for OpenShift auto-detection and
	// derive it from the cancellable startup context, so a SIGTERM or a network partition during
	// a slow bootstrap API call aborts startup instead of hanging indefinitely.
	resolveCtx, resolveCancel := context.WithTimeout(ctx, appconfig.APIServerTimeout)
	tlsProfileSpec, tlsAdherence, resolveErr := resolveClusterTLSProfile(
		resolveCtx, bootstrapClient, appconfig.Openshift, appconfig.DisableClusterTLSProfile, setupLog)
	resolveCancel()
	if resolveErr != nil {
		setupLog.Error(resolveErr, "unable to resolve cluster TLS security profile")
		os.Exit(1)
	}

	tlsConfigFn, unsupportedCiphers := ostls.NewTLSConfigFromProfile(tlsProfileSpec)
	if len(unsupportedCiphers) > 0 {
		setupLog.Info("cluster TLS profile contains ciphers unsupported by Go TLS",
			"ciphers", unsupportedCiphers)
	}

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
	tlsOpts = append(tlsOpts, tlsConfigFn)
	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	// Restrict informer caches for high-cardinality core types to only objects managed by this operator.
	// Without this, each Owns() watch caches ALL objects of that type cluster-wide, causing OOM on
	// production clusters with hundreds of Deployments/Services from other workloads (SECURESIGN-4123).
	operatorLabelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{
			labels.LabelAppPartOf: constants.AppName,
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to create operator label selector for cache")
		os.Exit(1)
	}
	operatorCacheSelector := cache.ByObject{
		Label: operatorLabelSelector,
	}

	cacheOpts := cache.Options{
		ByObject: map[client.Object]cache.ByObject{
			&appsv1.Deployment{}:    operatorCacheSelector,
			&appsv1.ReplicaSet{}:    operatorCacheSelector,
			&appsv1.StatefulSet{}:   operatorCacheSelector,
			&corev1.Pod{}:           operatorCacheSelector,
			&corev1.Service{}:       operatorCacheSelector,
			&networkingv1.Ingress{}: operatorCacheSelector,
			&batchv1.CronJob{}:      operatorCacheSelector,
			&batchv1.Job{}:          operatorCacheSelector,
		},
	}

	if kubernetes.IsOpenShift() {
		// Configure the manager's cache.
		// We must explicitly configure the cache for config.openshift.io/ingresses to watch only the "cluster" resource.
		// This is because the operator's ClusterRole has permissions restricted to that specific resource name, and a full
		// cluster-wide list is forbidden.
		cacheOpts.ByObject[&configv1.Ingress{}] = cache.ByObject{
			Field: fields.SelectorFromSet(fields.Set{
				"metadata.name": "cluster",
			}),
		}
		if !appconfig.DisableClusterTLSProfile {
			// Restrict the APIServer cache to the single "cluster" object.
			// The ClusterRole only grants access to this named resource, so a
			// full cluster-wide list would be forbidden.
			cacheOpts.ByObject[&configv1.APIServer{}] = cache.ByObject{
				Field: fields.SelectorFromSet(fields.Set{
					"metadata.name": "cluster",
				}),
			}
		}
	}

	metricsOpts := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}
	if secureMetrics {
		metricsOpts.FilterProvider = filters.WithAuthenticationAndAuthorization
	}
	if metricsCertDir != "" {
		metricsOpts.CertDir = metricsCertDir
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsOpts,
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
		Controller: config.Controller{
			RecoverPanic: ptr.To(true),
		},
		Cache: cacheOpts,
		// Secret/ConfigMap are excluded from cacheOpts.ByObject: a label/field selector on
		// these types would return a permanent NotFound for any object outside the selector
		// (no live fallback), breaking user-provided (BYO) secret/configmap refs. DisableFor
		// keeps reads live instead, avoiding an unfiltered, cluster-wide cache.
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{
					&corev1.Secret{},
					&corev1.ConfigMap{},
				},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Watch the cluster TLS security profile for changes. When the profile or adherence
	// policy changes, cancel() triggers a graceful shutdown so the operator restarts
	// and picks up the new configuration.
	if appconfig.Openshift && !appconfig.DisableClusterTLSProfile {
		if err := (&ostls.SecurityProfileWatcher{
			Client:                    mgr.GetClient(),
			InitialTLSProfileSpec:     tlsProfileSpec,
			InitialTLSAdherencePolicy: tlsAdherence,
			OnProfileChange: func(_ context.Context, old, new configv1.TLSProfileSpec) {
				setupLog.Info("cluster TLS profile changed; restarting to apply new configuration",
					"old", old, "new", new)
				cancel()
			},
			OnAdherencePolicyChange: func(_ context.Context, old, new configv1.TLSAdherencePolicy) {
				setupLog.Info("cluster TLS adherence policy changed; restarting to apply new configuration",
					"old", old, "new", new)
				cancel()
			},
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to set up TLS security profile watcher")
			os.Exit(1)
		}
	}

	setupController("securesign", securesign.NewReconciler, mgr)
	setupController("fulcio", fulcio.NewReconciler, mgr)
	setupController("trillian", trillian.NewReconciler, mgr)
	setupController("rekor", rekor.NewReconciler, mgr)
	setupController("tuf", tuf.NewReconciler, mgr)
	setupController("ctlog", ctlog.NewReconciler, mgr)
	setupController("tsa", tsa.NewReconciler, mgr)
	setupController("console", console.NewReconciler, mgr)
	//+kubebuilder:scaffold:builder

	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		// The /convert conversion webhook is auto-registered by controller-runtime's
		// WebhookBuilder when it detects Hub/Spoke interfaces on the v1 types.
		// See: https://book.kubebuilder.io/multiversion-tutorial/conversion
		//      https://book.kubebuilder.io/multiversion-tutorial/webhooks
		setupWebhook("Securesign", rhtasv1.SetupSecuresignWebhookWithManager, mgr)
		setupWebhook("Fulcio", rhtasv1.SetupFulcioWebhookWithManager, mgr)
		setupWebhook("Trillian", rhtasv1.SetupTrillianWebhookWithManager, mgr)
		setupWebhook("Rekor", rhtasv1.SetupRekorWebhookWithManager, mgr)
		setupWebhook("Tuf", rhtasv1.SetupTufWebhookWithManager, mgr)
		setupWebhook("CTlog", rhtasv1.SetupCTlogWebhookWithManager, mgr)
		setupWebhook("TimestampAuthority", rhtasv1.SetupTimestampAuthorityWebhookWithManager, mgr)
		setupWebhook("Console", rhtasv1.SetupConsoleWebhookWithManager, mgr)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err := mgr.AddReadyzCheck("webhook", webhookServer.StartedChecker()); err != nil {
			setupLog.Error(err, "unable to set up webhook ready check")
			os.Exit(1)
		}
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
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// resolveClusterTLSProfile determines the cluster TLS security profile and adherence policy to
// apply to the operator's webhook and metrics servers.
//
// On vanilla Kubernetes or when disabled is set, it returns the Intermediate profile defaults
// (TLS 1.2 minimum) without contacting the API server. On OpenShift it fetches the cluster-wide
// profile from the config.openshift.io APIServer, falling back to Intermediate defaults when the
// resource or API is unavailable. A non-nil error is returned only for unexpected failures that
// should abort startup.
func resolveClusterTLSProfile(ctx context.Context, cli client.Client, openshift, disabled bool, log logr.Logger) (configv1.TLSProfileSpec, configv1.TLSAdherencePolicy, error) {
	intermediateSpec := *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]

	if !openshift || disabled {
		if !openshift {
			log.Info("not running on OpenShift; using Intermediate TLS defaults")
		} else {
			log.Info("cluster TLS profile resolution disabled via flag; using Intermediate defaults")
		}
		return intermediateSpec, configv1.TLSAdherencePolicyNoOpinion, nil
	}

	tlsProfileSpec, err := ostls.FetchAPIServerTLSProfile(ctx, cli)
	if err != nil {
		if apiErrors.IsNotFound(err) || apimeta.IsNoMatchError(err) {
			log.Info("config.openshift.io APIServer not available; using Intermediate TLS defaults")
			tlsProfileSpec = intermediateSpec
		} else {
			return configv1.TLSProfileSpec{}, "", fmt.Errorf("unable to fetch cluster TLS security profile: %w", err)
		}
	}

	tlsAdherence, err := ostls.FetchAPIServerTLSAdherencePolicy(ctx, cli)
	if err != nil {
		if apiErrors.IsNotFound(err) || apimeta.IsNoMatchError(err) {
			log.Info("TLSAdherencePolicy API not available; defaulting to NoOpinion")
		} else {
			log.Error(err, "unable to fetch cluster TLS adherence policy; defaulting to NoOpinion")
		}
		tlsAdherence = configv1.TLSAdherencePolicyNoOpinion
	}

	log.Info("cluster TLS security profile resolved")
	return tlsProfileSpec, tlsAdherence, nil
}

func setupController(name string, constructor controller.Constructor, manager ctrl.Manager) {
	if err := constructor(
		manager.GetClient(),
		manager.GetScheme(),
		manager.GetEventRecorder(name+"-controller"),
	).SetupWithManager(manager); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", name)
		os.Exit(1)
	}
}

func setupWebhook(name string, setup func(ctrl.Manager) error, manager ctrl.Manager) {
	if err := setup(manager); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", name)
		os.Exit(1)
	}
}
