package main

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
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
	"sigs.k8s.io/logical-cluster"
)

func init() {
	ctrl.SetLogger(klog.Background())
}

func main() {
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

	// Setup a Manager
	entryLog.Info("Setting up manager")
	mgr, err := manager.New(
		cfg,
		manager.Options{}.WithExperimentalClusterProvider(&KindClusterProvider{}),
	)
	if err != nil {
		entryLog.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	builder.ControllerManagedBy(mgr).
		For(&corev1.Pod{}).Complete(reconcile.Func(
		func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
			log := log.FromContext(ctx)

			cluster, err := mgr.GetCluster(ctx, req.Cluster)
			if err != nil {
				return reconcile.Result{}, err
			}
			client := cluster.GetClient()

			// Retrieve the pod from the cluster.
			pod := &corev1.Pod{}
			if err := client.Get(ctx, req.NamespacedName, pod); err != nil {
				return reconcile.Result{}, err
			}
			log.Info("Reconciling pod", "name", pod.Name, "uuid", pod.UID)

			// Print any annotations that start with fleet.
			for k, v := range pod.Labels {
				if strings.HasPrefix(k, "fleet-") {
					log.Info("Detected fleet annotation!", "key", k, "value", v)
				}
			}

			return ctrl.Result{}, nil
		},
	))

	entryLog.Info("Starting manager")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		entryLog.Error(err, "unable to run manager")
		os.Exit(1)
	}
}

// KindClusterProvider is a cluster provider that works with a local Kind instance.
type KindClusterProvider struct{}

func (k *KindClusterProvider) Get(ctx context.Context, name logical.Name, opts ...cluster.Option) (cluster.Cluster, error) {
	provider := kind.NewProvider()
	kubeconfig, err := provider.KubeConfig(string(name), false)
	if err != nil {
		return nil, err
	}
	// Parse the kubeconfig into a rest.Config.
	cfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
	if err != nil {
		return nil, err
	}
	return cluster.New(cfg, opts...)
}

func (k *KindClusterProvider) List() ([]logical.Name, error) {
	provider := kind.NewProvider()
	list, err := provider.List()
	if err != nil {
		return nil, err
	}
	res := make([]logical.Name, 0, len(list))
	for _, cluster := range list {
		if !strings.HasPrefix(cluster, "fleet-") {
			continue
		}
		res = append(res, logical.Name(cluster))
	}
	return res, nil
}

func (k *KindClusterProvider) Watch() (cluster.Watcher, error) {
	return &KindWatcher{ch: make(chan cluster.WatchEvent)}, nil
}

type KindWatcher struct {
	init   sync.Once
	wg     sync.WaitGroup
	ch     chan cluster.WatchEvent
	cancel context.CancelFunc
}

func (k *KindWatcher) Stop() {
	if k.cancel != nil {
		k.cancel()
	}
	k.wg.Wait()
	close(k.ch)
}
func (k *KindWatcher) ResultChan() <-chan cluster.WatchEvent {
	k.init.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		k.cancel = cancel
		set := sets.New[string]()
		k.wg.Add(1)
		go func() {
			defer k.wg.Done()
			for {
				select {
				case <-time.After(2 * time.Second):
					provider := kind.NewProvider()
					list, err := provider.List()
					if err != nil {
						klog.Error(err)
						continue
					}
					newSet := sets.New(list...)
					// Check for new clusters.
					for _, cl := range newSet.Difference(set).UnsortedList() {
						if !strings.HasPrefix(cl, "fleet-") {
							continue
						}
						k.ch <- cl.WatchEvent{
							Type: watch.Added,
							Name: logical.Name(cl),
						}
					}
					// Check for deleted clusters.
					for _, cl := range set.Difference(newSet).UnsortedList() {
						if !strings.HasPrefix(cl, "fleet-") {
							continue
						}
						k.ch <- cluster.WatchEvent{
							Type: watch.Deleted,
							Name: logical.Name(cl),
						}
					}
					set = newSet
				case <-ctx.Done():
					return
				}
			}
		}()
	})
	return k.ch
}
