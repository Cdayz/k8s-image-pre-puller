package controller

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/suite"
	"golang.org/x/exp/maps"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	imagesv1 "github.com/Cdayz/k8s-image-pre-puller/pkg/apis/images/v1"
	imclientset "github.com/Cdayz/k8s-image-pre-puller/pkg/client/clientset/versioned"
	imclientsetfake "github.com/Cdayz/k8s-image-pre-puller/pkg/client/clientset/versioned/fake"
)

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestPrePullImageReconcillerTestSuite(t *testing.T) {
	suite.Run(t, new(PrePullImageReconcillerTestSuite))
}

type PrePullImageReconcillerTestSuite struct {
	suite.Suite

	k8sClient         kubernetes.Interface
	imClient          imclientset.Interface
	reconcillerConfig *PrePullImageReconcilerConfig
}

func (s *PrePullImageReconcillerTestSuite) SetupTest() {
	s.k8sClient = fake.NewSimpleClientset()
	s.imClient = imclientsetfake.NewSimpleClientset()
	s.reconcillerConfig = &PrePullImageReconcilerConfig{
		MainContainer: ContainerConfig{
			Name:      "main",
			Image:     "busybox",
			Command:   []string{"/bin/sh"},
			Args:      []string{"-c", "'sleep inf'"},
			Resources: corev1.ResourceRequirements{},
		},
		PrePullContainer: ContainerConfig{
			Command:   []string{"/bin/sh"},
			Args:      []string{"-c", "'exit 0'"},
			Resources: corev1.ResourceRequirements{},
		},
		ImagePullSecretNames: []string{"artifactory-cred-secret"},
	}
}

func (s *PrePullImageReconcillerTestSuite) validateDaemonSetMatchesPrePullImage(daemonSet *appsv1.DaemonSet, prePullImage *imagesv1.PrePullImage) {
	s.T().Helper()

	s.Require().Equal(makeDaemonSetName(prePullImage), daemonSet.Name)
	s.Require().GreaterOrEqual(len(daemonSet.Spec.Template.Spec.InitContainers), 1)

	imageNames, err := GetImageNamesFromAnnotation(daemonSet)
	s.Require().NoError(err)
	s.Require().Contains(imageNames.Index, prePullImage.Name)
	s.Require().Equal(imageNames.Index[prePullImage.Name], prePullImage.Spec.Image)
	s.Require().Contains(imageNames.InvertedIndex, prePullImage.Spec.Image)
	s.Require().Contains(imageNames.InvertedIndex[prePullImage.Spec.Image], prePullImage.Name)

	imageNames, err = GetImageNamesFromAnnotation(&daemonSet.Spec.Template)
	s.Require().NoError(err)
	s.Require().Contains(imageNames.Index, prePullImage.Name)
	s.Require().Equal(imageNames.Index[prePullImage.Name], prePullImage.Spec.Image)
	s.Require().Contains(imageNames.InvertedIndex, prePullImage.Spec.Image)
	s.Require().Contains(imageNames.InvertedIndex[prePullImage.Spec.Image], prePullImage.Name)

	idx := slices.IndexFunc(daemonSet.Spec.Template.Spec.InitContainers, func(c corev1.Container) bool {
		return c.Image == prePullImage.Spec.Image
	})
	s.Require().NotEqual(-1, idx, "init container should be defined inside daemonset")
}

