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

package wire

import (
	"fmt"
	"reflect"
)

type InformerFactory interface {
	Start(<-chan struct{})
	WaitForCacheSync(<-chan struct{}) map[reflect.Type]bool
}

type InformerFactories []InformerFactory

func NewInformerFactories() *InformerFactories {
	return new(InformerFactories)
}

func (fs *InformerFactories) AddFactory(f InformerFactory) {
	*fs = append(*fs, f)
}

// Start kicks off all informer factories and then waits for all of them to synchronize.
func (fs *InformerFactories) Start(stopCh <-chan struct{}) error {
	if fs == nil {
		return nil
	}
	for _, f := range *fs {
		f.Start(stopCh)
	}

	for _, f := range *fs {
		for t, ok := range f.WaitForCacheSync(stopCh) {
			if !ok {
				return fmt.Errorf("failed to wait for cache of type %s to sync", t)
			}
		}
	}
	return nil
}
