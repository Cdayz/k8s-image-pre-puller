package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-logr/zerologr"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog/v2"

	"github.com/Cdayz/k8s-image-pre-puller/internal/config"
	"github.com/Cdayz/k8s-image-pre-puller/internal/controller"
	"github.com/Cdayz/k8s-image-pre-puller/internal/health"
	imclientset "github.com/Cdayz/k8s-image-pre-puller/pkg/client/clientset/versioned"

	_ "go.uber.org/automaxprocs"
)

var Version = "dev"

var (
	kubeconfig string
	configFile string
)

func init() {
	klog.InitFlags(nil)

	flag.StringVar(&kubeconfig, "kubeconfig", "", "optional path to kubeconfig")
	flag.StringVar(&configFile, "config", "", "path to controller config file")
}

func getClientConfig() (*rest.Config, error) {
	var (
		k8sConfig *rest.Config
		err       error
	)

	if kubeconfig != "" {
		k8sConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("build config from kubeconfig: %w", err)
		}
		return k8sConfig, nil
	}

	k8sConfig, err = rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("get in-cluster k8s config: %w", err)
	}

	return k8sConfig, nil
}

func configureLogging() {
	klog.SetLogger(zerologr.New(&log.Logger))
}

func main() {
	flag.Parse()

	configureLogging()

	controllerCfg, err := config.ParseConfig(configFile)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot read controller config")
	}

	k8sCfg, err := getClientConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("cannot build k8s client config")
	}

	k8sClient, err := kubernetes.NewForConfig(k8sCfg)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot build k8s client")
	}

	imClient, err := imclientset.NewForConfig(k8sCfg)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot build images client")
	}

	prePullImageControllerConfig, err := controllerCfg.PrePullImageReconcilerConfig.ToControllerConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("cannot build prePullImageControllerConfig")
	}

	var (
		ctxWithLogger = log.Logger.WithContext(context.Background())
		wg            = sync.WaitGroup{}
		probes        = &Health{
			Liveness:  health.NewProbe("liveness"),
			Readiness: health.NewProbe("readiness"),
		}
	)

	ctx, cancel := signal.NotifyContext(ctxWithLogger, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	probes.Liveness.Enable()
	probes.Readiness.Disable()

	prePullImageController := controller.NewPrePullImageController(
		k8sClient,
		imClient,
		informers.NewSharedInformerFactory(k8sClient, 0),
		controllerCfg.WorkersCount,
		prePullImageControllerConfig,
	)

	wg.Add(1)
	go func() {
		defer wg.Done()

		controllerCtx, cancelController := context.WithCancel(ctx)
		leaderelection.RunOrDie(controllerCtx, leaderelection.LeaderElectionConfig{
			Lock: &resourcelock.LeaseLock{
				LeaseMeta: metav1.ObjectMeta{
					Name:      controllerCfg.LeaderElectionConfig.Name,
					Namespace: controllerCfg.LeaderElectionConfig.Namespace,
				},
				Client: k8sClient.CoordinationV1(),
				LockConfig: resourcelock.ResourceLockConfig{
					Identity: controllerCfg.LeaderElectionConfig.InstanceID,
				},
			},
			ReleaseOnCancel: true,
			LeaseDuration:   60 * time.Second,
			RenewDeadline:   15 * time.Second,
			RetryPeriod:     5 * time.Second,
			Callbacks: leaderelection.LeaderCallbacks{
				OnStartedLeading: func(ctx context.Context) {
					probes.Readiness.Enable()
					prePullImageController.Run(ctx)
				},
				OnStoppedLeading: func() {
					probes.Readiness.Disable()
					cancelController()
				},
			},
		})
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		RunHealthchecks(ctx, controllerCfg, probes)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		RunMetrics(ctx, controllerCfg)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		RunProfiling(ctx, controllerCfg)
	}()

	wg.Wait()
}

type Health struct {
	Liveness  *health.Probe
	Readiness *health.Probe
}

func RunHealthchecks(ctx context.Context, cfg *config.Config, probes *Health) {
	router := http.NewServeMux()
	router.Handle(cfg.Health.LivenessPath, probes.Liveness)
	router.Handle(cfg.Health.ReadinessPath, probes.Readiness)

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Health.Port)
	healthzServer := &http.Server{
		Handler: router,
		Addr:    addr,
	}

	go func() {
		log.Info().Msgf("Starting healthchecks at %s", addr)
		if err := healthzServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("healthcheck server error")
		}
	}()

	<-ctx.Done()
	log.Info().Msg("healthchecks shutdown")
}

func RunProfiling(ctx context.Context, cfg *config.Config) {
	if !cfg.Pprof.EnableServerPprof {
		return
	}

	pprofMux := http.NewServeMux()
	pprofMux.HandleFunc("/debug/pprof/", pprof.Index)
	pprofMux.HandleFunc("/debug/pprof/{action}", pprof.Index)
	pprofMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Pprof.Port)
	pprofServer := http.Server{
		Handler: pprofMux,
		Addr:    addr,
	}

	go func() {
		log.Info().Msgf("Starting pprof at %s", addr)
		if err := pprofServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("pprof server error")
		}
	}()

	<-ctx.Done()
	log.Info().Msg("pprof shutdown")
}

func RunMetrics(ctx context.Context, cfg *config.Config) {
	router := http.NewServeMux()
	router.Handle(cfg.Metrics.Path, promhttp.Handler())

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Metrics.Port)
	metricsServer := &http.Server{
		Handler: router,
		Addr:    addr,
	}

	go func() {
		log.Info().Msgf("Starting metrics at %s", addr)
		if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("metrics server error")
		}
	}()

	<-ctx.Done()
	log.Info().Msg("metrics shutdown")
}
