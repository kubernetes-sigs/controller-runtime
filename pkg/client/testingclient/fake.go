package testingclient

import "sigs.k8s.io/controller-runtime/pkg/client/fake"

var NewFakeClientBuilder = fake.NewClientBuilder
