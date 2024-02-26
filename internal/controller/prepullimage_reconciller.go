package controller

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/Cdayz/k8s-image-pre-puller/internal/controller/utils"
	"github.com/Cdayz/k8s-image-pre-puller/internal/names"
	imagesv1 "github.com/Cdayz/k8s-image-pre-puller/pkg/apis/images/v1"
)

const (
	ControllerLabelPurposeName  = "cdayz.k8s.extensions/purpose"
	ControllerLabelPurposeValue = "images"

	DaemonSetLabelNodeSelectorHash = "cdayz.k8s.extensions/node-selector-hash"

	FinalizerName = "images.cdayz.k8s.extensions/finalizer"

	PodNamePrefix       = "image-pre-puller-"
	DaemonSetNamePrefix = "image-pre-puller-"
)

var MaxUnavailablePodsOfDaemonSetDuringRollingUpdate = intstr.FromString("100%")

func (r *PrePullImageController) Reconcile(ctx context.Context, req ReconcileRequest) (ControllerResult, error) {
	prePullImage, err := r.imClient.ImagesV1().PrePullImages(req.Namespace).Get(ctx, req.Name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ControllerResult{}, nil
		}
		return ControllerResult{}, fmt.Errorf("failed to get PrePullImage<%s/%s>: %w", req.Namespace, req.Name, err)
	}

	if prePullImage.ObjectMeta.DeletionTimestamp.IsZero() {
		if !utils.ContainsFinalizer(prePullImage, FinalizerName) {
			utils.AddFinalizer(prePullImage, FinalizerName)
			prePullImage, err = r.imClient.ImagesV1().PrePullImages(prePullImage.Namespace).Update(ctx, prePullImage, metav1.UpdateOptions{})
			if err != nil {
				return ControllerResult{}, err
			}
		}
	} else {
		if utils.ContainsFinalizer(prePullImage, FinalizerName) {
			if err := r.removeFromPrePulling(ctx, prePullImage); err != nil {
				return ControllerResult{}, err
			}

			utils.RemoveFinalizer(prePullImage, FinalizerName)

			_, err := r.imClient.ImagesV1().PrePullImages(prePullImage.Namespace).Update(ctx, prePullImage, metav1.UpdateOptions{})
			if err != nil {
				return ControllerResult{}, err
			}
		}
		return ControllerResult{}, nil
	}

	if err := r.ensurePrePulling(ctx, prePullImage); err != nil {
		return ControllerResult{}, err
	}

	return ControllerResult{}, nil
}

func (r *PrePullImageController) removeFromPrePulling(ctx context.Context, prePullImage *imagesv1.PrePullImage) error {
	daemonSet, err := r.getCurrentPrePullingDaemonset(ctx, prePullImage)
	if err != nil {
		return fmt.Errorf("get current pre-pulling daemonset: %w", err)
	}
	if daemonSet == nil {
		return nil
	}

	if err := RemoveImageNameFromAnnotation(daemonSet, prePullImage.Name); err != nil {
		return fmt.Errorf("remove image names from daemonset annotations: %w", err)
	}

	daemonSet.Spec.Template.Spec.InitContainers = slices.DeleteFunc(
		daemonSet.Spec.Template.Spec.InitContainers,
		func(ctr corev1.Container) bool { return ctr.Image == prePullImage.Spec.Image },
	)
	if len(daemonSet.Spec.Template.Spec.InitContainers) > 0 {
		_, err := r.kubeClient.AppsV1().DaemonSets(daemonSet.Namespace).Update(ctx, daemonSet, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("remove init-container with pre-puller: %w", err)
		}
	} else {
		err := r.kubeClient.AppsV1().DaemonSets(daemonSet.Namespace).Delete(ctx, daemonSet.Name, metav1.DeleteOptions{})
		if err != nil {
			return fmt.Errorf("remove daemonset: %w", err)
		}
	}

	return nil
}

