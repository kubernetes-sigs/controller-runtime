Switch Prometheus to OpenTelemetry
===================

## Motivation

Today, controller-runtime already provides metrics accessible through the
Prometheus client, and exposed on the `/metrics` endpoint. However, this forces
components to use Prometheus, when they may want to monitor their app another
way, not necessarily with a pull system.

## Goals

* Provide a generic way to send metrics, so they can be exported virtually anywwhere.
* Do not introduce existing metrics changes for users

## Proposal

We will entirely remove the current metrics server. If users need to run
Prometheus, they will still be able to do so after configuring OpenTelemetry by
using the included exporter.

We will configure a global meter so we can send metrics from anywhere within
the component. That meter will be set when needed, and not at boot time, so
users can set their own `MeterProvider`  and specify where they need to exporte
their metrics.

A new `pkg/meter` package will introduce a `GetMeter()` method, which allows
retrieving the component-wide meter.

```
func GetMeter() metric.Meter {
  return otel.Meter("kubernetes.io/controller-runtime")
}
```

This method will be aliased as `ctrl.GetMeter()`.

Then, every metric we transmit across the component will switch from the
prometheus client to that meter.

For example, in `pkg/internal/controller/metrics`, we have the `ReconcileTotal`
metric, which currently is defined like this:

```go
ReconcileTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
  Name: "controller_runtime_reconcile_total",
  Help: "Total number of reconciliations per controller",
}, []string{"controller", "result"})
```

This package will be removed, as we can't initialize metrics at boot time anymore.
However, the `initMetrics` method in `pkg/internal/controller` will initialize the metric:

```
var (
  reconcileTotal metric.Int64Counter
)

func (c *Controller) initMetrics() {
  reconcileTotal = GetMeter().NewInt64Counter(
    "controller_runtime_reconcile_total",
    metric.WithDescription("Total number of reconciliations per controller")
  )
}
```

Then, when the metric value needs to change, we currently have, in
`pkg/internal/controller`:

```go
ctrlmetrics.ReconcileTotal.WithLabelValues(c.Name, labelError).Inc()
```

Which will be changed to:

```
GetMeter().RecordBatch(
  reconcileTotal.Add(1,
    label.String("controller", c.Name),
    label.String("result", labelError),
  ),
  // More metrics can come here
)
```

## Example

Below is the new way users will be able to define their controllers in
`main.go`, while still using Prometheus:

```go
package main

import (
  "flag"
  "os"

  "k8s.io/apimachinery/pkg/runtime"
  clientgoscheme "k8s.io/client-go/kubernetes/scheme"
  _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
  ctrl "sigs.k8s.io/controller-runtime"
  "sigs.k8s.io/controller-runtime/pkg/log/zap"

  "go.opentelemetry.io/otel/exporters/metric/prometheus"
  "go.opentelemetry.io/otel"

  // +kubebuilder:scaffold:imports
)

var (
  scheme   = runtime.NewScheme()
  setupLog = ctrl.Log.WithName("setup")
)

func init() {
  _ = clientgoscheme.AddToScheme(scheme)

  _ = pgv1.AddToScheme(scheme)
  // +kubebuilder:scaffold:scheme
}

func main() {
  var metricsAddr string
  var enableLeaderElection bool
  flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
  flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
    "Enable leader election for controller manager. "+
      "Enabling this will ensure there is only one active controller manager.")
  flag.Parse()

  err := initMeter(metricsAddr)
  if err != nil {
    setupLog.Error(err, "unable to start metrics")
    os.Exit(1)
  }

  ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

  mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
    Scheme:             scheme,
    Port:               9443,
    LeaderElection:     enableLeaderElection,
    LeaderElectionID:   "0a0da0b6.example.com",
  })
  if err != nil {
    setupLog.Error(err, "unable to start manager")
    os.Exit(1)
  }

  // +kubebuilder:scaffold:builder

  setupLog.Info("starting manager")
  if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
    setupLog.Error(err, "problem running manager")
    os.Exit(1)
  }
}

func initMeter(addr string) error {
	exporter, err := prometheus.InstallNewPipeline(prometheus.Config{})
	if err != nil {
    return err
	}
	http.HandleFunc("/", exporter.ServeHTTP)
	go func() {
		_ = http.ListenAndServe(addr, nil)
	}()

  log.Info(fmt.Sprintf("Prometheus server running on %s", addr))
  return nil
}
```