func (s *PrePullImageReconcillerTestSuite) validateDaemonSetContainsOnlyProperContainers(daemonSet *appsv1.DaemonSet) {
	s.T().Helper()

	imageNames, err := GetImageNamesFromAnnotation(daemonSet)
	s.Require().NoError(err)

	imagesList, err := s.imClient.ImagesV1().PrePullImages(daemonSet.Namespace).List(context.Background(), metav1.ListOptions{})
	s.Require().NoError(err)

	matchingImages := []imagesv1.PrePullImage{}
	matchingImagesLookup := ImageNamesLookup{
		Index:         map[string]string{},
		InvertedIndex: map[string][]string{},
	}
	for _, image := range imagesList.Items {
		if makeDaemonSetName(&image) != daemonSet.Name {
			continue
		}
		if !image.ObjectMeta.DeletionTimestamp.IsZero() {
			continue
		}
		matchingImages = append(matchingImages, image)
		matchingImagesLookup.InvertedIndex[image.Spec.Image] = append(matchingImagesLookup.InvertedIndex[image.Spec.Image], image.Name)
	}

	// Ensure annotation matches real data
	s.Require().Lenf(matchingImagesLookup.InvertedIndex, len(imageNames.InvertedIndex), "%v", imageNames.InvertedIndex)
	s.Require().ElementsMatch(maps.Keys(imageNames.InvertedIndex), maps.Keys(matchingImagesLookup.InvertedIndex))
	for imageName, objectNames := range imageNames.InvertedIndex {
		s.Require().ElementsMatchf(objectNames, matchingImagesLookup.InvertedIndex[imageName], "objectNames by key %s didnt match in invertedIndex", imageName)
	}

	// Ensure daemonset really matches real data
	for _, img := range matchingImages {
		s.validateDaemonSetMatchesPrePullImage(daemonSet, &img)
	}

	// Ensure only containers related to annotation declared
	for _, ctr := range daemonSet.Spec.Template.Spec.InitContainers {
		s.Require().Contains(imageNames.InvertedIndex, ctr.Image)
	}
}

func (s *PrePullImageReconcillerTestSuite) TestWhenObjectDoesNotExistsNothingChanged() {
	r := prePullImageReconciller{
		reconcileConfig: s.reconcillerConfig,
		kubeClient:      s.k8sClient,
		imClient:        s.imClient,
	}

	result, err := r.Reconcile(context.Background(), ReconcileRequest{
		Name:      "",
		Namespace: "",
	})
	s.Require().NoError(err)
	s.Require().Equal(ControllerResult{}, result)

	daemonSets, err := s.k8sClient.AppsV1().DaemonSets(metav1.NamespaceAll).List(context.Background(), metav1.ListOptions{})
	s.Require().NoError(err)
	s.Require().Len(daemonSets.Items, 0, "no daemonsets should be created")
}

func (s *PrePullImageReconcillerTestSuite) TestWhenNewObjectComesDaemonsetCreated() {
	r := prePullImageReconciller{
		reconcileConfig: s.reconcillerConfig,
		kubeClient:      s.k8sClient,
		imClient:        s.imClient,
	}

	namespace := "default"
	prePullImage, err := s.imClient.ImagesV1().PrePullImages(namespace).Create(context.Background(), &imagesv1.PrePullImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pre-pull-1",
			Namespace: namespace,
		},
		Spec: imagesv1.PrePullImageSpec{
			Image: "my-image:v0.0.1",
		},
	}, metav1.CreateOptions{})
	s.Require().NoError(err)

	result, err := r.Reconcile(context.Background(), ReconcileRequest{
		Name:      prePullImage.Name,
		Namespace: prePullImage.Namespace,
	})
	s.Require().NoError(err)
	s.Require().Equal(ControllerResult{}, result)

	daemonSets, err := s.k8sClient.AppsV1().DaemonSets(prePullImage.Namespace).List(context.Background(), metav1.ListOptions{})
	s.Require().NoError(err)
	s.Require().Len(daemonSets.Items, 1, "daemonset should be created")
	s.validateDaemonSetContainsOnlyProperContainers(&daemonSets.Items[0])
}

