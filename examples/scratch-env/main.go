package main

import (
	goflag "flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	flag "github.com/spf13/pflag"

	"k8s.io/client-go/tools/clientcmd"
	kcapi "k8s.io/client-go/tools/clientcmd/api"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	crdPaths              = flag.StringSlice("crd-paths", nil, "paths to files or directories containing CRDs to install on start")
	webhookPaths          = flag.StringSlice("webhook-paths", nil, "paths to files or directories containing webhook configurations to install on start")
	attachControlPlaneOut = flag.Bool("debug-env", false, "attach to test env (apiserver & etcd) output -- just a convinience flag to force KUBEBUILDER_ATTACH_CONTROL_PLANE_OUTPUT=true")
)

func writeKubeConfig(kubeConfig *kcapi.Config, kubeconfigFile *os.File) error {
	defer kubeconfigFile.Close()

	contents, err := clientcmd.Write(*kubeConfig)
	if err != nil {
		return fmt.Errorf("unable to serialize kubeconfig file: %w", err)
	}

	amt, err := kubeconfigFile.Write(contents)
	if err != nil {
		return fmt.Errorf("unable to write kubeconfig file: %w", err)
	}
	if amt != len(contents) {
		fmt.Errorf("unable to write all of the kubeconfig file: %w", io.ErrShortWrite)
	}

	return nil
}

// have a separate function so we can return an exit code w/o skipping defers
func runMain() int {
	loggerOpts := &zap.Options{
		Development: true, // a sane default
	}
	{
		var goFlagSet goflag.FlagSet
		loggerOpts.BindFlags(&goFlagSet)
		flag.CommandLine.AddGoFlagSet(&goFlagSet)
	}
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(loggerOpts)))

	log := ctrl.Log.WithName("main")

	env := &envtest.Environment{}
	env.CRDInstallOptions.Paths = *crdPaths
	env.WebhookInstallOptions.Paths = *webhookPaths

	if *attachControlPlaneOut {
		os.Setenv("KUBEBUILDER_ATTACH_CONTROL_PLANE_OUTPUT", "true")
	}

	log.Info("Starting apiserver & etcd")
	cfg, err := env.Start()
	if err != nil {
		log.Error(err, "unable to start the test environment")
		return 1
	}

	log.Info("apiserver running", "host", cfg.Host)

	// TODO(directxman12): add support for writing to a new context in an existing file
	kubeconfigFile, err := ioutil.TempFile("", "scratch-env-kubeconfig-")
	if err != nil {
		log.Error(err, "unable to create kubeconfig file, continuing on without it")
	} else {
		defer os.Remove(kubeconfigFile.Name())

		log := log.WithValues("path", kubeconfigFile.Name())
		log.V(1).Info("Writing kubeconfig")

		// TODO(directxman12): this config isn't quite fully specified, but I
		// think it's the best we can do for now -- I don't see any obvious
		// "rest.Config --> clientcmdapi.Config" helper
		kubeConfig := kcapi.NewConfig()
		kubeConfig.Clusters["scratch-env"] = &kcapi.Cluster{
			Server: fmt.Sprintf("http://%s", cfg.Host),
		}
		kcCtx := kcapi.NewContext()
		kcCtx.Cluster = "scratch-env"
		kubeConfig.Contexts["scratch-env"] = kcCtx
		kubeConfig.CurrentContext = "scratch-env"

		if err := writeKubeConfig(kubeConfig, kubeconfigFile); err != nil {
			log.Error(err, "unable to save kubeconfig")
			return 1
		}

		log.Info("Wrote kubeconfig")
	}

	ctx := ctrl.SetupSignalHandler()
	<-ctx.Done()

	log.Info("Shutting down apiserver & etcd")
	err = env.Stop()
	if err != nil {
		log.Error(err, "unable to stop the test environment")
		return 1
	}

	log.Info("Shutdown succesful")
	return 0
}

func main() {
	os.Exit(runMain())
}
