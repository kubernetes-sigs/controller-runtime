package erreur_test

import (
	oldfmt "fmt"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/erreur"
	rzap "go.uber.org/zap"
	"golang.org/x/exp/errors/fmt"
)

func init() {
	out, _, err := rzap.Open()
	if err != nil {
		panic(err)
	}
	log.SetLogger(zap.LoggerTo(out, false))
}

func TestWithErreur(t *testing.T) {
	printErreurError()
}

func printErreurError() {
	cause := erreur.WithValues("something went wrong", "borked", "subsystem")
	err := erreur.WithValues("there was a problem", "problem", "in the manifolds").CausedBy(cause)
	log.Log.Error(err, "couldn't do the thing")
}

func BenchmarkWithErreur(b *testing.B) {
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		printErreurError()
	}
}

func printExpError() {
	cause := fmt.Errorf("something went wrong -- borked: %s", "subsystem")
	err := fmt.Errorf("there was a problem -- problem: %s -- %v", "in the manifolds", cause)
	log.Log.Error(err, "couldn't do the thing")
}

func BenchmarkExpError(b *testing.B) {
	for i := 0; i < b.N; i++ {
		printExpError()
	}
}

func printFmtError() {
	cause := oldfmt.Errorf("something went wrong -- borked: %s", "subsystem")
	err := oldfmt.Errorf("there was a problem -- problem: %s -- %v", "in the manifolds", cause)
	log.Log.Error(err, "couldn't do the thing")
}

func BenchmarkPlainFmt(b *testing.B) {
	for i := 0; i < b.N; i++ {
		printFmtError()
	}
}
