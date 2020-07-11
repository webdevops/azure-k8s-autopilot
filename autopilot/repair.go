package autopilot

import (
	"context"
	"errors"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	log "github.com/sirupsen/logrus"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"strings"
	"time"
)

func (r *AzureK8sAutopilot) repairRun() {
	nodeList, err := r.getK8sNodeList()

	if err != nil {
		log.Errorf("unable to fetch K8s Node list: %v", err.Error())
		return
	}

	repairThresholdSeconds := r.NotReadyThreshold.Seconds()

	r.nodeCache.DeleteExpired()

	log.Debugf("found %v nodes in cluster (%v in locked state)", len(nodeList.Items), r.nodeCache.ItemCount())

nodeLoop:
	for _, node := range nodeList.Items {
		log.Debugf("checking node %v", node.Name)
		r.prometheus.nodeStatus.WithLabelValues(node.Name).Set(0)

		// detect if node is ready/healthy
		nodeIsHealthy := true
		nodeLastHeartbeatAge := float64(0)
		nodeLastHeartbeat := "<unknown>"
		for _, condition := range node.Status.Conditions {
			if condition.Type == "Ready" && condition.Status != "True" {
				nodeIsHealthy = false
				nodeLastHeartbeat = condition.LastHeartbeatTime.Time.String()
				nodeLastHeartbeatAge = time.Now().Sub(condition.LastHeartbeatTime.Time).Seconds()
			}
		}

		if !nodeIsHealthy {
			// node is NOT healthy

			// ignore cordoned nodes, maybe maintenance work in progress
			if node.Spec.Unschedulable {
				log.Infof("detected unhealthy node %s, ignoring because node is cordoned", node.Name)
				continue nodeLoop
			}

			// check if heartbeat already exceeded threshold
			if nodeLastHeartbeatAge < repairThresholdSeconds {
				log.Infof("detected unhealthy node %s (last heartbeat: %s), deadline of %v not reached", node.Name, nodeLastHeartbeat, r.NotReadyThreshold.String())
				continue nodeLoop
			}

			nodeProviderId := node.Spec.ProviderID
			if strings.HasPrefix(nodeProviderId, "azure://") {
				// is an azure node
				r.prometheus.nodeStatus.WithLabelValues(node.Name).Set(1)

				var err error
				ctx := context.Background()

				// redeploy timeout lock
				if _, expiry, exists := r.nodeCache.GetWithExpiration(node.Name); exists == true {
					log.Infof("detected unhealthy node %s (last heartbeat: %s), locked (relased in %v)", node.Name, nodeLastHeartbeat, expiry.Sub(time.Now()))
					continue nodeLoop
				}

				// concurrency repair limit
				if r.Limit > 0 && r.nodeCache.ItemCount() >= r.Limit {
					log.Infof("detected unhealthy node %s (last heartbeat: %s), skipping due to concurrent repair limit", node.Name, nodeLastHeartbeat)
					continue nodeLoop
				}

				log.Infof("detected unhealthy node %s (last heartbeat: %s), starting repair", node.Name, nodeLastHeartbeat)

				// parse node informations from provider ID
				nodeInfo, err := r.buildNodeInfo(&node)
				if err != nil {
					log.Errorln(err.Error())
					continue nodeLoop
				}

				if r.DryRun {
					log.Infof("node %s repair skipped, dry run", node.Name)
					r.nodeCache.Add(node.Name, true, *r.LockDuration) //nolint:golint,errcheck
					continue nodeLoop
				}

				if nodeInfo.IsVmss {
					// node is VMSS instance
					err = r.repairAzureVmssInstance(ctx, *nodeInfo)
				} else {
					// node is a VM
					err = r.repairAzureVm(ctx, *nodeInfo)
				}
				r.prometheus.repairCount.WithLabelValues().Inc()

				if err != nil {
					log.Errorf("node %s repair failed: %s", node.Name, err.Error())
					r.nodeCache.Add(node.Name, true, *r.LockDurationError) //nolint:golint,errcheck
					continue nodeLoop
				} else {
					// lock vm for next redeploy, can take up to 15 mins
					r.nodeCache.Add(node.Name, true, *r.LockDuration) //nolint:golint,errcheck
					log.Infof("node %s successfully scheduled for repair", node.Name)
				}
			}
		} else {
			// node is NOT healthy
			log.Debugf("detected healthy node %s", node.Name)
			r.nodeCache.Delete(node.Name)
		}
	}
}

