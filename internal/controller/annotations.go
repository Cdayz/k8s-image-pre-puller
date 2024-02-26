package controller

import (
	"encoding/json"
	"fmt"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const ControllerAnnotationImageNames = "cdayz.k8s.extensions/image-names"

func GetImageNamesFromAnnotation(obj metav1.Object) ([]string, error) {
	var imageNames []string
	if imageNamesListStr, ok := obj.GetAnnotations()[ControllerAnnotationImageNames]; ok {
		if err := json.Unmarshal([]byte(imageNamesListStr), &imageNames); err != nil {
			return nil, fmt.Errorf("unmarshal image-names annotation: %w", err)
		}
	}
	return imageNames, nil
}

func RemoveImageNameFromAnnotation(obj metav1.Object, name string) error {
	imageNames, err := GetImageNamesFromAnnotation(obj)
	if err != nil {
		return err
	}

	imageNames = slices.DeleteFunc(imageNames, func(s string) bool { return s == name })

	val, err := MakeAnnotationValue(imageNames)
	if err != nil {
		return err
	}

	ann := obj.GetAnnotations()
	ann[ControllerAnnotationImageNames] = val
	obj.SetAnnotations(ann)

	return nil
}

func AddImageNameToAnnotation(obj metav1.Object, name string) (added bool, err error) {
	imageNames, err := GetImageNamesFromAnnotation(obj)
	if err != nil {
		return false, err
	}

	if slices.Contains(imageNames, name) {
		return false, nil
	}

	imageNames = append(imageNames, name)
	val, err := MakeAnnotationValue(imageNames)
	if err != nil {
		return false, err
	}

	ann := obj.GetAnnotations()
	ann[ControllerAnnotationImageNames] = val
	obj.SetAnnotations(ann)

	return true, nil
}

func MakeAnnotationValue(imageNames []string) (string, error) {
	imageNamesListBytes, err := json.Marshal(imageNames)
	if err != nil {
		return "", fmt.Errorf("marshal image-name annotation: %w", err)
	}

	return string(imageNamesListBytes), nil
}
