package sharedmain

import (
	"fmt"
	"time"

	"github.com/google/wire"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"knative.dev/pkg/client/injection/kube/client"
	"knative.dev/pkg/configmap"
	pkgwire "knative.dev/pkg/injection/wire"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/system"
)

var ProviderSet = wire.NewSet(
	NewController,
	pkgwire.NewInformerFactories,
	NewWatchedLogger,
	NewLogger,
	wire.Value([]zap.Option(nil)),
	NewLoggingConfig,
	NewRestCfg,
	NewConfigMapWatch,
	wire.Bind(new(configmap.Watcher), new(*configmap.InformedWatcher)),
	client.ClientSet,
)

// WatchedLogger is a logger setup to update from a logging configmap.
type unwatchedLogger struct {
	logger      *zap.SugaredLogger
	atomicLevel zap.AtomicLevel
}

func NewWatchedLogger(
	component ComponentName,
	uwl unwatchedLogger,
	kc kubernetes.Interface,
	cmw configmap.Watcher,
) *zap.SugaredLogger {
	if _, err := kc.CoreV1().ConfigMaps(system.Namespace()).Get(logging.ConfigMapName(),
		metav1.GetOptions{}); err == nil {
		cmw.Watch(logging.ConfigMapName(), logging.UpdateLevelFromConfigMap(uwl.logger, uwl.atomicLevel, string(component)))
	} else if !apierrors.IsNotFound(err) {
		uwl.logger.With(zap.Error(err)).Fatalf("Error reading ConfigMap %q", logging.ConfigMapName())
	}
	return uwl.logger
}

func NewLogger(config *logging.Config, component ComponentName) unwatchedLogger {
	logger, atomicLevel := logging.NewLoggerFromConfig(config, string(component))
	return unwatchedLogger{
		logger:      logger,
		atomicLevel: atomicLevel,
	}
}

// NewLoggingConfig gets the logging config from either the file system if present
// or via reading a configMap from the API.
func NewLoggingConfig(kc kubernetes.Interface) (*logging.Config, error) {
	var loggingConfigMap *corev1.ConfigMap
	// These timeout and retry interval are set by heuristics.
	// e.g. istio sidecar needs a few seconds to configure the pod network.
	if err := wait.PollImmediate(1*time.Second, 5*time.Second, func() (bool, error) {
		var err error
		loggingConfigMap, err = kc.CoreV1().ConfigMaps(system.Namespace()).Get(logging.ConfigMapName(), metav1.GetOptions{})
		return err == nil || apierrors.IsNotFound(err), nil
	}); err != nil {
		return nil, err
	}
	if loggingConfigMap == nil {
		return logging.NewConfigFromMap(nil)
	}
	return logging.NewConfigFromConfigMap(loggingConfigMap)
}

type RestArgs struct {
	QPS   float32
	Burst int
}

func NewRestCfg(args RestArgs) *rest.Config {
	cfg := ParseAndGetConfigOrDie()
	cfg.QPS = args.QPS
	cfg.Burst = args.Burst
	return cfg
}

// NewConfigMapWatch establishes a watch of the configmaps in the system namespace that are labeled
// to be watched or returns an error.
func NewConfigMapWatch(kc kubernetes.Interface, uwl unwatchedLogger) (*configmap.InformedWatcher, error) {
	// Create ConfigMaps watcher with optional label-based filter.
	var cmLabelReqs []labels.Requirement
	if cmLabel := system.ResourceLabel(); cmLabel != "" {
		req, err := configmap.FilterConfigByLabelExists(cmLabel)
		if err != nil {
			return nil, fmt.Errorf("failed to generate requirement for label: %w", err)
		}

		uwl.logger.Infof("Setting up ConfigMap watcher with label selector %q", req)
		cmLabelReqs = append(cmLabelReqs, *req)
	}
	return configmap.NewInformedWatcher(kc, system.Namespace(), cmLabelReqs...), nil
}
