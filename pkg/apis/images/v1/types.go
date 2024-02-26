package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PrePullImage is the Schema for the prepullimages API
type PrePullImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PrePullImageSpec   `json:"spec,omitempty"`
	Status PrePullImageStatus `json:"status,omitempty"`
}

// PrePullImageSpec defines the desired state of PrePullImage
type PrePullImageSpec struct {
	// Image which should be pulled on nodes
	Image string `json:"image"`
	// NodeSelector for selecting only particular nodes where image should be pre-pulled
	NodeSelector map[string]string `json:"nodeSelector"`
}

type PrePullImageStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PrePullImageList contains a list of PrePullImage
type PrePullImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PrePullImage `json:"items"`
}
