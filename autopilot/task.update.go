package autopilot

import (
	"fmt"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	log "github.com/sirupsen/logrus"
	"github.com/webdevopos/azure-k8s-autopilot/k8s"
)

func (r *AzureK8sAutopilot) upgradeRun(contextLogger *log.Entry) {
	nodeList, err := r.getK8sNodeList()
	if err != nil {
		contextLogger.Errorf("unable to fetch K8s Node list: %v", err.Error())
		return
	}

	r.syncNodeLockCache(contextLogger, nodeList, r.Update.NodeLockAnnotation, r.nodeUpdateLock)

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
					if r.Update.Limit > 0 && r.nodeUpdateLock.ItemCount() >= r.Update.Limit {
						vmssInstanceContextLogger.Infof("detected updateable node %s, skipping due to concurrent update limit", node.Name)
						continue vmssInstanceLoop
					}

					vmssInstanceContextLogger.Infof("found updateable instance %v/%v", vmssInfo.VMScaleSetName, *vmssInstance.InstanceID)

					if r.DryRun {
						vmssInstanceContextLogger.Infof("node %s update skipped, dry run", node.Name)
						r.nodeUpdateLock.Add(node.Name, true, *r.Update.LockDuration) //nolint:golint,errcheck
						continue vmssInstanceLoop
					}

					// TODO: need to drain node

					//err = r.azureVmssInstanceUpdate(vmssInstanceContextLogger, *nodeInfo)
					fmt.Println(nodeInfo)
					err = nil
					r.prometheus.update.count.WithLabelValues().Inc()

					if err != nil {
						vmssInstanceContextLogger.Errorf("node %s upgrade failed: %s", node.Name, err.Error())
						r.nodeUpdateLock.Add(node.Name, true, *r.Update.LockDurationError) //nolint:golint,errcheck
						if k8sErr := r.k8sSetNodeLockAnnotation(node, r.Update.NodeLockAnnotation, *r.Update.LockDurationError); k8sErr != nil {
							vmssInstanceContextLogger.Error(k8sErr)
						}
						continue vmssInstanceLoop
					} else {
						// lock vm for next redeploy, can take up to 15 mins
						r.nodeUpdateLock.Add(node.Name, true, *r.Update.LockDuration) //nolint:golint,errcheck
						if k8sErr := r.k8sSetNodeLockAnnotation(node, r.Update.NodeLockAnnotation, *r.Update.LockDuration); k8sErr != nil {
							vmssInstanceContextLogger.Error(k8sErr)
						}
						vmssInstanceContextLogger.Infof("node %s successfully scheduled for update", node.Name)
					}
				}
			}
		}
	}
}
