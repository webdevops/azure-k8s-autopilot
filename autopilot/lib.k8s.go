package autopilot

import (
	"log/slog"
	"time"

	"github.com/jinzhu/copier"
	"github.com/webdevops/go-common/log/slogger"

	"github.com/webdevopos/azure-k8s-autopilot/config"
	"github.com/webdevopos/azure-k8s-autopilot/k8s"
)

// trigger drain node
func (r *AzureK8sAutopilot) k8sDrainNode(logger *slogger.Logger, node *k8s.Node) error {
	nodeLogger := logger.With(slog.String("node", node.Name))

	if !r.Config.Drain.Enable {
		nodeLogger.Info("not draining node (disabled)")
		return nil
	}

	var drainOpts config.OptsDrain
	if copyErr := copier.Copy(&drainOpts, &r.Config.Drain); copyErr != nil {
		return copyErr
	}

	// first drain
	kubectl := k8s.Kubectl{}
	kubectl.Conf = drainOpts
	kubectl.SetNode(node.Name)
	kubectl.SetLogger(nodeLogger)
	err := kubectl.NodeDrain()

	// retry drain if first one failed
	if err != nil && r.Config.Drain.RetryWithoutEviction {
		drainOpts.DisableEviction = true

		kubectl := k8s.Kubectl{}
		kubectl.Conf = drainOpts
		kubectl.SetNode(node.Name)
		kubectl.SetLogger(nodeLogger)
		err = kubectl.NodeDrain()
	}

	// ignore error
	if err != nil && r.Config.Drain.IgnoreFailure {
		nodeLogger.Warn("failed to drain node, but ignoring error", slog.Any("error", err))
		err = nil
	}

	if err == nil {
		nodeLogger.Info("waiting after drain", slog.Duration("waitTime", r.Config.Drain.WaitAfter))
		time.Sleep(r.Config.Drain.WaitAfter)
	}

	return err
}

// trigger uncordon node
func (r *AzureK8sAutopilot) k8sUncordonNode(contextLogger *slogger.Logger, node *k8s.Node) error {
	kubectl := k8s.Kubectl{}
	kubectl.Conf = r.Config.Drain
	kubectl.SetNode(node.Name)
	kubectl.SetLogger(contextLogger)
	return kubectl.NodeUncordon()
}
