package addon

import (
	"flag"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var initialized bool

// Init should be called at the beginning of the main function for all addon operator controllers
func Init() {
	flag.Set("logtostderr", "true")
	flag.Parse()
	logf.SetLogger(logf.ZapLogger(true))

	initialized = true
}
