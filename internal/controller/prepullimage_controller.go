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
	"fmt"
	"slices"
	"strconv"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	imagesv1 "github.com/Cdayz/k8s-image-pre-puller/api/v1"
	"github.com/Cdayz/k8s-image-pre-puller/internal/names"
)

const (
	DaemonSetLabelPurposeName  = "cdayz.k8s.extensions/purpose"
	DaemonSetLabelPurposeValue = "images"

	DaemonSetLabelNodeSelectorHash = "cdayz.k8s.extensions/node-selector-hash"

	FinalizerName = "images.cdayz.k8s.extensions/finalizer"

	PodNamePrefix       = "image-pre-puller-"
	DaemonSetNamePrefix = "image-pre-puller-"

	MainContainerName = "main"
)

var (
	MaxUnavailablePodsOfDaemonSetDuringRollingUpdate = intstr.FromString("100%")
)

// PrePullImageReconciler reconciles a PrePullImage object
type PrePullImageReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=images.cdayz.k8s.extensions,resources=prepullimages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=images.cdayz.k8s.extensions,resources=prepullimages/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=images.cdayz.k8s.extensions,resources=prepullimages/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=daemonset,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=daemonset/status,verbs=get

func (r *PrePullImageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	prePullImage := &imagesv1.PrePullImage{}
	if err := r.Get(ctx, req.NamespacedName, prePullImage); err != nil {
		log.Error(err, "unable to fetch PrePullImage")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if prePullImage.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(prePullImage, FinalizerName) {
			controllerutil.AddFinalizer(prePullImage, FinalizerName)
			if err := r.Update(ctx, prePullImage); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		if controllerutil.ContainsFinalizer(prePullImage, FinalizerName) {
			if err := r.removeFromPrePulling(ctx, prePullImage); err != nil {
				return ctrl.Result{}, err
			}

			controllerutil.RemoveFinalizer(prePullImage, FinalizerName)
			if err := r.Update(ctx, prePullImage); err != nil {
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil
	}

	if err := r.ensurePrePulling(ctx, prePullImage); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *PrePullImageReconciler) removeFromPrePulling(ctx context.Context, prePullImage *imagesv1.PrePullImage) error {
	daemonSet, err := r.getCurrentPrePullingDaemonset(ctx, prePullImage)
	if err != nil {
		return fmt.Errorf("get current pre-pulling daemonset: %w", err)
	}
	if daemonSet == nil {
		return nil
	}

	daemonSet.Spec.Template.Spec.InitContainers = slices.DeleteFunc(
		daemonSet.Spec.Template.Spec.InitContainers,
		func(ctr corev1.Container) bool { return ctr.Image == prePullImage.Spec.Image },
	)
	if len(daemonSet.Spec.Template.Spec.InitContainers) > 0 {
		if err := r.Update(ctx, daemonSet); err != nil {
			return fmt.Errorf("remove init-container with pre-puller: %w", err)
		}
	} else {
		if err := r.Delete(ctx, daemonSet); err != nil {
			return fmt.Errorf("remove daemonset: %w", err)
		}
	}

	return nil
}

func (r *PrePullImageReconciler) ensurePrePulling(ctx context.Context, prePullImage *imagesv1.PrePullImage) error {
	daemonSet, err := r.getCurrentPrePullingDaemonset(ctx, prePullImage)
	if err != nil {
		return fmt.Errorf("get current pre-pulling daemonset: %w", err)
	}
	if daemonSet == nil {
		daemonSet, err = r.createDaemonSetForPrePulling(ctx, prePullImage)
		if err != nil {
			return fmt.Errorf("create daemonset: %w", err)
		}
	}

	daemonSet, err = r.ensurePrePullContainerInDaemonSet(ctx, daemonSet, prePullImage)
	if err != nil {
		return fmt.Errorf("ensure pre–pull container in daemonset: %w", err)
	}

	if _, err := r.ensureDaemonsetHasProperTolerations(ctx, daemonSet, prePullImage); err != nil {
		return fmt.Errorf("ensure pre-pull daemonset has proper tolerations: %w", err)
	}

	return nil
}

func (r *PrePullImageReconciler) getCurrentPrePullingDaemonset(ctx context.Context, prePullImage *imagesv1.PrePullImage) (*appsv1.DaemonSet, error) {
	key := client.ObjectKey{Namespace: prePullImage.Namespace, Name: r.makeDaemonSetName(prePullImage)}
	daemonSet := &appsv1.DaemonSet{}
	if err := r.Get(ctx, key, daemonSet); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("fetch daemonset: %w", err)
	}

	return daemonSet, nil
}

func (r *PrePullImageReconciler) createDaemonSetForPrePulling(ctx context.Context, prePullImage *imagesv1.PrePullImage) (*appsv1.DaemonSet, error) {
	labels := map[string]string{
		DaemonSetLabelNodeSelectorHash: r.nodeSelectorHash(prePullImage),
		DaemonSetLabelPurposeName:      DaemonSetLabelPurposeValue,
	}

	tolerations, err := r.getTolerationsByNodeSelector(ctx, prePullImage.Spec.NodeSelector)
	if err != nil {
		return nil, fmt.Errorf("get tolerations for pre-pull: %w", err)
	}

	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.makeDaemonSetName(prePullImage),
			Namespace: prePullImage.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type:          appsv1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDaemonSet{MaxUnavailable: &MaxUnavailablePodsOfDaemonSetDuringRollingUpdate},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: PodNamePrefix,
					Namespace:    prePullImage.Namespace,
					Labels:       labels,
				},
				Spec: corev1.PodSpec{
					InitContainers:   []corev1.Container{r.createPrePullContainer(prePullImage)},
					Containers:       []corev1.Container{r.createMainContainer()},
					RestartPolicy:    corev1.RestartPolicyOnFailure,
					NodeSelector:     prePullImage.Spec.NodeSelector,
					ImagePullSecrets: []corev1.LocalObjectReference{}, // TODO
					Tolerations:      tolerations,
				},
			},
		},
	}

	if err := r.Create(ctx, daemonSet); err != nil {
		return nil, fmt.Errorf("create daemonset: %w", err)
	}

	return daemonSet, nil
}

