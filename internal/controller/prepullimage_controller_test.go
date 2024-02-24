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

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	imagesv1 "github.com/Cdayz/k8s-image-pre-puller/api/v1"
)

var _ = Describe("PrePullImage Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		prepullimage := &imagesv1.PrePullImage{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind PrePullImage")
			err := k8sClient.Get(ctx, typeNamespacedName, prepullimage)
			if err != nil && errors.IsNotFound(err) {
				resource := &imagesv1.PrePullImage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: imagesv1.PrePullImageSpec{
						Image:        "test-image",
						NodeSelector: map[string]string{},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &imagesv1.PrePullImage{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance PrePullImage")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &PrePullImageReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),

				Config: PrePullImageReconcilerConfig{
					MainContainer: ContainerConfig{
						Name:    "main",
						Image:   "busybox",
						Command: []string{"/bin/sh"},
						Args:    []string{"-c", "'sleep inf'"},
					},
					PrePullContainer: ContainerConfig{
						Command:   []string{"/bin/sh"},
						Args:      []string{"-c", "'exit 0'"},
						Resources: corev1.ResourceRequirements{},
					},
					ImagePullSecretNames: []string{"artifactory-cred-secret"},
				},
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