func (s *PrePullImageReconcillerTestSuite) TestWhenLastObjectRelatedToDaemonSetDeletesDaemonsetDeleted() {
	r := prePullImageReconciller{
		reconcileConfig: s.reconcillerConfig,
		kubeClient:      s.k8sClient,
		imClient:        s.imClient,
	}

	// Create object and daemonset
	var prePullImage *imagesv1.PrePullImage
	{
		var err error
		namespace := "default"
		prePullImage, err = s.imClient.ImagesV1().PrePullImages(namespace).Create(context.Background(), &imagesv1.PrePullImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pre-pull-1",
				Namespace: namespace,
			},
			Spec: imagesv1.PrePullImageSpec{
				Image: "my-image:v0.0.1",
			},
		}, metav1.CreateOptions{})
		s.Require().NoError(err)

		result, err := r.Reconcile(context.Background(), ReconcileRequest{
			Name:      prePullImage.Name,
			Namespace: prePullImage.Namespace,
		})
		s.Require().NoError(err)
		s.Require().Equal(ControllerResult{}, result)

		daemonSets, err := s.k8sClient.AppsV1().DaemonSets(prePullImage.Namespace).List(context.Background(), metav1.ListOptions{})
		s.Require().NoError(err)
		s.Require().Len(daemonSets.Items, 1, "daemonset should be created")
		s.validateDaemonSetContainsOnlyProperContainers(&daemonSets.Items[0])
	}

	// Refresh object, we need to update from latest changes
	prePullImage, err := s.imClient.ImagesV1().PrePullImages(prePullImage.Namespace).Get(context.Background(), prePullImage.Name, metav1.GetOptions{})
	s.Require().NoError(err)

	// Mark object for deletion
	now := metav1.Now()
	prePullImage.DeletionTimestamp = &now
	prePullImage, err = s.imClient.ImagesV1().PrePullImages(prePullImage.Namespace).Update(context.Background(), prePullImage, metav1.UpdateOptions{})
	s.Require().NoError(err)

	result, err := r.Reconcile(context.Background(), ReconcileRequest{
		Name:      prePullImage.Name,
		Namespace: prePullImage.Namespace,
	})
	s.Require().NoError(err)
	s.Require().Equal(ControllerResult{}, result)

	daemonSets, err := s.k8sClient.AppsV1().DaemonSets(prePullImage.Namespace).List(context.Background(), metav1.ListOptions{})
	s.Require().NoError(err)
	s.Require().Len(daemonSets.Items, 0, "daemonset should be deleted")
}

func (s *PrePullImageReconcillerTestSuite) TestWhenObjectSpecUpdatedDaemonsetAlsoUpdated() {
	r := prePullImageReconciller{
		reconcileConfig: s.reconcillerConfig,
		kubeClient:      s.k8sClient,
		imClient:        s.imClient,
	}

	// Prepare stage
	namespace := "default"
	prePullImage, err := s.imClient.ImagesV1().PrePullImages(namespace).Create(context.Background(), &imagesv1.PrePullImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pre-pull-1",
			Namespace: namespace,
		},
		Spec: imagesv1.PrePullImageSpec{
			Image: "my-image:v0.0.1",
		},
	}, metav1.CreateOptions{})
	s.Require().NoError(err)

	result, err := r.Reconcile(context.Background(), ReconcileRequest{
		Name:      prePullImage.Name,
		Namespace: prePullImage.Namespace,
	})
	s.Require().NoError(err)
	s.Require().Equal(ControllerResult{}, result)

	daemonSets, err := s.k8sClient.AppsV1().DaemonSets(prePullImage.Namespace).List(context.Background(), metav1.ListOptions{})
	s.Require().NoError(err)
	s.Require().Len(daemonSets.Items, 1, "daemonset should be created")
	s.validateDaemonSetContainsOnlyProperContainers(&daemonSets.Items[0])

	// Update
	prePullImage.Spec.Image = "my-image-2:v0.0.2"
	prePullImage, err = s.imClient.ImagesV1().PrePullImages(namespace).Update(context.Background(), prePullImage, metav1.UpdateOptions{})
	s.Require().NoError(err)

	// Reconcile updated object
	result, err = r.Reconcile(context.Background(), ReconcileRequest{
		Name:      prePullImage.Name,
		Namespace: prePullImage.Namespace,
	})
	s.Require().NoError(err)
	s.Require().Equal(ControllerResult{}, result)

	// Assert updated are commited
	daemonSets, err = s.k8sClient.AppsV1().DaemonSets(prePullImage.Namespace).List(context.Background(), metav1.ListOptions{})
	s.Require().NoError(err)
	s.Require().Len(daemonSets.Items, 1, "daemonset should be created")
	s.validateDaemonSetContainsOnlyProperContainers(&daemonSets.Items[0])
}

