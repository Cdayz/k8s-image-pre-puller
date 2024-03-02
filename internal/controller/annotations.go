package controller

import (
	"encoding/json"
	"fmt"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const ControllerAnnotationImageNames = "cdayz.k8s.extensions/image-names"

type (
	ImageName                = string
	PrePullImageResourceName = string
	ImageNamesLookup         struct {
		Index         map[PrePullImageResourceName]ImageName   `json:"index"`
		InvertedIndex map[ImageName][]PrePullImageResourceName `json:"invIndex"`
	}
)

func GetImageNamesFromAnnotation(obj metav1.Object) (*ImageNamesLookup, error) {
	var imageNames ImageNamesLookup
	if imageNamesListStr, ok := obj.GetAnnotations()[ControllerAnnotationImageNames]; ok {
		if err := json.Unmarshal([]byte(imageNamesListStr), &imageNames); err != nil {
			return nil, fmt.Errorf("unmarshal image-names annotation: %w", err)
		}
	}
	return &imageNames, nil
}

func RemoveImageNameFromAnnotation(obj metav1.Object, imageName, objectName string) (*ImageNamesLookup, error) {
	imageNames, err := GetImageNamesFromAnnotation(obj)
	if err != nil {
		return nil, err
	}

	oldImageName, existsInIndex := imageNames.Index[objectName]
	_, exitstInInvIndex := imageNames.InvertedIndex[oldImageName]

	if !existsInIndex && !exitstInInvIndex {
		return imageNames, nil
	}

	delete(imageNames.Index, objectName)
	imageNames.InvertedIndex[imageName] = slices.DeleteFunc(imageNames.InvertedIndex[imageName], func(s string) bool { return s == objectName })
	if len(imageNames.InvertedIndex[imageName]) == 0 {
		delete(imageNames.InvertedIndex, imageName)
	}
	imageNames.InvertedIndex[oldImageName] = slices.DeleteFunc(imageNames.InvertedIndex[oldImageName], func(s string) bool { return s == objectName })
	if len(imageNames.InvertedIndex[oldImageName]) == 0 {
		delete(imageNames.InvertedIndex, oldImageName)
	}

	val, err := MakeAnnotationValue(imageNames)
	if err != nil {
		return nil, err
	}

	ann := obj.GetAnnotations()
	ann[ControllerAnnotationImageNames] = val
	obj.SetAnnotations(ann)

	return imageNames, nil
}

func AddImageNameToAnnotation(obj metav1.Object, imageName, objectName string) (added bool, err error) {
	imageNames, err := GetImageNamesFromAnnotation(obj)
	if err != nil {
		return false, err
	}

	oldImageName, existInIndex := imageNames.Index[objectName]
	if existInIndex && oldImageName == imageName && slices.Contains(imageNames.InvertedIndex[imageName], objectName) {
		return false, nil
	}

	imageNames.Index[objectName] = imageName
	imageNames.InvertedIndex[oldImageName] = slices.DeleteFunc(imageNames.InvertedIndex[oldImageName], func(s string) bool { return s == objectName })
	if len(imageNames.InvertedIndex[oldImageName]) == 0 {
		delete(imageNames.InvertedIndex, oldImageName)
	}

	imageNames.InvertedIndex[imageName] = append(imageNames.InvertedIndex[imageName], objectName)
	val, err := MakeAnnotationValue(imageNames)
	if err != nil {
		return false, err
	}

	ann := obj.GetAnnotations()
	ann[ControllerAnnotationImageNames] = val
	obj.SetAnnotations(ann)

	return true, nil
}

func MakeAnnotationValue(imageNames *ImageNamesLookup) (string, error) {
	imageNamesListBytes, err := json.Marshal(imageNames)
	if err != nil {
		return "", fmt.Errorf("marshal image-name annotation: %w", err)
	}

	return string(imageNamesListBytes), nil
}
