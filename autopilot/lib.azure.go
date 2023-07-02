package autopilot

import (
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"go.uber.org/zap"

	"github.com/webdevopos/azure-k8s-autopilot/k8s"
)

// trigger VMSS repair task
func (r *AzureK8sAutopilot) azureVmssInstanceRepair(contextLogger *zap.SugaredLogger, nodeInfo k8s.NodeInfo) error {
	vmssClient, err := armcompute.NewVirtualMachineScaleSetsClient(nodeInfo.Subscription, r.azureClient.GetCred(), r.azureClient.NewArmClientOptions())
	if err != nil {
		return err
	}

	vmssVmClient, err := armcompute.NewVirtualMachineScaleSetVMsClient(nodeInfo.Subscription, r.azureClient.GetCred(), r.azureClient.NewArmClientOptions())
	if err != nil {
		return err
	}

	// fetch instances
	vmInstance, err := vmssVmClient.Get(r.ctx, nodeInfo.ResourceGroup, nodeInfo.VMScaleSetName, nodeInfo.VMInstanceID, nil)
	if err != nil {
		return err
	}

	// checking vm provision state
	if err := r.checkVmProvisionState(vmInstance.Properties.ProvisioningState); err != nil {
		return err
	}

	contextLogger.Infof("scheduling Azure VMSS instance for %s: %s", r.Config.Repair.AzureVmssAction, nodeInfo.ProviderId)
	r.sendNotificationf("trigger automatic repair of K8s node %v (action: %v)", nodeInfo.NodeName, r.Config.Repair.AzureVmssAction)

	// trigger repair
	switch r.Config.Repair.AzureVmssAction {
	case "restart":
		restartOpts := armcompute.VirtualMachineScaleSetsClientBeginRestartOptions{
			VMInstanceIDs: &armcompute.VirtualMachineScaleSetVMInstanceIDs{
				InstanceIDs: []*string{&nodeInfo.VMInstanceID},
			},
		}
		if future, err := vmssClient.BeginRestart(r.ctx, nodeInfo.ResourceGroup, nodeInfo.VMScaleSetName, &restartOpts); err == nil {
			if _, futureErr := future.PollUntilDone(r.ctx, nil); futureErr != nil {
				return futureErr
			}
		} else {
			return err
		}
	case "redeploy":
		redeployOpts := armcompute.VirtualMachineScaleSetsClientBeginRedeployOptions{
			VMInstanceIDs: &armcompute.VirtualMachineScaleSetVMInstanceIDs{
				InstanceIDs: []*string{&nodeInfo.VMInstanceID},
			},
		}
		if future, err := vmssClient.BeginRedeploy(r.ctx, nodeInfo.ResourceGroup, nodeInfo.VMScaleSetName, &redeployOpts); err == nil {
			if _, futureErr := future.PollUntilDone(r.ctx, nil); futureErr != nil {
				return futureErr
			}
		} else {
			return err
		}
	case "reimage":
		reimageOpts := armcompute.VirtualMachineScaleSetsClientBeginReimageOptions{
			VMScaleSetReimageInput: &armcompute.VirtualMachineScaleSetReimageParameters{
				InstanceIDs: []*string{&nodeInfo.VMInstanceID},
			},
		}
		if future, err := vmssClient.BeginReimage(r.ctx, nodeInfo.ResourceGroup, nodeInfo.VMScaleSetName, &reimageOpts); err == nil {
			if _, futureErr := future.PollUntilDone(r.ctx, nil); futureErr != nil {
				return futureErr
			}
		} else {
			return err
		}
	case "delete":
		deleteOpts := armcompute.VirtualMachineScaleSetsClientBeginDeleteInstancesOptions{}
		vmssInstanceIdsDelete := armcompute.VirtualMachineScaleSetVMInstanceRequiredIDs{
			InstanceIDs: []*string{&nodeInfo.VMInstanceID},
		}

		if future, err := vmssClient.BeginDeleteInstances(r.ctx, nodeInfo.ResourceGroup, nodeInfo.VMScaleSetName, vmssInstanceIdsDelete, &deleteOpts); err == nil {
			if _, futureErr := future.PollUntilDone(r.ctx, nil); futureErr != nil {
				return futureErr
			}
		} else {
			return err
		}
	default:
		return fmt.Errorf("action %s is not valid", r.Config.Repair.AzureVmssAction)
	}

	return nil
}