func (r *PrePullImageReconciler) ensurePrePullContainerInDaemonSet(ctx context.Context, daemonSet *appsv1.DaemonSet, prePullImage *imagesv1.PrePullImage) (*appsv1.DaemonSet, error) {
	hasImageInSet := slices.ContainsFunc(
		daemonSet.Spec.Template.Spec.InitContainers,
		func(ctr corev1.Container) bool { return ctr.Image == prePullImage.Spec.Image },
	)
	if hasImageInSet {
		return daemonSet, nil
	}

	daemonSet.Spec.Template.Spec.InitContainers = append(daemonSet.Spec.Template.Spec.InitContainers, r.createPrePullContainer(prePullImage))
	if err := r.Update(ctx, daemonSet); err != nil {
		return nil, fmt.Errorf("add init-container with pre-puller to daemonset: %w", err)
	}

	return daemonSet, nil
}

func (r *PrePullImageReconciler) ensureDaemonsetHasProperTolerations(ctx context.Context, daemonSet *appsv1.DaemonSet, prePullImage *imagesv1.PrePullImage) (*appsv1.DaemonSet, error) {
	tolerations, err := r.getTolerationsByNodeSelector(ctx, prePullImage.Spec.NodeSelector)
	if err != nil {
		return nil, fmt.Errorf("list tolerations by node selector: %w", err)
	}

	tolerationsAreEqual := slices.Equal(daemonSet.Spec.Template.Spec.Tolerations, tolerations)
	if tolerationsAreEqual {
		return daemonSet, nil
	}

	daemonSet.Spec.Template.Spec.Tolerations = tolerations
	if err := r.Update(ctx, daemonSet); err != nil {
		return nil, fmt.Errorf("update tolerations of daemonset: %w", err)
	}

	return daemonSet, nil
}

func (r *PrePullImageReconciler) getTolerationsByNodeSelector(ctx context.Context, nodeSelector map[string]string) ([]corev1.Toleration, error) {
	matchingLabels := client.MatchingLabelsSelector{Selector: labels.SelectorFromSet(labels.Set(nodeSelector))}

	var nodes corev1.NodeList
	if err := r.List(ctx, &nodes, matchingLabels); err != nil {
		return nil, fmt.Errorf("unable to list nodes: %w", err)
	}

	tolerationMap := map[string]corev1.Toleration{}
	for _, item := range nodes.Items {
		for _, taint := range item.Spec.Taints {
			key := fmt.Sprintf("%s:%s", taint.Key, taint.Effect)

			if _, ok := tolerationMap[key]; !ok {

				tolerationMap[key] = corev1.Toleration{
					Key:      key,
					Operator: corev1.TolerationOpExists,
					Value:    taint.Value,
					Effect:   taint.Effect,
				}
			}
		}
	}

	tolerations := make([]corev1.Toleration, 0, len(tolerationMap))
	for _, v := range tolerationMap {
		tolerations = append(tolerations, v)
	}

	return tolerations, nil
}

func (r *PrePullImageReconciler) createPrePullContainer(prePullImage *imagesv1.PrePullImage) corev1.Container {
	return corev1.Container{
		Name:      r.makePrePullContainerName(prePullImage),
		Image:     prePullImage.Spec.Image,
		Command:   []string{},                    // TODO
		Args:      []string{},                    // TODO
		Resources: corev1.ResourceRequirements{}, // TODO
	}
}

func (r *PrePullImageReconciler) createMainContainer() corev1.Container {
	return corev1.Container{
		Name:      MainContainerName,
		Image:     "",         // TODO
		Command:   []string{}, // TODO
		Args:      []string{}, // TODO
		Resources: corev1.ResourceRequirements{},
	}
}

func (r *PrePullImageReconciler) makePrePullContainerName(prePullImage *imagesv1.PrePullImage) string {
	imageHashStr := names.StringHash(prePullImage.Spec.Image)
	return fmt.Sprintf("pre-pull-%d", imageHashStr)
}

func (r *PrePullImageReconciler) nodeSelectorHash(prePullImage *imagesv1.PrePullImage) string {
	return strconv.FormatUint(uint64(names.StringMapHash(prePullImage.Spec.NodeSelector)), 10)
}

func (r *PrePullImageReconciler) makeDaemonSetName(prePullImage *imagesv1.PrePullImage) string {
	nodeSelectorHashStr := r.nodeSelectorHash(prePullImage)
	return names.MakeK8SName([]string{DaemonSetNamePrefix, nodeSelectorHashStr}, names.IncludeCRC(true), names.MaxLength(63))
}

// SetupWithManager sets up the controller with the Manager.
func (r *PrePullImageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&imagesv1.PrePullImage{}).
		Complete(r)
}
