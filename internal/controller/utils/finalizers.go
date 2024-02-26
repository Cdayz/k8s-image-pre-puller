package utils

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ContainsFinalizer(o metav1.Object, finalizer string) bool {
	f := o.GetFinalizers()
	for _, e := range f {
		if e == finalizer {
			return true
		}
	}
	return false
}

func AddFinalizer(o metav1.Object, finalizer string) (finalizersUpdated bool) {
	f := o.GetFinalizers()
	for _, e := range f {
		if e == finalizer {
			return false
		}
	}
	o.SetFinalizers(append(f, finalizer))
	return true
}

func RemoveFinalizer(o metav1.Object, finalizer string) (finalizersUpdated bool) {
	f := o.GetFinalizers()
	length := len(f)

	index := 0
	for i := 0; i < length; i++ {
		if f[i] == finalizer {
			continue
		}
		f[index] = f[i]
		index++
	}
	o.SetFinalizers(f[:index])
	return length != index
}
