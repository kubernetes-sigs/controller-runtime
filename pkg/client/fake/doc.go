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
Package fake provides a fake client for testing.

fake client is backed by its simple object store indexed by GroupVersionResource.
You can create a fake client with optional objects.

	client := NewFakeClient(initObjs...) // initObjs is a slice of runtime.Object

You can invoke the methods defined in the Client interface.

When in doubt, it's almost always better not to use this package and instead use
envtest.Environment with a real client and API server.

Current Limitations / Known Issues with the fake Client:
- this client does not use reactors so it can not be setup with detailed responses for testing
- possible locking issues when using fake client and not the actual client
- by default you are using the wrong scheme
- there is no support for sub resources which can cause issues with tests if you're trying to update
  e.g. metadata and status in the same reconcile
- There is also no OpenAPI validation
- It does not bump `Generation` or `ResourceVersion` so Patch/Update will not behave properly

*/
package fake