func (r *AzureK8sAutopilot) azureVmRepair(contextLogger *zap.SugaredLogger, nodeInfo k8s.NodeInfo) error {
	var err error

	client, err := armcompute.NewVirtualMachinesClient(nodeInfo.Subscription, r.azureClient.GetCred(), r.azureClient.NewArmClientOptions())
	if err != nil {
		return err
	}

	// fetch instances
	vmInstance, err := client.Get(r.ctx, nodeInfo.ResourceGroup, nodeInfo.VMname, nil)
	if err != nil {
		return err
	}

	// checking vm provision state
	if err := r.checkVmProvisionState(vmInstance.Properties.ProvisioningState); err != nil {
		return err
	}

	contextLogger.Infof("scheduling Azure VM for %s: %s", r.Config.Repair.AzureVmAction, nodeInfo.ProviderId)
	r.sendNotificationf("trigger automatic repair of K8s node %v (action: %v)", nodeInfo.NodeName, r.Config.Repair.AzureVmAction)

	switch r.Config.Repair.AzureVmAction {
	case "restart":
		if future, err := client.BeginRestart(r.ctx, nodeInfo.ResourceGroup, nodeInfo.VMname, nil); err == nil {
			if _, futureErr := future.PollUntilDone(r.ctx, nil); futureErr != nil {
				return futureErr
			}
		} else {
			return err
		}
	case "redeploy":
		if future, err := client.BeginRedeploy(r.ctx, nodeInfo.ResourceGroup, nodeInfo.VMname, nil); err == nil {
			if _, futureErr := future.PollUntilDone(r.ctx, nil); futureErr != nil {
				return futureErr
			}
		} else {
			return err
		}
	default:
		return fmt.Errorf("action %s is not valid", r.Config.Repair.AzureVmssAction)
	}

	return nil
}

// trigger VMSS instance update
func (r *AzureK8sAutopilot) azureVmssInstanceUpdate(contextLogger *zap.SugaredLogger, node *k8s.Node, nodeInfo k8s.NodeInfo, doReimage bool) error {
	var err error

	vmssClient, err := armcompute.NewVirtualMachineScaleSetsClient(nodeInfo.Subscription, r.azureClient.GetCred(), r.azureClient.NewArmClientOptions())
	if err != nil {
		return err
	}

	vmssVmClient, err := armcompute.NewVirtualMachineScaleSetVMsClient(nodeInfo.Subscription, r.azureClient.GetCred(), r.azureClient.NewArmClientOptions())
	if err != nil {
		return err
	}

	// fetch instances
	vmInstance, err := vmssVmClient.Get(r.ctx, nodeInfo.ResourceGroup, nodeInfo.VMScaleSetName, nodeInfo.VMInstanceID, nil)
	if err != nil {
		return err
	}

	// checking vm provision state
	if err := r.checkVmProvisionState(vmInstance.Properties.ProvisioningState); err != nil {
		return err
	}

	r.sendNotificationf("trigger automatic update of K8s node %v", nodeInfo.NodeName)

	// drain node
	if err := r.k8sDrainNode(contextLogger, node); err != nil {
		return fmt.Errorf("node %s failed to drain: %w", node.Name, err)
	}

	// trigger update call
	contextLogger.Info("scheduling Azure VMSS instance update")
	vmssInstanceUpdateOpts := armcompute.VirtualMachineScaleSetVMInstanceRequiredIDs{
		InstanceIDs: []*string{vmInstance.InstanceID},
	}
	if future, err := vmssClient.BeginUpdateInstances(r.ctx, nodeInfo.ResourceGroup, nodeInfo.VMScaleSetName, vmssInstanceUpdateOpts, nil); err == nil {
		// wait for update
		if _, futureErr := future.PollUntilDone(r.ctx, nil); futureErr != nil {
			return futureErr
		}
	} else {
		return err
	}

	// trigger reimage call
	if doReimage {
		contextLogger.Info("scheduling Azure VMSS instance reimage")
		vmssInstanceReimage := armcompute.VirtualMachineScaleSetsClientBeginRedeployOptions{
			VMInstanceIDs: &armcompute.VirtualMachineScaleSetVMInstanceIDs{
				InstanceIDs: []*string{vmInstance.InstanceID},
			},
		}
		if future, err := vmssClient.BeginRedeploy(r.ctx, nodeInfo.ResourceGroup, nodeInfo.VMScaleSetName, &vmssInstanceReimage); err == nil {
			// wait for update
			if _, futureErr := future.PollUntilDone(r.ctx, nil); futureErr != nil {
				return futureErr
			}
		} else {
			return err
		}
	}

	return nil
}

// check current VM provision state if change is allowed
func (r *AzureK8sAutopilot) checkVmProvisionState(provisioningState *string) (err error) {
	if r.Config.Repair.ProvisioningStateAll || provisioningState == nil {
		return
	}

	// checking vm provision state
	vmProvisionState := strings.ToLower(*provisioningState)
	if !stringArrayContains(r.Config.Repair.ProvisioningState, vmProvisionState) {
		err = fmt.Errorf("VM is in ProvisioningState \"%v\"", vmProvisionState)
	}

	return
}
