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

/*
Package webhook provides methods to build and bootstrap a webhook server.

Currently, it only supports admission webhooks. It will support CRD conversion webhooks in the near future.

NOTE: there are some functions and files with `legacy` suffix, which are used to support `k8s.io/api/admission.k8s.io/v1beta1` api.
Since `k8s.io/api/admission.k8s.io/v1beta1` is deprecated in kubernetes v1.16 in favor of admission.k8s.io/v1, please consider
to upgrade if possible.
*/
package webhook

import (
	logf "sigs.k8s.io/controller-runtime/pkg/internal/log"
)

var log = logf.RuntimeLog.WithName("webhook")
