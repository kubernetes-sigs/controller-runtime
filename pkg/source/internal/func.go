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

package internal

import (
	"context"
	"fmt"

	"k8s.io/client-go/util/workqueue"
)

// Func is a function that implements Source.
type Func func(context.Context, workqueue.RateLimitingInterface) error

// Start implements Source.
func (f Func) Start(ctx context.Context, queue workqueue.RateLimitingInterface) error {
	return f(ctx, queue)
}

func (f Func) String() string {
	return fmt.Sprintf("func source: %p", f)
}