func (s *PrePullImageReconcillerTestSuite) TestWhenObjectSpecUpdatedDaemonsetAlsoUpdatedManyObjectDifferentImages() {
	r := prePullImageReconciller{
		reconcileConfig: s.reconcillerConfig,
		kubeClient:      s.k8sClient,
		imClient:        s.imClient,
	}

	// Prepare stage
	var (
		prePullImageFirst, prePullImageSecond *imagesv1.PrePullImage
		err                                   error
	)
	{
		namespace := "default"
		prePullImageFirst, err = s.imClient.ImagesV1().PrePullImages(namespace).Create(context.Background(), &imagesv1.PrePullImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pre-pull-1",
				Namespace: namespace,
			},
			Spec: imagesv1.PrePullImageSpec{
				Image: "my-image:v0.0.1",
			},
		}, metav1.CreateOptions{})
		s.Require().NoError(err)

		result, err := r.Reconcile(context.Background(), ReconcileRequest{
			Name:      prePullImageFirst.Name,
			Namespace: prePullImageFirst.Namespace,
		})
		s.Require().NoError(err)
		s.Require().Equal(ControllerResult{}, result)

		daemonSets, err := s.k8sClient.AppsV1().DaemonSets(prePullImageFirst.Namespace).List(context.Background(), metav1.ListOptions{})
		s.Require().NoError(err)
		s.Require().Len(daemonSets.Items, 1, "daemonset should be created")
		s.validateDaemonSetContainsOnlyProperContainers(&daemonSets.Items[0])
	}

	// Prepare stage - second object with different image
	{
		namespace := "default"
		prePullImageSecond, err = s.imClient.ImagesV1().PrePullImages(namespace).Create(context.Background(), &imagesv1.PrePullImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pre-pull-2",
				Namespace: namespace,
			},
			Spec: imagesv1.PrePullImageSpec{
				Image: "my-image-2:v0.0.1",
			},
		}, metav1.CreateOptions{})
		s.Require().NoError(err)

		result, err := r.Reconcile(context.Background(), ReconcileRequest{
			Name:      prePullImageSecond.Name,
			Namespace: prePullImageSecond.Namespace,
		})
		s.Require().NoError(err)
		s.Require().Equal(ControllerResult{}, result)

		daemonSets, err := s.k8sClient.AppsV1().DaemonSets(prePullImageSecond.Namespace).List(context.Background(), metav1.ListOptions{})
		s.Require().NoError(err)
		s.Require().Len(daemonSets.Items, 1, "daemonset should be updated")
		s.validateDaemonSetContainsOnlyProperContainers(&daemonSets.Items[0])
	}

	// Update first image - change to has same image as second
	prePullImageFirst.Spec.Image = "my-image-2:v0.0.2"
	prePullImageFirst, err = s.imClient.ImagesV1().PrePullImages(prePullImageFirst.Namespace).Update(context.Background(), prePullImageFirst, metav1.UpdateOptions{})
	s.Require().NoError(err)

	// Reconcile updated object
	result, err := r.Reconcile(context.Background(), ReconcileRequest{
		Name:      prePullImageFirst.Name,
		Namespace: prePullImageFirst.Namespace,
	})
	s.Require().NoError(err)
	s.Require().Equal(ControllerResult{}, result)

	// Assert updated are commited
	daemonSets, err := s.k8sClient.AppsV1().DaemonSets(prePullImageFirst.Namespace).List(context.Background(), metav1.ListOptions{})
	s.Require().NoError(err)
	s.Require().Len(daemonSets.Items, 1, "daemonset should be updated")
	s.validateDaemonSetContainsOnlyProperContainers(&daemonSets.Items[0])
}

