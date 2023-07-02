package autopilot

import (
	"time"

	"github.com/jinzhu/copier"
	"go.uber.org/zap"

	"github.com/webdevopos/azure-k8s-autopilot/config"
	"github.com/webdevopos/azure-k8s-autopilot/k8s"
)

// trigger drain node
func (r *AzureK8sAutopilot) k8sDrainNode(contextLogger *zap.SugaredLogger, node *k8s.Node) error {
	if !r.Config.Drain.Enable {
		contextLogger.Infof("not draining node %s, disable", node.Name)
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
	kubectl.SetLogger(contextLogger)
	err := kubectl.NodeDrain()

	// retry drain if first one failed
	if err != nil && r.Config.Drain.RetryWithoutEviction {
		drainOpts.DisableEviction = true

		kubectl := k8s.Kubectl{}
		kubectl.Conf = drainOpts
		kubectl.SetNode(node.Name)
		kubectl.SetLogger(contextLogger)
		err = kubectl.NodeDrain()
	}

	// ignore error
	if err != nil && r.Config.Drain.IgnoreFailure {
		contextLogger.Warnf("failed to drain node %s, but ignoring error: %v", node.Name, err.Error())
		err = nil
	}

	if err == nil {
		contextLogger.Infof("waiting %s after drain of node %s", r.Config.Drain.WaitAfter.String(), node.Name)
		time.Sleep(r.Config.Drain.WaitAfter)
	}

	return err
}

// trigger uncordon node
func (r *AzureK8sAutopilot) k8sUncordonNode(contextLogger *zap.SugaredLogger, node *k8s.Node) error {
	kubectl := k8s.Kubectl{}
	kubectl.Conf = r.Config.Drain
	kubectl.SetNode(node.Name)
	kubectl.SetLogger(contextLogger)
	return kubectl.NodeUncordon()
}