func (r *PrePullImageController) ensurePrePulling(ctx context.Context, prePullImage *imagesv1.PrePullImage) error {
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

func (r *PrePullImageController) getCurrentPrePullingDaemonset(ctx context.Context, prePullImage *imagesv1.PrePullImage) (*appsv1.DaemonSet, error) {
	daemonSet, err := r.kubeClient.AppsV1().DaemonSets(prePullImage.Namespace).Get(ctx, r.makeDaemonSetName(prePullImage), metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("fetch daemonset: %w", err)
	}

	return daemonSet, nil
}

func (r *PrePullImageController) createDaemonSetForPrePulling(ctx context.Context, prePullImage *imagesv1.PrePullImage) (*appsv1.DaemonSet, error) {
	labels := map[string]string{
		DaemonSetLabelNodeSelectorHash: r.nodeSelectorHash(prePullImage),
		ControllerLabelPurposeName:     ControllerLabelPurposeValue,
	}

	annVal, err := MakeAnnotationValue([]string{prePullImage.Name})
	if err != nil {
		return nil, fmt.Errorf("make index annotation: %w", err)
	}

	annotations := map[string]string{ControllerAnnotationImageNames: annVal}

	imagePullSecrets := []corev1.LocalObjectReference{}
	for _, secretName := range r.reconcileConfig.ImagePullSecretNames {
		imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReference{Name: secretName})
	}

	tolerations, err := r.getTolerationsByNodeSelector(ctx, prePullImage.Spec.NodeSelector)
	if err != nil {
		return nil, fmt.Errorf("get tolerations for pre-pull: %w", err)
	}

	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        r.makeDaemonSetName(prePullImage),
			Namespace:   prePullImage.Namespace,
			Labels:      labels,
			Annotations: annotations,
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
					Annotations:  annotations,
				},
				Spec: corev1.PodSpec{
					InitContainers:   []corev1.Container{r.createPrePullContainer(prePullImage)},
					Containers:       []corev1.Container{r.createMainContainer()},
					RestartPolicy:    corev1.RestartPolicyAlways,
					NodeSelector:     prePullImage.Spec.NodeSelector,
					ImagePullSecrets: imagePullSecrets,
					Tolerations:      tolerations,
				},
			},
		},
	}

	daemonSet, err = r.kubeClient.AppsV1().DaemonSets(daemonSet.Namespace).Create(ctx, daemonSet, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create daemonset: %w", err)
	}

	return daemonSet, nil
}

func (r *PrePullImageController) ensurePrePullContainerInDaemonSet(ctx context.Context, daemonSet *appsv1.DaemonSet, prePullImage *imagesv1.PrePullImage) (*appsv1.DaemonSet, error) {
	var declaredInSpec bool
	for _, ctr := range daemonSet.Spec.Template.Spec.InitContainers {
		if ctr.Image == prePullImage.Spec.Image {
			declaredInSpec = true
			break
		}
	}

	addedToAnnotation, err := AddImageNameToAnnotation(daemonSet, prePullImage.Name)
	if err != nil {
		return nil, fmt.Errorf("add image-name to daemonset annotation: %w", err)
	}

	if declaredInSpec && !addedToAnnotation {
		return daemonSet, nil
	}

	daemonSet.Spec.Template.Spec.InitContainers = append(daemonSet.Spec.Template.Spec.InitContainers, r.createPrePullContainer(prePullImage))
	daemonSet, err = r.kubeClient.AppsV1().DaemonSets(daemonSet.Namespace).Update(ctx, daemonSet, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("add init-container with pre-puller to daemonset: %w", err)
	}

	return daemonSet, nil
}

