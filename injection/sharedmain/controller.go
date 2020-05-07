/*
Copyright 2020 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package sharedmain

import (
	"context"
	"fmt"
	"net/http"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/injection/wire"
	kle "knative.dev/pkg/leaderelection"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/profiling"
	"knative.dev/pkg/system"
	"knative.dev/pkg/version"
)

type ComponentName string

type Controller struct {
	component            string
	factories            *wire.InformerFactories
	informers            []controller.Informer
	controllers          []*controller.Impl
	kubeclient           kubernetes.Interface
	cmw                  configmap.Watcher
	observabilityHandler *ObservabilityHandler
	logger               *zap.SugaredLogger
}

func NewController(
	component ComponentName,
	factories *wire.InformerFactories,
	controllers []*controller.Impl,
	kubeclient kubernetes.Interface,
	cmw configmap.Watcher,
	observabilityHandler *ObservabilityHandler,
	logger *zap.SugaredLogger,
) *Controller {
	return &Controller{
		component:            string(component),
		factories:            factories,
		controllers:          controllers,
		kubeclient:           kubeclient,
		cmw:                  cmw,
		observabilityHandler: observabilityHandler,
		logger:               logger,
	}
}

func (c *Controller) Start(ctx context.Context) error {
	defer flush(c.logger)
	ctx = logging.WithLogger(ctx, c.logger)

	MemStatsOrDie(ctx)

	checkK8sClientMinimumVersionOrDie(c.kubeclient, c.logger)

	if err := c.observabilityHandler.Start(ctx); err != nil {
		return err
	}

	// Set up leader election config
	leaderElectionConfig, err := getLeaderElectionConfig(c.kubeclient)
	if err != nil {
		return fmt.Errorf("error loading leader election configuration: %w", err)
	}
	leConfig := leaderElectionConfig.GetComponentConfig(c.component)

	if !leConfig.LeaderElect {
		c.logger.Infof("%v will not run in leader-elected mode", c.component)
		c.run(ctx)
	} else {
		RunLeaderElected(ctx, c.logger, c.run, leConfig)
	}
	return nil
}

func (c *Controller) run(ctx context.Context) {
	c.logger.Info("Starting configuration manager...")
	if err := c.cmw.Start(ctx.Done()); err != nil {
		c.logger.Fatalw("Failed to start configuration manager", zap.Error(err))
	}
	c.logger.Info("Starting informers...")
	if err := c.factories.Start(ctx.Done()); err != nil {
		c.logger.Fatalw("Failed to start informer factories", zap.Error(err))
	}
	if err := controller.StartInformers(ctx.Done(), c.informers...); err != nil {
		c.logger.Fatalw("Failed to start informers", zap.Error(err))
	}
	c.logger.Info("Starting controllers...")
	go controller.StartAll(ctx, c.controllers...)

	<-ctx.Done()
}

// CheckK8sClientMinimumVersionOrDie checks that the hosting Kubernetes cluster
// is at least the minimum allowable version or dies by calling log.Fatalf.
func checkK8sClientMinimumVersionOrDie(kc kubernetes.Interface, logger *zap.SugaredLogger) {
	if err := version.CheckMinimumVersion(kc.Discovery()); err != nil {
		logger.Fatalw("Version check failed", zap.Error(err))
	}
}

// GetLeaderElectionConfig gets the leader election config.
func getLeaderElectionConfig(kc kubernetes.Interface) (*kle.Config, error) {
	leaderElectionConfigMap, err := kc.CoreV1().ConfigMaps(system.Namespace()).Get(kle.ConfigMapName(), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return kle.NewConfigFromConfigMap(nil)
	} else if err != nil {
		return nil, err
	}
	return kle.NewConfigFromConfigMap(leaderElectionConfigMap)
}

// ObservabilityHandler starts a profiling server and watches an observability configmap for
// profiling config changes.
type ObservabilityHandler struct {
	component  ComponentName
	kubeclient kubernetes.Interface
	cmw        configmap.Watcher
	logger     *zap.SugaredLogger
}

func NewObservabilityHandler(
	component ComponentName,
	kubeclient kubernetes.Interface,
	cmw configmap.Watcher,
	logger *zap.SugaredLogger,
) *ObservabilityHandler {
	return &ObservabilityHandler{
		component:  component,
		kubeclient: kubeclient,
		cmw:        cmw,
		logger:     logger,
	}
}

// Start starts a profiling server which will run until ctx is cancelled or an unrecoverable error
// occurs.
func (w *ObservabilityHandler) Start(ctx context.Context) error {
	profilingHandler := profiling.NewHandler(w.logger, false)
	profilingServer := profiling.NewServer(profilingHandler)

	if err := watchObservabilityConfig(w.kubeclient, w.cmw, profilingHandler, w.logger, string(w.component)); err != nil {
		return fmt.Errorf("error watching observability configmap: %w", err)
	}

	eg, egCtx := errgroup.WithContext(ctx)
	eg.Go(profilingServer.ListenAndServe)
	go func() {
		// This will block until either a signal arrives or one of the grouped functions
		// returns an error.
		<-egCtx.Done()

		profilingServer.Shutdown(context.Background())
		if err := eg.Wait(); err != nil && err != http.ErrServerClosed {
			w.logger.Errorw("Error while running server", zap.Error(err))
		}
	}()
	return nil
}
