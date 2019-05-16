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

package conversion

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1beta1 "k8s.io/api/apps/v1beta1"
	apix "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kscheme "k8s.io/client-go/kubernetes/scheme"

	jobsapis "sigs.k8s.io/controller-runtime/examples/conversion/pkg/apis"
	jobsv1 "sigs.k8s.io/controller-runtime/examples/conversion/pkg/apis/jobs/v1"
	jobsv2 "sigs.k8s.io/controller-runtime/examples/conversion/pkg/apis/jobs/v2"
)

var _ = Describe("Conversion Webhook", func() {

	var respRecorder *httptest.ResponseRecorder
	var decoder *Decoder
	var scheme *runtime.Scheme
	webhook := Webhook{}

	BeforeEach(func() {
		respRecorder = &httptest.ResponseRecorder{
			Body: bytes.NewBuffer(nil),
		}

		scheme = kscheme.Scheme
		Expect(jobsapis.AddToScheme(scheme)).To(Succeed())
		Expect(webhook.InjectScheme(scheme)).To(Succeed())

		var err error
		decoder, err = NewDecoder(scheme)
		Expect(err).NotTo(HaveOccurred())

	})

	doRequest := func(convReq *apix.ConversionReview) *apix.ConversionReview {
		var payload bytes.Buffer

		Expect(json.NewEncoder(&payload).Encode(convReq)).Should(Succeed())

		convReview := &apix.ConversionReview{}
		req := &http.Request{
			Body: ioutil.NopCloser(bytes.NewReader(payload.Bytes())),
		}
		webhook.ServeHTTP(respRecorder, req)
		Expect(json.NewDecoder(respRecorder.Result().Body).Decode(convReview)).To(Succeed())
		return convReview
	}

	makeV1Obj := func() *jobsv1.ExternalJob {
		return &jobsv1.ExternalJob{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ExternalJob",
				APIVersion: "jobs.example.org/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "obj-1",
			},
			Spec: jobsv1.ExternalJobSpec{
				RunAt: "every 2 seconds",
			},
		}
	}

	It("should convert objects successfully", func() {

		v1Obj := makeV1Obj()

		expected := &jobsv2.ExternalJob{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ExternalJob",
				APIVersion: "jobs.example.org/v2",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "obj-1",
			},
			Spec: jobsv2.ExternalJobSpec{
				ScheduleAt: "every 2 seconds",
			},
		}

		convReq := &apix.ConversionReview{
			TypeMeta: metav1.TypeMeta{},
			Request: &apix.ConversionRequest{
				DesiredAPIVersion: "jobs.example.org/v2",
				Objects: []runtime.RawExtension{
					{
						Object: v1Obj,
					},
				},
			},
		}

		convReview := doRequest(convReq)

		Expect(convReview.Response.ConvertedObjects).To(HaveLen(1))
		got, _, err := decoder.Decode(convReview.Response.ConvertedObjects[0].Raw)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(expected))
	})

	It("should return error when dest/src objects belong to different API groups", func() {
		v1Obj := makeV1Obj()

		convReq := &apix.ConversionReview{
			TypeMeta: metav1.TypeMeta{},
			Request: &apix.ConversionRequest{
				// request conversion for different group
				DesiredAPIVersion: "jobss.example.org/v2",
				Objects: []runtime.RawExtension{
					{
						Object: v1Obj,
					},
				},
			},
		}

		convReview := doRequest(convReq)
		Expect(convReview.Response.Result.Status).To(Equal("Failure"))
		Expect(convReview.Response.ConvertedObjects).To(BeEmpty())
	})

	It("should return error when dest/src objects are of same type", func() {

		v1Obj := makeV1Obj()

		convReq := &apix.ConversionReview{
			TypeMeta: metav1.TypeMeta{},
			Request: &apix.ConversionRequest{
				DesiredAPIVersion: "jobs.example.org/v1",
				Objects: []runtime.RawExtension{
					{
						Object: v1Obj,
					},
				},
			},
		}

		convReview := doRequest(convReq)
		Expect(convReview.Response.Result.Status).To(Equal("Failure"))
		Expect(convReview.Response.ConvertedObjects).To(BeEmpty())
	})

	It("should return error when the API group does not have a hub defined", func() {

		v1Obj := &appsv1beta1.Deployment{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Deployment",
				APIVersion: "apps/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "obj-1",
			},
		}

		convReq := &apix.ConversionReview{
			TypeMeta: metav1.TypeMeta{},
			Request: &apix.ConversionRequest{
				DesiredAPIVersion: "apps/v1",
				Objects: []runtime.RawExtension{
					{
						Object: v1Obj,
					},
				},
			},
		}

		convReview := doRequest(convReq)
		Expect(convReview.Response.Result.Status).To(Equal("Failure"))
		Expect(convReview.Response.ConvertedObjects).To(BeEmpty())
	})

})

var _ = Describe("Convertibility Check", func() {

	var scheme *runtime.Scheme

	BeforeEach(func() {

		scheme = kscheme.Scheme
		Expect(jobsapis.AddToScheme(scheme)).To(Succeed())

	})

	It("should not return error for convertible types", func() {
		obj := &jobsv2.ExternalJob{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ExternalJob",
				APIVersion: "jobs.example.org/v2",
			},
		}

		err := CheckConvertibility(scheme, obj)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should not return error for a built-in multi-version type", func() {
		obj := &appsv1beta1.Deployment{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Deployment",
				APIVersion: "apps/v1beta1",
			},
		}

		err := CheckConvertibility(scheme, obj)
		Expect(err).NotTo(HaveOccurred())
	})
})
