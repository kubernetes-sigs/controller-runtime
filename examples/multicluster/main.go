/*
Copyright 2020 The Kubernetes Authors.

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
	"fmt"
	"os"

	"k8s.io/client-go/rest"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/examples/multicluster/reconciler"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/clusterconnector"
)

func main() {
	log := ctrl.Log.WithName("pod-mirror-controller")

	referenceClusterCfg, err := config.GetConfigWithContext("reference-cluster")
	if err != nil {
		log.Error(err, "failed to get reference cluster config")
		os.Exit(1)
	}

	mirrorClusterCfg, err := config.GetConfigWithContext("mirror-cluster")
	if err != nil {
		log.Error(err, "failed to get mirror cluster config")
		os.Exit(1)
	}
	ctrl.SetupSignalHandler()

	if err := run(referenceClusterCfg, mirrorClusterCfg, ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "failed to run controller")
		os.Exit(1)
	}

	log.Info("Finished gracefully")
}

func run(referenceClusterConfig, mirrorClusterConfig *rest.Config, stop <-chan struct{}) error {
	mgr, err := ctrl.NewManager(referenceClusterConfig, ctrl.Options{})
	if err != nil {
		return fmt.Errorf("failed to construct manager: %w", err)
	}
	clusterConnector, err := clusterconnector.New(mirrorClusterConfig, mgr, "mirror_cluster")
	if err != nil {
		return fmt.Errorf("failed to construct clusterconnector: %w", err)
	}

	if err := reconciler.Add(mgr, clusterConnector); err != nil {
		return fmt.Errorf("failed to construct reconciler: %w", err)
	}

	if err := mgr.Start(stop); err != nil {
		return fmt.Errorf("failed to start manager: %w", err)
	}

	return nil
}
