package autopilot

import (
	"fmt"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	log "github.com/sirupsen/logrus"
	"github.com/webdevopos/azure-k8s-autopilot/k8s"
	"time"
)

func (r *AzureK8sAutopilot) updateRun(contextLogger *log.Entry) {
	nodeList, err := r.getK8sNodeList()
	if err != nil {
		contextLogger.Errorf("unable to fetch K8s Node list: %v", err.Error())
		return
	}

	r.autoUncordonExpiredNodes(contextLogger, nodeList, r.Config.Update.NodeLockAnnotation)
	r.syncNodeLockCache(contextLogger, nodeList, r.Config.Update.NodeLockAnnotation, r.nodeUpdateLock)

	vmssList, err := nodeList.GetAzureVmssList()
	if err != nil {
		contextLogger.Errorf("unable to build K8s Vmss list: %v", err.Error())
		return
	}

vmssLoop:
	for _, vmssInfo := range vmssList {
		vmssContextLogger := contextLogger.WithFields(log.Fields{
			"subscription":  vmssInfo.Subscription,
			"resourceGroup": vmssInfo.ResourceGroup,
			"vmss":          vmssInfo.VMScaleSetName,
		})

		vmssClient := compute.NewVirtualMachineScaleSetsClient(vmssInfo.Subscription)
		vmssClient.Authorizer = r.azureAuthorizer

		vmssVmClient := compute.NewVirtualMachineScaleSetVMsClient(vmssInfo.Subscription)
		vmssVmClient.Authorizer = r.azureAuthorizer

		vmssInstanceList, err := vmssVmClient.List(r.ctx, vmssInfo.ResourceGroup, vmssInfo.VMScaleSetName, "", "", "")
		if err != nil {
			vmssContextLogger.Error(err)
			continue vmssLoop
		}

	vmssInstanceLoop:
		for _, vmssInstance := range vmssInstanceList.Values() {
			if vmssInstance.LatestModelApplied != nil && !*vmssInstance.LatestModelApplied {
				k8sProviderId := fmt.Sprintf(
					"azure://%s",
					*vmssInstance.ID,
				)

				node := nodeList.FindNodeByProviderId(k8sProviderId)
				if node != nil {
					vmssInstanceContextLogger := vmssContextLogger.WithFields(log.Fields{
						"vmssInstance": *vmssInstance.InstanceID,
						"node":         node.Name,
					})

					// parse node informations from provider ID
					nodeInfo, err := k8s.ExtractNodeInfo(node)
					if err != nil {
						vmssInstanceContextLogger.Errorln(err.Error())
						continue vmssInstanceLoop
					}

					// concurrency repair limit
					if r.Config.Update.Limit > 0 && r.nodeUpdateLock.ItemCount() >= r.Config.Update.Limit {
						vmssInstanceContextLogger.Infof("detected updateable node %s, skipping due to concurrent update limit", node.Name)
						continue vmssInstanceLoop
					}

					vmssInstanceContextLogger.Infof("found updateable instance %v/%v", vmssInfo.VMScaleSetName, *vmssInstance.InstanceID)

					if r.DryRun {
						vmssInstanceContextLogger.Infof("node %s update skipped, dry run", node.Name)
						r.updateNodeLock(vmssInstanceContextLogger, node, r.Config.Update.LockDuration)
						continue vmssInstanceLoop
					}

					// drain node
					if err := r.k8sDrainNode(contextLogger, node); err != nil {
						vmssInstanceContextLogger.Errorf("node %s failed to drain: %v", node.Name, err)
						r.updateNodeLock(vmssInstanceContextLogger, node, r.Config.Update.LockDurationError)
						continue vmssInstanceLoop
					}

					// trigger Azure VMSS instance update
					r.prometheus.update.count.WithLabelValues().Inc()
					err = r.azureVmssInstanceUpdate(vmssInstanceContextLogger, *nodeInfo)

					if err != nil {
						r.prometheus.general.errors.WithLabelValues("azure").Inc()
						vmssInstanceContextLogger.Errorf("node %s upgrade failed: %s", node.Name, err.Error())
						r.updateNodeLock(vmssInstanceContextLogger, node, r.Config.Update.LockDurationError)
						continue vmssInstanceLoop
					} else {
						// uncordon node
						if err := r.k8sUncordonNode(contextLogger, node); err != nil {
							vmssInstanceContextLogger.Errorf("node %s failed to uncordon: %v", node.Name, err)
						}

						// lock vm for next redeploy, can take up to 15 mins
						r.updateNodeLock(vmssInstanceContextLogger, node, r.Config.Update.LockDuration)
						vmssInstanceContextLogger.Infof("node %s successfully updated", node.Name)
					}
				}
			}
		}
	}
}

func (r *AzureK8sAutopilot) updateNodeLock(contextLogger *log.Entry, node *k8s.Node, dur time.Duration) {
	r.nodeUpdateLock.Add(node.Name, true, dur) //nolint:golint,errcheck
	if k8sErr := r.k8sSetNodeLockAnnotation(node, r.Config.Update.NodeLockAnnotation, dur); k8sErr != nil {
		contextLogger.Error(k8sErr)
	}
}
