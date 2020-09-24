// Copyright 2020 The NATS Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Code generated by informer-gen. DO NOT EDIT.

package v1

import (
	"context"
	time "time"

	jetstreamv1 "github.com/nats-io/nack/pkg/jetstream/apis/jetstream/v1"
	versioned "github.com/nats-io/nack/pkg/jetstream/generated/clientset/versioned"
	internalinterfaces "github.com/nats-io/nack/pkg/jetstream/generated/informers/externalversions/internalinterfaces"
	v1 "github.com/nats-io/nack/pkg/jetstream/generated/listers/jetstream/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// StreamTemplateInformer provides access to a shared informer and lister for
// StreamTemplates.
type StreamTemplateInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1.StreamTemplateLister
}

type streamTemplateInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewStreamTemplateInformer constructs a new informer for StreamTemplate type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewStreamTemplateInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredStreamTemplateInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredStreamTemplateInformer constructs a new informer for StreamTemplate type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredStreamTemplateInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.JetstreamV1().StreamTemplates(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.JetstreamV1().StreamTemplates(namespace).Watch(context.TODO(), options)
			},
		},
		&jetstreamv1.StreamTemplate{},
		resyncPeriod,
		indexers,
	)
}

func (f *streamTemplateInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredStreamTemplateInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *streamTemplateInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&jetstreamv1.StreamTemplate{}, f.defaultInformer)
}

func (f *streamTemplateInformer) Lister() v1.StreamTemplateLister {
	return v1.NewStreamTemplateLister(f.Informer().GetIndexer())
}
