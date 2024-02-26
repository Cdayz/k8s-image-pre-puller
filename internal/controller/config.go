package controller

import corev1 "k8s.io/api/core/v1"

type ContainerConfig struct {
	Name      string
	Image     string
	Command   []string
	Args      []string
	Resources corev1.ResourceRequirements
}

type PrePullImageReconcilerConfig struct {
	MainContainer        ContainerConfig
	PrePullContainer     ContainerConfig
	ImagePullSecretNames []string
}
