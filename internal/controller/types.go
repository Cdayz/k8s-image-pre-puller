package controller

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ReconcileRequest struct {
	Name      string
	Namespace string

	PodName *string
}

func NewReconcileRequestFromObject(obj metav1.Object) ReconcileRequest {
	return ReconcileRequest{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
}

func (n *ReconcileRequest) Key() string {
	return fmt.Sprintf("%s/%s", n.Namespace, n.Name)
}

type ControllerResult struct {
	Requeue      bool
	RequeueAfter time.Duration
}
