package autopilot

import (
	"fmt"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	log "github.com/sirupsen/logrus"
	"github.com/webdevopos/azure-k8s-autopilot/k8s"
	"time"
)

func (r *AzureK8sAutopilot) updateRun(contextLogger *log.Entry) {
	nodeList, err := r.nodeList.NodeListWithAzure()
	if err != nil {
		contextLogger.Errorf("unable to fetch K8s Node list: %v", err.Error())
		return
	}

	// find update candidates
	candidateList := r.updateCollectCandiates(contextLogger, nodeList)
	contextLogger.Infof("found %v nodes (%v upgradable)", len(nodeList), len(candidateList))
	r.prometheus.general.candidateNodes.WithLabelValues("update").Set(float64(len(candidateList)))

	// sanity checks
	failedNodeCount := r.nodeList.NodeCountByProvisionState(string(compute.ProvisioningStateFailed))
	r.prometheus.general.failedNodes.WithLabelValues("provisionState").Set(float64(failedNodeCount))
	if failedNodeCount >= r.Config.Update.FailedThreshold {
		contextLogger.Infof("detected %v failed nodes in cluster, threshold of %v reached, update stopped", failedNodeCount, r.Config.Update.FailedThreshold)
		return
	}

	if !r.DryRun {
		for _, node := range candidateList {
			// concurrency update limit
			if r.Config.Update.Limit > 0 && r.update.nodeLock.ItemCount() >= r.Config.Update.Limit {
				contextLogger.Infof("reached concurrent update lock, skipping node updates")
				break
			}

			// check if self eviction is needed
			if r.checkSelfEviction(node) {
				return
			}

			// parse node information from provider ID
			nodeInfo, err := k8s.ExtractNodeInfo(node)
			if err != nil {
				contextLogger.Error(err)
				continue
			}

			nodeLogger := contextLogger.WithFields(log.Fields{
				"subscription":  nodeInfo.Subscription,
				"resourceGroup": nodeInfo.ResourceGroup,
				"vmss":          nodeInfo.VMScaleSetName,
				"vmssInstance":  nodeInfo.VMInstanceID,
				"node":          node.Name,
			})

			contextLogger.Infof("starting update of node %v", node.Name)
			if err := r.updateNode(nodeLogger, node, nodeInfo); err != nil {
				// update failed
				contextLogger.Error(err)
				r.updateNodeLock(contextLogger, node, r.Config.Update.LockDurationError)
				break
			} else {
				// update successfull
				// lock vm for next redeploy, can take up to 15 mins
				r.updateNodeLock(contextLogger, node, r.Config.Update.LockDuration)
			}
		}
	}
}

func (r *AzureK8sAutopilot) updateCollectCandiates(contextLogger *log.Entry, nodeList []*k8s.Node) (candidateList []*k8s.Node) {
	candidateList = []*k8s.Node{}

	// check if there are ongoing updates (eg. operator was killed while doing updates)
	for _, v := range nodeList {
		node := v

		// check if node is excluded
		if node.AnnotationExists(r.Config.Update.NodeExcludeAnnotation) {
			continue
		}

		if node.AnnotationExists(r.Config.Update.NodeOngoingAnnotation) {
			// annotation found, continue with update of this node and only this node
			candidateList = append(candidateList, node)
			return
		}
	}

	// check if there are nodes which needs updates
	for _, v := range nodeList {
		node := v

		// check if node is excluded
		if node.AnnotationExists(r.Config.Update.NodeExcludeAnnotation) {
			continue
		}

		if node.AzureVmss != nil {
			if node.AzureVmss.LatestModelApplied != nil && !*node.AzureVmss.LatestModelApplied {
				contextLogger.WithField("node", node.Name).Infof("found updatable node")
				candidateList = append(candidateList, node)
			}
		}
	}
	return
}

func (r *AzureK8sAutopilot) updateNode(contextLogger *log.Entry, node *k8s.Node, nodeInfo *k8s.NodeInfo) error {
	// trigger Azure VMSS instance update
	r.prometheus.update.count.WithLabelValues().Inc()

	annotations := map[string]string{
		// mark node as ongoing update
		r.Config.Update.NodeOngoingAnnotation: "true",
		// ensure cluster-autoscaler is not deleting node
		k8s.ClusterAutoscaleScaleDownDisableAnnotation: "true",
	}

	if err := node.AnnotationsSet(annotations); err != nil {
		return err
	}

	doReimage := r.Config.Update.AzureVmssAction == "update+reimage"
	err := r.azureVmssInstanceUpdate(contextLogger, node, *nodeInfo, doReimage)
	if err != nil {
		r.prometheus.general.errors.WithLabelValues("azure").Inc()
		return fmt.Errorf("node upgrade failed: %s", err.Error())
	} else {
		// uncordon node
		if err := r.k8sUncordonNode(contextLogger, node); err != nil {
			return fmt.Errorf("node failed to uncordon: %v", err)
		}
		contextLogger.Info("node successfully updated")

		if err := node.AnnotationRemove(r.Config.Update.NodeOngoingAnnotation); err != nil {
			return err
		}
	}

	return nil
}

func (r *AzureK8sAutopilot) updateNodeLock(contextLogger *log.Entry, node *k8s.Node, dur time.Duration) {
	r.update.nodeLock.Add(node.Name, true, dur) //nolint:golint,errcheck
	if k8sErr := node.AnnotationLockSet(r.Config.Update.NodeLockAnnotation, dur); k8sErr != nil {
		contextLogger.Error(k8sErr)
	}
}