func (r *AzureK8sAutopilot) repairAzureVmssInstance(ctx context.Context, nodeInfo AzureK8sAutopilotNodeAzureInfo) error {
	var err error
	vmssInstanceIds := compute.VirtualMachineScaleSetVMInstanceIDs{
		InstanceIds: &[]string{nodeInfo.VMInstanceID},
	}

	vmssInstanceReimage := compute.VirtualMachineScaleSetReimageParameters{
		InstanceIds: &[]string{nodeInfo.VMInstanceID},
	}

	vmssClient := compute.NewVirtualMachineScaleSetsClient(nodeInfo.Subscription)
	vmssClient.Authorizer = r.azureAuthorizer

	vmssVmClient := compute.NewVirtualMachineScaleSetVMsClient(nodeInfo.Subscription)
	vmssVmClient.Authorizer = r.azureAuthorizer

	// fetch instances
	vmInstance, err := vmssVmClient.Get(ctx, nodeInfo.ResourceGroup, nodeInfo.VMScaleSetName, nodeInfo.VMInstanceID, "")
	if err != nil {
		return err
	}

	// checking vm provision state
	if err := r.checkVmProvisionState(vmInstance.ProvisioningState); err != nil {
		return err
	}

	log.Infof("scheduling Azure VMSS instance for %s: %s", r.Repair.VmssAction, nodeInfo.ProviderId)
	r.sendNotificationf("trigger automatic repair of K8s node %v (action: %v)", nodeInfo.NodeName, r.Repair.VmssAction)

	// trigger repair
	switch r.Repair.VmssAction {
	case "restart":
		_, err = vmssClient.Restart(ctx, nodeInfo.ResourceGroup, nodeInfo.VMScaleSetName, &vmssInstanceIds)
	case "redeploy":
		_, err = vmssClient.Redeploy(ctx, nodeInfo.ResourceGroup, nodeInfo.VMScaleSetName, &vmssInstanceIds)
	case "reimage":
		_, err = vmssClient.Reimage(ctx, nodeInfo.ResourceGroup, nodeInfo.VMScaleSetName, &vmssInstanceReimage)
	}
	return err
}

func (r *AzureK8sAutopilot) repairAzureVm(ctx context.Context, nodeInfo AzureK8sAutopilotNodeAzureInfo) error {
	var err error

	client := compute.NewVirtualMachinesClient(nodeInfo.Subscription)
	client.Authorizer = r.azureAuthorizer

	// fetch instances
	vmInstance, err := client.Get(ctx, nodeInfo.ResourceGroup, nodeInfo.VMname, "")
	if err != nil {
		return err
	}

	// checking vm provision state
	if err := r.checkVmProvisionState(vmInstance.ProvisioningState); err != nil {
		return err
	}

	log.Infof("scheduling Azure VM for %s: %s", r.Repair.VmAction, nodeInfo.ProviderId)
	r.sendNotificationf("trigger automatic repair of K8s node %v (action: %v)", nodeInfo.NodeName, r.Repair.VmAction)

	switch r.Repair.VmAction {
	case "restart":
		_, err = client.Restart(ctx, nodeInfo.ResourceGroup, nodeInfo.VMname)
	case "redeploy":
		_, err = client.Redeploy(ctx, nodeInfo.ResourceGroup, nodeInfo.VMname)
	}
	return err
}

func (r *AzureK8sAutopilot) checkVmProvisionState(provisioningState *string) (err error) {
	if r.Repair.provisioningStateAll || provisioningState == nil {
		return
	}

	// checking vm provision state
	vmProvisionState := strings.ToLower(*provisioningState)
	if !stringArrayContains(r.Repair.ProvisioningState, vmProvisionState) {
		err = errors.New(fmt.Sprintf("VM is in ProvisioningState \"%v\"", vmProvisionState))
	}

	return
}
