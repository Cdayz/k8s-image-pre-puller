package controller

import (
	"reflect"

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

func (cc *PrePullImageController) deletePrePullImage(obj interface{}) {
	prePullImage, ok := obj.(*imagesv1.PrePullImage)
	if !ok {
		// If we reached here it means the pod was deleted but its final state is unrecorded.
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			klog.Errorf("Couldn't get object from tombstone %#v", obj)
			return
		}
		prePullImage, ok = tombstone.Obj.(*imagesv1.PrePullImage)
		if !ok {
			klog.Errorf("Tombstone contained object that is not a PrePullImage: %#v", obj)
			return
		}
	}

	name := NewReconcileRequestFromObject(prePullImage)
	queue := cc.getWorkerQueue(name.Key())
	queue.Add(name)
}
