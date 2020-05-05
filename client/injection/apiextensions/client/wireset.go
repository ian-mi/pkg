/*
Copyright 2020 The Knative Authors

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

// Code generated by injection-gen. DO NOT EDIT.

package client

import (
	context "context"

	wire "github.com/google/wire"
	clientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	externalversions "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	controller "knative.dev/pkg/controller"
	injection "knative.dev/pkg/injection"
	injectionwire "knative.dev/pkg/injection/wire"
)

var (
	// Package-wide variables from generator "wireset".
	ClientSet = wire.NewSet(
		clientset.NewForConfig,
		wire.Bind(new(clientset.Interface), new(*clientset.Clientset)),
		NewInformerFactory,
	)
)

func NewInformerFactory(ctx context.Context, c *clientset.Clientset, fs *injectionwire.InformerFactories) externalversions.SharedInformerFactory {
	opts := make([]externalversions.SharedInformerOption, 0, 1)
	if injection.HasNamespaceScope(ctx) {
		opts = append(opts, externalversions.WithNamespace(injection.GetNamespaceScope(ctx)))
	}
	f := externalversions.NewSharedInformerFactoryWithOptions(c, controller.GetResyncPeriod(ctx), opts...)
	fs.AddFactory(f)
	return f
}
