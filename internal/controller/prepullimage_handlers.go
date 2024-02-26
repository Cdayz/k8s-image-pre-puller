package controller

import (
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	imagesv1 "github.com/Cdayz/k8s-image-pre-puller/pkg/apis/images/v1"
)

func (cc *PrePullImageController) addPrePullImage(obj interface{}) {
	prePullImage, ok := obj.(*imagesv1.PrePullImage)
	if !ok {
		klog.Errorf("obj is not PrePullImage")
		return
	}

	name := NewReconcileRequestFromObject(prePullImage)
	queue := cc.getWorkerQueue(name.Key())
	queue.Add(name)
}

func (cc *PrePullImageController) updatePrePullImage(oldObj, newObj interface{}) {
	newPrePull, ok := newObj.(*imagesv1.PrePullImage)
	if !ok {
		klog.Errorf("newObj is not PrePullImage")
		return
	}

	oldPrePull, ok := oldObj.(*imagesv1.PrePullImage)
	if !ok {
		klog.Errorf("oldObj is not PrePullImage")
		return
	}

	// No need to update if ResourceVersion is not changed
	if newPrePull.ResourceVersion == oldPrePull.ResourceVersion {
		klog.V(6).Infof("No need to update because PrePullImage is not modified.")
		return
	}

	// NOTE: Since we only reconcile PrePullImage based on Spec, we will ignore other attributes
	if reflect.DeepEqual(newPrePull.Spec, oldPrePull.Spec) {
		klog.V(6).Infof("PrePullImage update event is ignored since no update in 'Spec'.")
		return
	}

	name := NewReconcileRequestFromObject(newPrePull)
	queue := cc.getWorkerQueue(name.Key())
	queue.Add(name)
}

func (cc *PrePullImageController) addPod(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		klog.Errorf("obj is not Pod")
		return
	}

	if pod.Labels[ControllerLabelPurposeName] != ControllerLabelPurposeValue {
		return
	}

	names, err := GetImageNamesFromAnnotation(pod)
	if err != nil {
		klog.Errorf("failed to get names from pod<%s/%s>: %v", pod.Namespace, pod.Name, err)
		return
	}

	for _, imageName := range names {
		name := ReconcileRequest{Namespace: pod.Namespace, Name: imageName, PodName: &pod.Name}
		queue := cc.getWorkerQueue(name.Key())
		queue.Add(name)
	}
}

func (cc *PrePullImageController) updatePod(oldObj, newObj interface{}) {
	newPod, ok := newObj.(*corev1.Pod)
	if !ok {
		klog.Errorf("newObj is not Pod")
		return
	}

	oldPod, ok := oldObj.(*corev1.Pod)
	if !ok {
		klog.Errorf("oldObj is not Pod")
		return
	}

	// No need to update if ResourceVersion is not changed
	if newPod.ResourceVersion == oldPod.ResourceVersion {
		klog.V(6).Infof("No need to update because Pod is not modified.")
		return
	}

	if newPod.Labels[ControllerLabelPurposeName] != ControllerLabelPurposeValue {
		return
	}

	names, err := GetImageNamesFromAnnotation(newPod)
	if err != nil {
		klog.Errorf("failed to get names from pod<%s/%s>: %v", newPod.Namespace, newPod.Name, err)
		return
	}

	for _, imageName := range names {
		name := ReconcileRequest{Namespace: newPod.Namespace, Name: imageName, PodName: &newPod.Name}
		queue := cc.getWorkerQueue(name.Key())
		queue.Add(name)
	}
}

func (cc *PrePullImageController) deletePod(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		// If we reached here it means the pod was deleted but its final state is unrecorded.
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			klog.Errorf("Couldn't get object from tombstone %#v", obj)
			return
		}
		pod, ok = tombstone.Obj.(*corev1.Pod)
		if !ok {
			klog.Errorf("Tombstone contained object that is not a Pod: %#v", obj)
			return
		}
	}

	if pod.Labels[ControllerLabelPurposeName] != ControllerLabelPurposeValue {
		return
	}

	names, err := GetImageNamesFromAnnotation(pod)
	if err != nil {
		klog.Errorf("failed to get names from pod<%s/%s>: %v", pod.Namespace, pod.Name, err)
		return
	}

	for _, imageName := range names {
		name := ReconcileRequest{Namespace: pod.Namespace, Name: imageName, PodName: &pod.Name}
		queue := cc.getWorkerQueue(name.Key())
		queue.Add(name)
	}
}
