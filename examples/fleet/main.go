/*
Copyright 2024 The Kubernetes Authors.

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
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	kind "sigs.k8s.io/kind/pkg/cluster"
)

func init() {
	ctrl.SetLogger(klog.Background())
}

func main() {
	ctx := signals.SetupSignalHandler()
	entryLog := log.Log.WithName("entrypoint")

	testEnv := &envtest.Environment{}
	cfg, err := testEnv.Start()
	if err != nil {
		entryLog.Error(err, "failed to start local environment")
		os.Exit(1)
	}
	defer func() {
		if testEnv == nil {
			return
		}
		if err := testEnv.Stop(); err != nil {
			entryLog.Error(err, "failed to stop local environment")
			os.Exit(1)
		}
	}()

	// Setup a Manager, note that this not yet engages clusters, only makes them available.
	entryLog.Info("Setting up manager")
	provider := &KindClusterProvider{
		log:       log.Log.WithName("kind-cluster-provider"),
		clusters:  map[string]cluster.Cluster{},
		cancelFns: map[string]context.CancelFunc{},
	}
	mgr, err := manager.New(
		cfg,
		manager.Options{ExperimentalClusterProvider: provider},
	)
	if err != nil {
		entryLog.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	builder.ControllerManagedBy(mgr).
		For(&corev1.Pod{}).Complete(reconcile.Func(
		func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
			log := log.FromContext(ctx)

			cl, err := mgr.GetCluster(ctx, req.ClusterName)
			if err != nil {
				return reconcile.Result{}, err
			}
			client := cl.GetClient()

			// Retrieve the pod from the cluster.
			pod := &corev1.Pod{}
			if err := client.Get(ctx, req.NamespacedName, pod); err != nil {
				return reconcile.Result{}, err
			}
			log.Info("Reconciling pod", "ns", pod.GetNamespace(), "name", pod.Name, "uuid", pod.UID)

			// Print any annotations that start with fleet.
			for k, v := range pod.Labels {
				if strings.HasPrefix(k, "fleet-") {
					log.Info("Detected fleet annotation!", "key", k, "value", v)
				}
			}

			return ctrl.Result{}, nil
		},
	))

	entryLog.Info("Starting provider")
	go func() {
		if err := provider.Run(ctx, mgr); err != nil {
			entryLog.Error(err, "unable to run provider")
			os.Exit(1)
		}
	}()

	entryLog.Info("Starting manager")
	if err := mgr.Start(ctx); err != nil {
		entryLog.Error(err, "unable to run manager")
		os.Exit(1)
	}
}

// KindClusterProvider is a cluster provider that works with a local Kind instance.
type KindClusterProvider struct {
	Options   []cluster.Option
	log       logr.Logger
	lock      sync.RWMutex
	clusters  map[string]cluster.Cluster
	cancelFns map[string]context.CancelFunc
}

var _ cluster.Provider = &KindClusterProvider{}

func (k *KindClusterProvider) Get(ctx context.Context, clusterName string) (cluster.Cluster, error) {
	k.lock.RLock()
	defer k.lock.RUnlock()
	if cl, ok := k.clusters[clusterName]; ok {
		return cl, nil
	}

	return nil, fmt.Errorf("cluster %s not found", clusterName)
}

func (k *KindClusterProvider) Run(ctx context.Context, mgr manager.Manager) error {
	provider := kind.NewProvider()

	// initial list to smoke test
	if _, err := provider.List(); err != nil {
		return err
	}

	return wait.PollUntilContextCancel(ctx, time.Second*2, true, func(ctx context.Context) (done bool, err error) {
		list, err := provider.List()
		if err != nil {
			k.log.Info("failed to list kind clusters", "error", err)
			return false, nil // keep going
		}

		// start new clusters
		for _, clusterName := range list {
			log := k.log.WithValues("cluster", clusterName)

			// skip?
			if !strings.HasPrefix(clusterName, "fleet-") {
				continue
			}
			k.lock.RLock()
			if _, ok := k.clusters[clusterName]; ok {
				continue
			}
			k.lock.RUnlock()

			// create a new cluster
			kubeconfig, err := provider.KubeConfig(clusterName, false)
			if err != nil {
				k.log.Info("failed to get kind kubeconfig", "error", err)
				return false, nil // keep going
			}
			cfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
			if err != nil {
				k.log.Info("failed to create rest config", "error", err)
				return false, nil // keep going
			}
			clOptions := append([]cluster.Option{cluster.WithName(clusterName)}, k.Options...)
			cl, err := cluster.New(cfg, clOptions...)
			if err != nil {
				k.log.Info("failed to create cluster", "error", err)
				return false, nil // keep going
			}
			clusterCtx, cancel := context.WithCancel(ctx)
			go func() {
				if err := cl.Start(clusterCtx); err != nil {
					log.Error(err, "failed to start cluster")
					return
				}
			}()
			if !cl.GetCache().WaitForCacheSync(ctx) {
				cancel()
				log.Info("failed to sync cache")
				return false, nil
			}

			// remember
			k.lock.Lock()
			k.clusters[clusterName] = cl
			k.cancelFns[clusterName] = cancel
			k.lock.Unlock()

			// engage manager
			if mgr != nil {
				if err := mgr.Engage(clusterCtx, cl); err != nil {
					log.Error(err, "failed to engage manager")
					k.lock.Lock()
					delete(k.clusters, clusterName)
					delete(k.cancelFns, clusterName)
					k.lock.Unlock()
					return false, nil
				}
			}
		}

		// remove old clusters
		kindNames := sets.New(list...)
		k.lock.Lock()
		clusterNames := make([]string, 0, len(k.clusters))
		for name := range k.clusters {
			clusterNames = append(clusterNames, name)
		}
		k.lock.Unlock()
		for _, name := range clusterNames {
			if !kindNames.Has(name) {
				// disengage manager
				if mgr != nil {
					if err := mgr.Disengage(ctx, k.clusters[name]); err != nil {
						k.log.Error(err, "failed to disengage manager")
					}
				}

				// stop and forget
				k.lock.Lock()
				k.cancelFns[name]()
				delete(k.clusters, name)
				delete(k.cancelFns, name)
				k.lock.Unlock()
			}
		}

		return false, nil
	})
}
