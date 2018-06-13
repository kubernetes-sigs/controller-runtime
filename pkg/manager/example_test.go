/*
Copyright 2018 The Kubernetes Authors.

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

package manager_test

import (
	"log"

	"github.com/kubernetes-sigs/controller-runtime/pkg/client/config"
	"github.com/kubernetes-sigs/controller-runtime/pkg/manager"
	"github.com/kubernetes-sigs/controller-runtime/pkg/runtime/signals"
)

var mrg manager.Manager

// This example creates a new Manager that can be used with controller.New to create Controllers.
func ExampleNew() {
	mrg, err := manager.New(config.GetConfigOrDie(), manager.Options{})
	if err != nil {
		log.Fatal(err)
	}
	log.Print(mrg)
}

// This example adds a Runnable for the Manager to Start.
func ExampleManager_Add() {
	err := mrg.Add(manager.RunnableFunc(func(<-chan struct{}) error {
		// Do something
		return nil
	}))
	if err != nil {
		log.Fatal(err)
	}
}

// This example starts a Manager that has had Runnables added.
func ExampleManager_Start() {
	err := mrg.Start(signals.SetupSignalHandler())
	if err != nil {
		log.Fatal(err)
	}
}
