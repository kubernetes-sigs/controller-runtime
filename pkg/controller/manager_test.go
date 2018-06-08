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

package controller

import (
	. "github.com/onsi/ginkgo"
)

var _ = Describe("controller", func() {
	BeforeEach(func() {
	})

	AfterEach(func() {
	})

	Describe("Creating a Manager", func() {

		It("should return an error if Name is not Specified", func() {

		})

		It("should return an error if it can't create a RestMapper", func() {

		})

		It("should return an error if it cannot get a Config", func() {

		})

		It("should default the Config if none is specified", func() {

		})
	})

	Describe("Staring a Manager", func() {

		It("should Start each Controller", func() {

		})

		It("should return an error if any Controllers fail to stop", func() {

		})
	})

	Describe("Manager", func() {
		It("should provide a function to get the Config", func() {

		})

		It("should provide a function to get the Client", func() {

		})

		It("should provide a function to get the Scheme", func() {

		})

		It("should provide a function to get the FieldIndexer", func() {

		})
	})

	Describe("Creating a Controller", func() {

		It("should immediately start the Controller if the ControllerManager has already Started", func() {

		})

		It("should provide an inject function for providing dependencies", func() {

		})
	})

	Describe("Creating a Controller", func() {
		It("should return an error if Name is not Specified", func() {

		})

		It("should return an error if it cannot get a Config", func() {

		})

		It("should default the Config if none is specified", func() {

		})

		It("should immediately start the Controller if the ControllerManager has already Started", func() {

		})

	})
})
