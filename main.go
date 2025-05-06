package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	cruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	cruntimescheme "sigs.k8s.io/controller-runtime/pkg/scheme"

	"notready/controllers/notready"

	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = cruntime.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	// Add other schemes if necessary
}

func main() {
	var metricsAddr string
	var probeAddr string
	var unreachableTimeout time.Duration

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.DurationVar(&unreachableTimeout, "unreachable-timeout", 10*time.Minute, "The duration after which an unreachable node is deleted")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	cruntime.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		fmt.Println("Unable to add client-go scheme", err)
		os.Exit(1)
	}

	karpenterSchemeBuilder := &cruntimescheme.Builder{
		GroupVersion: schema.GroupVersion{
			Group:   "karpenter.sh",
			Version: "v1",
		},
	}
	karpenterSchemeBuilder.Register(&karpenterv1.NodeClaim{}, &karpenterv1.NodeClaimList{})
	if err := karpenterSchemeBuilder.AddToScheme(scheme); err != nil {
		fmt.Println("Unable to add Karpenter types to scheme", err)
		os.Exit(1)
	}

	// Create a new manager
	mgr, err := cruntime.NewManager(cruntime.GetConfigOrDie(), cruntime.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         false,
	})

	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Register the controller
	if err = notready.NewController(mgr.GetClient(), unreachableTimeout).Register(context.Background(), mgr); err != nil {
		setupLog.Error(err, "unable to register controller with manager")
		os.Exit(1)
	}

	// Setup health checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	setupLog.Info("Configured timeout: ", "unreachableTimeout", unreachableTimeout)
	if err := mgr.Start(cruntime.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
