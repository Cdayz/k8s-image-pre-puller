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

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PrePullImageSpec defines the desired state of PrePullImage
type PrePullImageSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Image which should be pulled on nodes
	Image string `json:"image"`
	// NodeSelector for selecting only particular nodes where image should be pre-pulled
	NodeSelector map[string]string `json:"nodeSelector"`
}

// PrePullImageStatus defines the observed state of PrePullImage
type PrePullImageStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// DaemonSetRef is reference to daemonSet which used to pull image
	DaemonSetRef *corev1.ObjectReference `json:"daemonSetRef"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// PrePullImage is the Schema for the prepullimages API
type PrePullImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PrePullImageSpec   `json:"spec,omitempty"`
	Status PrePullImageStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PrePullImageList contains a list of PrePullImage
type PrePullImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PrePullImage `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PrePullImage{}, &PrePullImageList{})
}
