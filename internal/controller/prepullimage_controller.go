package controller

import (
	"context"
	"hash"
	"hash/fnv"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	imclientset "github.com/Cdayz/k8s-image-pre-puller/pkg/client/clientset/versioned"
	iminformers "github.com/Cdayz/k8s-image-pre-puller/pkg/client/informers/externalversions"
	imageinformer "github.com/Cdayz/k8s-image-pre-puller/pkg/client/informers/externalversions/images/v1"
	imagelister "github.com/Cdayz/k8s-image-pre-puller/pkg/client/listers/images/v1"
)

type PrePullImageController struct {
	kubeClient kubernetes.Interface
	imClient   imclientset.Interface

	imInformerFactory    iminformers.SharedInformerFactory
	prePullImageInformer imageinformer.PrePullImageInformer
	prePullImageLister   imagelister.PrePullImageLister

	workers   uint32
	queueList []workqueue.RateLimitingInterface

	reconciller prePullImageReconciller
}

func NewPrePullImageController(
	kubeClient kubernetes.Interface,
	imClient imclientset.Interface,
	resyncPeriod time.Duration,
	workers uint32,
	reconcileConfig *PrePullImageReconcilerConfig,
) *PrePullImageController {
	queueList := make([]workqueue.RateLimitingInterface, workers)
	for i := uint32(0); i < workers; i++ {
		queueList[i] = workqueue.NewRateLimitingQueue(workqueue.NewItemExponentialFailureRateLimiter(5*time.Millisecond, 300*time.Second))
	}

	factory := iminformers.NewSharedInformerFactory(imClient, resyncPeriod)
	prePullImageInformer := factory.Images().V1().PrePullImages()
	prePullImageLister := prePullImageInformer.Lister()

	cc := PrePullImageController{
		kubeClient: kubeClient,
		imClient:   imClient,

		imInformerFactory: factory,

		prePullImageInformer: prePullImageInformer,
		prePullImageLister:   prePullImageLister,

		workers:   workers,
		queueList: queueList,

		reconciller: prePullImageReconciller{
			reconcileConfig: reconcileConfig,
			kubeClient:      kubeClient,
			imClient:        imClient,
		},
	}

	_, _ = cc.prePullImageInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    cc.addPrePullImage,
		UpdateFunc: cc.updatePrePullImage,
		DeleteFunc: cc.deletePrePullImage,
	})

	return &cc
}

// Run start PrePullImageController.
func (cc *PrePullImageController) Run(ctx context.Context) {
	cc.imInformerFactory.Start(ctx.Done())

	for informerType, ok := range cc.imInformerFactory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			klog.Errorf("caches failed to sync: %v", informerType)
			return
		}
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		<-ctx.Done()
		for _, q := range cc.queueList {
			q.ShutDown()
		}
	}()

	for i := uint32(0); i < cc.workers; i++ {
		wg.Add(1)
		go func(num uint32) {
			defer wg.Done()
			wait.Until(func() { cc.worker(ctx, num) }, time.Second, ctx.Done())
		}(i)
	}

	klog.Infof("PrePullImageController is running ...... ")

	wg.Wait()
}

func (cc *PrePullImageController) worker(ctx context.Context, i uint32) {
	klog.Infof("worker %d start ...... ", i)

	for cc.processNextReq(ctx, i) {
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

func (cc *PrePullImageController) processNextReq(ctx context.Context, count uint32) bool {
	queue := cc.queueList[count]
	obj, shutdown := queue.Get()
	if shutdown {
		klog.Errorf("fail to pop item from queue")
		return false
	}

	req := obj.(ReconcileRequest) //nolint:errcheck // request here SHOULD be this type, if not it's better to panic
	defer queue.Done(req)

	key := req.Key()
	if !cc.belongsToThisRoutine(key, count) {
		klog.Errorf("should not occur this PrePullImage does not belongs to this routine key:%s, worker:%d...... ", key, count)
		queueLocal := cc.getWorkerQueue(key)
		queueLocal.Add(req)
		return true
	}

	klog.V(3).Infof("try to handle request <%v>", req)

	res, err := cc.reconciller.Reconcile(ctx, req)
	if err != nil {
		klog.V(2).Infof("failed to handle PrePullImage<%s/%s>: %v", req.Namespace, req.Name, err)
		// If any error, requeue it.
		queue.AddRateLimited(req)
		return true
	} else if res.Requeue {
		if res.RequeueAfter != 0 {
			queue.AddAfter(req, res.RequeueAfter)
		} else {
			queue.AddRateLimited(req)
		}
		return true
	}

	// If no error, forget it.
	queue.Forget(req)

	return true
}
