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
	"errors"
	"fmt"
	"os"
	"sync"

	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	cache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func init() {
	ctrl.SetLogger(klog.Background())
}

func main() {
	entryLog := log.Log.WithName("entrypoint")
	ctx := signals.SetupSignalHandler()

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

	// Test fixtures
	cli, err := client.New(cfg, client.Options{})
	if err != nil {
		entryLog.Error(err, "failed to create client")
		os.Exit(1)
	}
	runtime.Must(cli.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "zoo"}}))
	runtime.Must(cli.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "zoo", Name: "elephant"}}))
	runtime.Must(cli.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "zoo", Name: "lion"}}))
	runtime.Must(cli.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "jungle"}}))
	runtime.Must(cli.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "jungle", Name: "monkey"}}))
	runtime.Must(cli.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "island"}}))
	runtime.Must(cli.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "island", Name: "bird"}}))

	entryLog.Info("Setting up provider")
	cl, err := cluster.New(cfg, func(options *cluster.Options) {
		options.Cache.AdditionalDefaultIndexes = map[string]client.IndexerFunc{
			ClusterNameIndex: func(obj client.Object) []string {
				return []string{
					fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName()),
					fmt.Sprintf("%s/%s", "*", obj.GetName()),
				}
			},
			ClusterIndex: func(obj client.Object) []string {
				return []string{obj.GetNamespace()}
			},
		}
	})
	if err != nil {
		entryLog.Error(err, "unable to set up provider")
		os.Exit(1)
	}
	provider := NewNamespacedClusterProvider(cl)

	// Setup a cluster-aware Manager, with the provider to lookup clusters.
	entryLog.Info("Setting up cluster-aware manager")
	mgr, err := manager.New(cfg, manager.Options{
		NewCache: func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
			// wrap cache to turn IndexField calls into cluster-scoped indexes.
			return &NamespaceScopeableCache{Cache: cl.GetCache()}, nil
		},
		ExperimentalClusterProvider: provider,
	})
	if err != nil {
		entryLog.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	if err := builder.ControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).Complete(reconcile.Func(
		func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
			log := log.FromContext(ctx)

			cl, err := mgr.GetCluster(ctx, req.ClusterName)
			if err != nil {
				return reconcile.Result{}, err
			}
			cli := cl.GetClient()

			// Retrieve the service account from the namespace.
			cm := &corev1.ConfigMap{}
			if err := cli.Get(ctx, req.NamespacedName, cm); err != nil {
				return reconcile.Result{}, err
			}
			log.Info("Reconciling configmap", "cluster", req.ClusterName, "ns", req.Namespace, "name", cm.Name, "uuid", cm.UID)

			return ctrl.Result{}, nil
		},
	)); err != nil {
		entryLog.Error(err, "unable to set up controller")
		os.Exit(1)
	}

	entryLog.Info("Starting provider")
	if err := provider.Start(ctx, mgr); err != nil { // does not block
		entryLog.Error(err, "unable to start provider")
		os.Exit(1)
	}

	entryLog.Info("Starting cluster")
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		if err := ignoreCanceled(cl.Start(ctx)); err != nil {
			return fmt.Errorf("failed to start provider: %w", err)
		}
		return nil
	})

	entryLog.Info("Starting cluster-aware manager")
	g.Go(func() error {
		if err := ignoreCanceled(mgr.Start(ctx)); err != nil {
			return fmt.Errorf("unable to run cluster-aware manager: %w", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		entryLog.Error(err, "failed to run managers")
		os.Exit(1)
	}
}

// NamespacedClusterProvider is a cluster provider that represents each namespace
// as a dedicated cluster with only a "default" namespace. It maps each namespace
// to "default" and vice versa, simulating a multi-cluster setup. It uses one
// informer to watch objects for all namespaces.
type NamespacedClusterProvider struct {
	cluster cluster.Cluster

	mgr manager.Manager

	lock      sync.RWMutex
	clusters  map[string]cluster.Cluster
	cancelFns map[string]context.CancelFunc
}

func NewNamespacedClusterProvider(cl cluster.Cluster) *NamespacedClusterProvider {
	return &NamespacedClusterProvider{
		cluster:   cl,
		clusters:  map[string]cluster.Cluster{},
		cancelFns: map[string]context.CancelFunc{},
	}
}

func (p *NamespacedClusterProvider) Start(ctx context.Context, mgr manager.Manager) error {
	nsInf, err := p.cluster.GetCache().GetInformer(ctx, &corev1.Namespace{})
	if err != nil {
		return err
	}

	if _, err := nsInf.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			ns := obj.(*corev1.Namespace)

			p.lock.RLock()
			if _, ok := p.clusters[ns.Name]; ok {
				defer p.lock.RUnlock()
				return
			}

			// create new cluster
			p.lock.Lock()
			clusterCtx, cancel := context.WithCancel(ctx)
			cl := &NamespacedCluster{clusterName: ns.Name, Cluster: p.cluster}
			p.clusters[ns.Name] = cl
			p.cancelFns[ns.Name] = cancel
			p.lock.Unlock()

			if err := mgr.Engage(clusterCtx, cl); err != nil {
				runtime.HandleError(fmt.Errorf("failed to engage manager with cluster %q: %w", ns.Name, err))

				// cleanup
				p.lock.Lock()
				delete(p.clusters, ns.Name)
				delete(p.cancelFns, ns.Name)
				p.lock.Unlock()
			}
		},
		DeleteFunc: func(obj interface{}) {
			ns := obj.(*corev1.Namespace)

			p.lock.RLock()
			cl, ok := p.clusters[ns.Name]
			if !ok {
				p.lock.RUnlock()
				return
			}
			p.lock.RUnlock()

			if err := mgr.Disengage(ctx, cl); err != nil {
				runtime.HandleError(fmt.Errorf("failed to disengage manager with cluster %q: %w", ns.Name, err))
			}

			// stop and forget
			p.lock.Lock()
			p.cancelFns[ns.Name]()
			delete(p.clusters, ns.Name)
			delete(p.cancelFns, ns.Name)
			p.lock.Unlock()
		},
	}); err != nil {
		return err
	}

	return nil
}

func (p *NamespacedClusterProvider) Get(ctx context.Context, clusterName string) (cluster.Cluster, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()
	if cl, ok := p.clusters[clusterName]; ok {
		return cl, nil
	}
	return nil, fmt.Errorf("cluster %s not found", clusterName)
}

func ignoreCanceled(err error) error {
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
