/*
Copyright 2024.

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
// Code generated by lister-gen. DO NOT EDIT.

package v1

import (
	v1 "github.com/Cdayz/k8s-image-pre-puller/pkg/apis/images/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// PrePullImageLister helps list PrePullImages.
// All objects returned here must be treated as read-only.
type PrePullImageLister interface {
	// List lists all PrePullImages in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1.PrePullImage, err error)
	// PrePullImages returns an object that can list and get PrePullImages.
	PrePullImages(namespace string) PrePullImageNamespaceLister
	PrePullImageListerExpansion
}

// prePullImageLister implements the PrePullImageLister interface.
type prePullImageLister struct {
	indexer cache.Indexer
}

// NewPrePullImageLister returns a new PrePullImageLister.
func NewPrePullImageLister(indexer cache.Indexer) PrePullImageLister {
	return &prePullImageLister{indexer: indexer}
}

// List lists all PrePullImages in the indexer.
func (s *prePullImageLister) List(selector labels.Selector) (ret []*v1.PrePullImage, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.PrePullImage))
	})
	return ret, err
}

// PrePullImages returns an object that can list and get PrePullImages.
func (s *prePullImageLister) PrePullImages(namespace string) PrePullImageNamespaceLister {
	return prePullImageNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// PrePullImageNamespaceLister helps list and get PrePullImages.
// All objects returned here must be treated as read-only.
type PrePullImageNamespaceLister interface {
	// List lists all PrePullImages in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1.PrePullImage, err error)
	// Get retrieves the PrePullImage from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1.PrePullImage, error)
	PrePullImageNamespaceListerExpansion
}

// prePullImageNamespaceLister implements the PrePullImageNamespaceLister
// interface.
type prePullImageNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all PrePullImages in the indexer for a given namespace.
func (s prePullImageNamespaceLister) List(selector labels.Selector) (ret []*v1.PrePullImage, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.PrePullImage))
	})
	return ret, err
}

// Get retrieves the PrePullImage from the indexer for a given namespace and name.
func (s prePullImageNamespaceLister) Get(name string) (*v1.PrePullImage, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1.Resource("prepullimage"), name)
	}
	return obj.(*v1.PrePullImage), nil
}
