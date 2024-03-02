package config

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/Cdayz/k8s-image-pre-puller/internal/controller"
)

type Resources map[string]string

func (r Resources) ToResourcesList() (corev1.ResourceList, error) {
	rList := corev1.ResourceList{}
	for rName, rVal := range r {
		qty, err := resource.ParseQuantity(rVal)
		if err != nil {
			return nil, fmt.Errorf("parse %q: %w", rName, err)
		}
		rList[corev1.ResourceName(rName)] = qty
	}

	return rList, nil
}

type ContainerResources struct {
	Limits   Resources `yaml:"limits"`
	Requests Resources `yaml:"requests"`
}

type ContainerConfig struct {
	Name      string             `yaml:"name,omitempty"`
	Image     string             `yaml:"image,omitempty"`
	Command   []string           `yaml:"command"`
	Args      []string           `yaml:"args"`
	Resources ContainerResources `yaml:"resources"`
}

type PrePullImageReconcilerConfig struct {
	MainContainer        ContainerConfig `yaml:"main_container"`
	PrePullContainer     ContainerConfig `yaml:"pre_pull_container"`
	ImagePullSecretNames []string        `yaml:"image_pull_secret_names"`
}

func (p *PrePullImageReconcilerConfig) ToControllerConfig() (*controller.PrePullImageReconcilerConfig, error) {
	mainLimits, err := p.MainContainer.Resources.Limits.ToResourcesList()
	if err != nil {
		return nil, fmt.Errorf("main container limits invalid: %w", err)
	}
	mainRequests, err := p.MainContainer.Resources.Requests.ToResourcesList()
	if err != nil {
		return nil, fmt.Errorf("main container requests invalid: %w", err)
	}

	prePullLimits, err := p.PrePullContainer.Resources.Limits.ToResourcesList()
	if err != nil {
		return nil, fmt.Errorf("pre-pull container limits invalid: %w", err)
	}
	prePullRequests, err := p.PrePullContainer.Resources.Requests.ToResourcesList()
	if err != nil {
		return nil, fmt.Errorf("pre-pull container requests invalid: %w", err)
	}

	return &controller.PrePullImageReconcilerConfig{
		MainContainer: controller.ContainerConfig{
			Name:      p.MainContainer.Name,
			Image:     p.MainContainer.Image,
			Command:   p.MainContainer.Command,
			Args:      p.MainContainer.Args,
			Resources: corev1.ResourceRequirements{Limits: mainLimits, Requests: mainRequests},
		},
		PrePullContainer: controller.ContainerConfig{
			Command:   p.PrePullContainer.Command,
			Args:      p.PrePullContainer.Args,
			Resources: corev1.ResourceRequirements{Limits: prePullLimits, Requests: prePullRequests},
		},
		ImagePullSecretNames: p.ImagePullSecretNames,
	}, nil
}

type LoggerConfig struct {
	System string `yaml:"system"`
	Level  string `yaml:"level"`
}

type MetricsConfig struct {
	Path string `yaml:"path"`
	Port int    `yaml:"port"`
}

type PprofConfig struct {
	EnableServerPprof bool `yaml:"enable"`
	Port              int  `yaml:"port"`
}

type HealthConfig struct {
	Port          int    `yaml:"port"`
	LivenessPath  string `yaml:"liveness_path"`
	ReadinessPath string `yaml:"readiness_path"`
}

type LeaderElectionConfig struct {
	InstanceID string `yaml:"instance_id"`
	Name       string `yaml:"lock_name"`
	Namespace  string `yaml:"lock_namespace"`
}

type Config struct {
	LoggerConfig                 LoggerConfig                 `yaml:"logger"`
	Metrics                      MetricsConfig                `yaml:"metrics"`
	Pprof                        PprofConfig                  `yaml:"pprof"`
	Health                       HealthConfig                 `yaml:"health"`
	LeaderElectionConfig         LeaderElectionConfig         `yaml:"leader_election"`
	ResyncPeriod                 time.Duration                `yaml:"resync_period"`
	WorkersCount                 uint32                       `yaml:"worker_count"`
	PrePullImageReconcilerConfig PrePullImageReconcilerConfig `yaml:"pre_pull_image_reconciller"`
}

func ParseConfig(path string) (*Config, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	contents = []byte(os.ExpandEnv(string(contents)))

	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	decoder.KnownFields(true)

	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (cfg *Config) validate() error {
	if cfg.LeaderElectionConfig.InstanceID == "" {
		return fmt.Errorf("leader_election.instance_id should be set")
	}
	if cfg.LeaderElectionConfig.Name == "" {
		return fmt.Errorf("leader_election.lock_name should be set")
	}
	if cfg.LeaderElectionConfig.Namespace == "" {
		return fmt.Errorf("leader_election.lock_namespace should be set")
	}

	if cfg.WorkersCount == 0 {
		return fmt.Errorf("worker_count should be greater than 0")
	}

	if cfg.PrePullImageReconcilerConfig.MainContainer.Name == "" {
		return fmt.Errorf("pre_pull_image_reconciller.main_container.name is empty")
	}
	if cfg.PrePullImageReconcilerConfig.MainContainer.Image == "" {
		return fmt.Errorf("pre_pull_image_reconciller.main_container.image is empty")
	}

	if _, err := cfg.PrePullImageReconcilerConfig.MainContainer.Resources.Requests.ToResourcesList(); err != nil {
		return fmt.Errorf("pre_pull_image_reconciller.main_container.resources.requests cannot be converted into corev1.ResourceList: %w", err)
	}

	if _, err := cfg.PrePullImageReconcilerConfig.MainContainer.Resources.Limits.ToResourcesList(); err != nil {
		return fmt.Errorf("pre_pull_image_reconciller.main_container.resources.limits cannot be converted into corev1.ResourceList: %w", err)
	}

	if _, err := cfg.PrePullImageReconcilerConfig.PrePullContainer.Resources.Requests.ToResourcesList(); err != nil {
		return fmt.Errorf("pre_pull_image_reconciller.pre_pull_container.resources.requests cannot be converted into corev1.ResourceList: %w", err)
	}

	if _, err := cfg.PrePullImageReconcilerConfig.PrePullContainer.Resources.Limits.ToResourcesList(); err != nil {
		return fmt.Errorf("pre_pull_image_reconciller.pre_pull_container.resources.limits cannot be converted into corev1.ResourceList: %w", err)
	}

	return nil
}