func (r *PrePullImageController) ensureDaemonsetHasProperTolerations(ctx context.Context, daemonSet *appsv1.DaemonSet, prePullImage *imagesv1.PrePullImage) (*appsv1.DaemonSet, error) {
	requiredTolerations, err := r.getTolerationsByNodeSelector(ctx, prePullImage.Spec.NodeSelector)
	if err != nil {
		return nil, fmt.Errorf("list tolerations by node selector: %w", err)
	}

	daemoSetTolerations := daemonSet.Spec.Template.Spec.Tolerations
	daemoSetTolerationsLookup := map[string][]corev1.Toleration{}
	for _, tol := range daemoSetTolerations {
		if _, ok := daemoSetTolerationsLookup[tol.Key]; !ok {
			daemoSetTolerationsLookup[tol.Key] = []corev1.Toleration{}
		}
		daemoSetTolerationsLookup[tol.Key] = append(daemoSetTolerationsLookup[tol.Key], tol)
	}

	tolerationsToAdd := []corev1.Toleration{}
	for _, toleration := range requiredTolerations {
		has := false
		for _, item := range daemoSetTolerationsLookup[toleration.Key] {
			if item == toleration {
				has = true
				break
			}
		}
		if !has {
			tolerationsToAdd = append(tolerationsToAdd, toleration)
		}
	}

	if len(tolerationsToAdd) == 0 {
		return daemonSet, nil
	}

	daemonSet.Spec.Template.Spec.Tolerations = append(daemonSet.Spec.Template.Spec.Tolerations, tolerationsToAdd...)

	daemonSet, err = r.kubeClient.AppsV1().DaemonSets(daemonSet.Namespace).Update(ctx, daemonSet, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("update tolerations of daemonset: %w", err)
	}

	return daemonSet, nil
}

func (r *PrePullImageController) getTolerationsByNodeSelector(ctx context.Context, nodeSelector map[string]string) ([]corev1.Toleration, error) {
	nodes, err := r.kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(nodeSelector)).String(),
	})
	if err != nil {
		return nil, fmt.Errorf("unable to list nodes: %w", err)
	}

	tolerationMap := map[string]corev1.Toleration{}
	for _, item := range nodes.Items {
		for _, taint := range item.Spec.Taints {
			key := taint.ToString()

			if _, ok := tolerationMap[key]; !ok {
				operator := corev1.TolerationOpExists
				if taint.Value != "" {
					operator = corev1.TolerationOpEqual
				}
				tolerationMap[key] = corev1.Toleration{
					Key:      taint.Key,
					Operator: operator,
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

func (r *PrePullImageController) createPrePullContainer(prePullImage *imagesv1.PrePullImage) corev1.Container {
	return corev1.Container{
		Name:      r.makePrePullContainerName(prePullImage),
		Image:     prePullImage.Spec.Image,
		Command:   r.reconcileConfig.PrePullContainer.Command,
		Args:      r.reconcileConfig.PrePullContainer.Args,
		Resources: r.reconcileConfig.PrePullContainer.Resources,
	}
}

func (r *PrePullImageController) createMainContainer() corev1.Container {
	return corev1.Container{
		Name:      r.reconcileConfig.MainContainer.Name,
		Image:     r.reconcileConfig.MainContainer.Image,
		Command:   r.reconcileConfig.MainContainer.Command,
		Args:      r.reconcileConfig.MainContainer.Args,
		Resources: r.reconcileConfig.MainContainer.Resources,
	}
}

func (r *PrePullImageController) makePrePullContainerName(prePullImage *imagesv1.PrePullImage) string {
	imageHashStr := names.StringHash(prePullImage.Spec.Image)
	return fmt.Sprintf("pre-pull-%d", imageHashStr)
}

func (r *PrePullImageController) nodeSelectorHash(prePullImage *imagesv1.PrePullImage) string {
	return strconv.FormatUint(uint64(names.StringMapHash(prePullImage.Spec.NodeSelector)), 10)
}

func (r *PrePullImageController) makeDaemonSetName(prePullImage *imagesv1.PrePullImage) string {
	nodeSelectorHashStr := r.nodeSelectorHash(prePullImage)
	return names.MakeK8SName([]string{DaemonSetNamePrefix, nodeSelectorHashStr}, names.IncludeCRC(true), names.MaxLength(63))
}
