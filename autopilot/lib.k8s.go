package autopilot

import (
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/webdevopos/azure-k8s-autopilot/k8s"
)

// trigger drain node
func (r *AzureK8sAutopilot) k8sDrainNode(contextLogger *log.Entry, node *k8s.Node) error {
	if !r.Config.Drain.Enable {
		contextLogger.Infof("not draining node %s, disable", node.Name)
		return nil
	}

	kubectl := k8s.Kubectl{}
	kubectl.Conf = r.Config.Drain
	kubectl.SetNode(node.Name)
	kubectl.SetLogger(contextLogger)
	if err := kubectl.NodeDrain(); err != nil {
		return err
	}

	contextLogger.Infof("waiting %s after drain of node %s", r.Config.Drain.WaitAfter.String(), node.Name)
	time.Sleep(r.Config.Drain.WaitAfter)

	return nil
}

// trigger uncordon node
func (r *AzureK8sAutopilot) k8sUncordonNode(contextLogger *log.Entry, node *k8s.Node) error {
	kubectl := k8s.Kubectl{}
	kubectl.Conf = r.Config.Drain
	kubectl.SetNode(node.Name)
	kubectl.SetLogger(contextLogger)
	return kubectl.NodeUncordon()
}