func (s *PrePullImageReconcillerTestSuite) TestWhenObjectDeletedDaemonsetAlsoUpdated() {
	r := prePullImageReconciller{
		reconcileConfig: s.reconcillerConfig,
		kubeClient:      s.k8sClient,
		imClient:        s.imClient,
	}

	// Prepare stage
	var (
		prePullImageFirst, prePullImageSecond *imagesv1.PrePullImage
		err                                   error
	)
	{
		namespace := "default"
		prePullImageFirst, err = s.imClient.ImagesV1().PrePullImages(namespace).Create(context.Background(), &imagesv1.PrePullImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pre-pull-1",
				Namespace: namespace,
			},
			Spec: imagesv1.PrePullImageSpec{
				Image: "my-image:v0.0.1",
			},
		}, metav1.CreateOptions{})
		s.Require().NoError(err)

		result, err := r.Reconcile(context.Background(), ReconcileRequest{
			Name:      prePullImageFirst.Name,
			Namespace: prePullImageFirst.Namespace,
		})
		s.Require().NoError(err)
		s.Require().Equal(ControllerResult{}, result)

		// Refresh object, we need to update from latest changes
		prePullImageFirst, err = s.imClient.ImagesV1().PrePullImages(prePullImageFirst.Namespace).Get(context.Background(), prePullImageFirst.Name, metav1.GetOptions{})
		s.Require().NoError(err)

		daemonSets, err := s.k8sClient.AppsV1().DaemonSets(prePullImageFirst.Namespace).List(context.Background(), metav1.ListOptions{})
		s.Require().NoError(err)
		s.Require().Len(daemonSets.Items, 1, "daemonset should be created")
		s.validateDaemonSetContainsOnlyProperContainers(&daemonSets.Items[0])
	}

	// Prepare stage - second object with different image
	{
		namespace := "default"
		prePullImageSecond, err = s.imClient.ImagesV1().PrePullImages(namespace).Create(context.Background(), &imagesv1.PrePullImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pre-pull-2",
				Namespace: namespace,
			},
			Spec: imagesv1.PrePullImageSpec{
				Image: "my-image-2:v0.0.1",
			},
		}, metav1.CreateOptions{})
		s.Require().NoError(err)

		result, err := r.Reconcile(context.Background(), ReconcileRequest{
			Name:      prePullImageSecond.Name,
			Namespace: prePullImageSecond.Namespace,
		})
		s.Require().NoError(err)
		s.Require().Equal(ControllerResult{}, result)

		// Refresh object, we need to update from latest changes
		prePullImageSecond, err = s.imClient.ImagesV1().PrePullImages(prePullImageSecond.Namespace).Get(context.Background(), prePullImageSecond.Name, metav1.GetOptions{})
		s.Require().NoError(err)

		daemonSets, err := s.k8sClient.AppsV1().DaemonSets(prePullImageSecond.Namespace).List(context.Background(), metav1.ListOptions{})
		s.Require().NoError(err)
		s.Require().Len(daemonSets.Items, 1, "daemonset should be updated")
		s.validateDaemonSetContainsOnlyProperContainers(&daemonSets.Items[0])
	}

	// Mark object for deletion
	now := metav1.Now()
	prePullImageFirst.DeletionTimestamp = &now
	prePullImageFirst, err = s.imClient.ImagesV1().PrePullImages(prePullImageFirst.Namespace).Update(context.Background(), prePullImageFirst, metav1.UpdateOptions{})
	s.Require().NoError(err)

	result, err := r.Reconcile(context.Background(), ReconcileRequest{
		Name:      prePullImageFirst.Name,
		Namespace: prePullImageFirst.Namespace,
	})
	s.Require().NoError(err)
	s.Require().Equal(ControllerResult{}, result)

	// Assert updated are commited
	daemonSets, err := s.k8sClient.AppsV1().DaemonSets(prePullImageFirst.Namespace).List(context.Background(), metav1.ListOptions{})
	s.Require().NoError(err)
	s.Require().Len(daemonSets.Items, 1, "daemonset should be updated")
	s.validateDaemonSetContainsOnlyProperContainers(&daemonSets.Items[0])
}
