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

package printer

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/reporters"
	"k8s.io/apimachinery/pkg/util/sets"
)

var (
	allRegisteredSuites     = sets.String{}
	allRegisteredSuitesLock = &sync.Mutex{}
)

// AddReport adds.
func AddReport(report ginkgo.Report, suiteName string) {
	allRegisteredSuitesLock.Lock()
	if allRegisteredSuites.Has(suiteName) {
		panic(fmt.Sprintf("Suite named %q registered more than once", suiteName))
	}
	allRegisteredSuites.Insert(suiteName)
	allRegisteredSuitesLock.Unlock()

	artifactsDir := os.Getenv("ARTIFACTS")

	if os.Getenv("CI") != "" && artifactsDir != "" {
		path := filepath.Join(artifactsDir, fmt.Sprintf("junit_%s_%d.xml", suiteName, report.SuiteConfig.ParallelProcess))
		err := reporters.GenerateJUnitReport(report, path)
		if err != nil {
			fmt.Printf("Failed to generate report\n\t%s", err.Error())
		}
	}
}
