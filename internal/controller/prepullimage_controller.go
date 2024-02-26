package controller

import (
	"hash"
	"hash/fnv"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	imclientset "github.com/Cdayz/k8s-image-pre-puller/pkg/client/clientset/versioned"
	imscheme "github.com/Cdayz/k8s-image-pre-puller/pkg/client/clientset/versioned/scheme"
	iminformers "github.com/Cdayz/k8s-image-pre-puller/pkg/client/informers/externalversions"
	imageinformer "github.com/Cdayz/k8s-image-pre-puller/pkg/client/informers/externalversions/images/v1"
	imagelister "github.com/Cdayz/k8s-image-pre-puller/pkg/client/listers/images/v1"
)

type PrePullImageController struct {
	reconcileConfig PrePullImageReconcilerConfig

	kubeClient kubernetes.Interface
	imClient   imclientset.Interface

	podInformer          coreinformers.PodInformer
	prePullImageInformer imageinformer.PrePullImageInformer

	informerFactory   informers.SharedInformerFactory
	imInformerFactory iminformers.SharedInformerFactory

	podLister corelisters.PodLister
	podSynced func() bool

	prePullImageLister imagelister.PrePullImageLister
	prePullImageSynced func() bool

	workers   uint32
	queueList []workqueue.RateLimitingInterface

	recorder record.EventRecorder
}

func NewPrePullImageController(
	kubeClient kubernetes.Interface,
	imClient imclientset.Interface,
	informerFactory informers.SharedInformerFactory,
	workers uint32,
	reconcileConfig PrePullImageReconcilerConfig,
) (*PrePullImageController, error) {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(imscheme.Scheme, v1.EventSource{Component: "vc-controller-manager"})

	queueList := make([]workqueue.RateLimitingInterface, workers)
	for i := uint32(0); i < workers; i++ {
		queueList[i] = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	}

	factory := iminformers.NewSharedInformerFactory(imClient, 0)
	prePullImageInformer := factory.Images().V1().PrePullImages()
	prePullImageLister := prePullImageInformer.Lister()
	prePullImageSynced := prePullImageInformer.Informer().HasSynced

	podInformer := informerFactory.Core().V1().Pods()
	podLister := podInformer.Lister()
	podSynced := podInformer.Informer().HasSynced

	cc := PrePullImageController{
		reconcileConfig: reconcileConfig,

		kubeClient: kubeClient,
		imClient:   imClient,

		informerFactory:   informerFactory,
		imInformerFactory: factory,

		prePullImageInformer: prePullImageInformer,
		prePullImageLister:   prePullImageLister,
		prePullImageSynced:   prePullImageSynced,

		podInformer: podInformer,
		podLister:   podLister,
		podSynced:   podSynced,

		workers:   workers,
		queueList: queueList,

		recorder: recorder,
	}

	cc.prePullImageInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    cc.addPrePullImage,
		UpdateFunc: cc.updatePrePullImage,
	})
	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    cc.addPod,
		UpdateFunc: cc.updatePod,
		DeleteFunc: cc.deletePod,
	})

	return &cc, nil
}

// Run start JobController.
func (cc *PrePullImageController) Run(stopCh <-chan struct{}) {
	cc.informerFactory.Start(stopCh)
	cc.imInformerFactory.Start(stopCh)

	for informerType, ok := range cc.informerFactory.WaitForCacheSync(stopCh) {
		if !ok {
			klog.Errorf("caches failed to sync: %v", informerType)
			return
		}
	}

	for informerType, ok := range cc.imInformerFactory.WaitForCacheSync(stopCh) {
		if !ok {
			klog.Errorf("caches failed to sync: %v", informerType)
			return
		}
	}

	var i uint32
	for i = 0; i < cc.workers; i++ {
		go func(num uint32) {
			wait.Until(
				func() {
					cc.worker(num)
				},
				time.Second,
				stopCh)
		}(i)
	}

	klog.Infof("PrePullImageController is running ...... ")
}

func (cc *PrePullImageController) worker(i uint32) {
	klog.Infof("worker %d start ...... ", i)

	for cc.processNextReq(i) {
	}
}

func (cc *PrePullImageController) belongsToThisRoutine(key string, count uint32) bool {
	var hashVal hash.Hash32
	var val uint32

	hashVal = fnv.New32()
	hashVal.Write([]byte(key))

	val = hashVal.Sum32()

	return val%cc.workers == count
}

func (cc *PrePullImageController) getWorkerQueue(key string) workqueue.RateLimitingInterface {
	var hashVal hash.Hash32
	var val uint32

	hashVal = fnv.New32()
	hashVal.Write([]byte(key))

	val = hashVal.Sum32()

	queue := cc.queueList[val%cc.workers]

	return queue
}

func (cc *PrePullImageController) processNextReq(count uint32) bool {
	queue := cc.queueList[count]
	obj, shutdown := queue.Get()
	if shutdown {
		klog.Errorf("Fail to pop item from queue")
		return false
	}

	req := obj.(ReconcileRequest)
	defer queue.Done(req)

	key := req.Key()
	if !cc.belongsToThisRoutine(key, count) {
		klog.Errorf("should not occur The job does not belongs to this routine key:%s, worker:%d...... ", key, count)
		queueLocal := cc.getWorkerQueue(key)
		queueLocal.Add(req)
		return true
	}

	klog.V(3).Infof("Try to handle request <%v>", req)

	err := stub(req) // TODO: Here should be processing of request
	if err != nil {
		klog.V(2).Infof("Failed to handle PrePullImage<%s/%s>: %v", req.Namespace, req.Name, err)
		// If any error, requeue it.
		queue.AddRateLimited(req)
		return true
	}

	// If no error, forget it.
	queue.Forget(req)

	return true
}

func stub(req ReconcileRequest) error {
	return nil
}
