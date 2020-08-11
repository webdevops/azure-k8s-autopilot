package autopilot

import (
	"errors"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	log "github.com/sirupsen/logrus"
	"github.com/webdevopos/azure-k8s-autopilot/k8s"
	"strings"
)

func (r *AzureK8sAutopilot) azureVmssInstanceRepair(contextLogger *log.Entry, nodeInfo k8s.NodeInfo) error {
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
	vmInstance, err := vmssVmClient.Get(r.ctx, nodeInfo.ResourceGroup, nodeInfo.VMScaleSetName, nodeInfo.VMInstanceID, "")
	if err != nil {
		return err
	}

	// checking vm provision state
	if err := r.checkVmProvisionState(vmInstance.ProvisioningState); err != nil {
		return err
	}

	contextLogger.Infof("scheduling Azure VMSS instance for %s: %s", r.Config.Repair.AzureVmssAction, nodeInfo.ProviderId)
	r.sendNotificationf("trigger automatic repair of K8s node %v (action: %v)", nodeInfo.NodeName, r.Config.Repair.AzureVmssAction)

	// trigger repair
	switch r.Config.Repair.AzureVmssAction {
	case "restart":
		_, err = vmssClient.Restart(r.ctx, nodeInfo.ResourceGroup, nodeInfo.VMScaleSetName, &vmssInstanceIds)
	case "redeploy":
		_, err = vmssClient.Redeploy(r.ctx, nodeInfo.ResourceGroup, nodeInfo.VMScaleSetName, &vmssInstanceIds)
	case "reimage":
		_, err = vmssClient.Reimage(r.ctx, nodeInfo.ResourceGroup, nodeInfo.VMScaleSetName, &vmssInstanceReimage)
	}
	return err
}

func (r *AzureK8sAutopilot) azureVmRepair(contextLogger *log.Entry, nodeInfo k8s.NodeInfo) error {
	var err error

	client := compute.NewVirtualMachinesClient(nodeInfo.Subscription)
	client.Authorizer = r.azureAuthorizer

	// fetch instances
	vmInstance, err := client.Get(r.ctx, nodeInfo.ResourceGroup, nodeInfo.VMname, "")
	if err != nil {
		return err
	}

	// checking vm provision state
	if err := r.checkVmProvisionState(vmInstance.ProvisioningState); err != nil {
		return err
	}

	contextLogger.Infof("scheduling Azure VM for %s: %s", r.Config.Repair.AzureVmAction, nodeInfo.ProviderId)
	r.sendNotificationf("trigger automatic repair of K8s node %v (action: %v)", nodeInfo.NodeName, r.Config.Repair.AzureVmAction)

	switch r.Config.Repair.AzureVmAction {
	case "restart":
		_, err = client.Restart(r.ctx, nodeInfo.ResourceGroup, nodeInfo.VMname)
	case "redeploy":
		_, err = client.Redeploy(r.ctx, nodeInfo.ResourceGroup, nodeInfo.VMname)
	}
	return err
}

func (r *AzureK8sAutopilot) azureVmssInstanceUpdate(contextLogger *log.Entry, node *k8s.Node, nodeInfo k8s.NodeInfo, doReimage bool) error {
	var err error

	vmssClient := compute.NewVirtualMachineScaleSetsClient(nodeInfo.Subscription)
	vmssClient.Authorizer = r.azureAuthorizer

	vmssVmClient := compute.NewVirtualMachineScaleSetVMsClient(nodeInfo.Subscription)
	vmssVmClient.Authorizer = r.azureAuthorizer

	// fetch instances
	vmInstance, err := vmssVmClient.Get(r.ctx, nodeInfo.ResourceGroup, nodeInfo.VMScaleSetName, nodeInfo.VMInstanceID, "")
	if err != nil {
		return err
	}

	// checking vm provision state
	if err := r.checkVmProvisionState(vmInstance.ProvisioningState); err != nil {
		return err
	}

	r.sendNotificationf("trigger automatic update of K8s node %v", nodeInfo.NodeName)

	// drain node
	if err := r.k8sDrainNode(contextLogger, node); err != nil {
		return fmt.Errorf("node %s failed to drain: %v", node.Name, err)
	}

	// trigger update call
	contextLogger.Info("scheduling Azure VMSS instance update")
	vmssInstanceUpdateOpts := compute.VirtualMachineScaleSetVMInstanceRequiredIDs{
		InstanceIds: &[]string{*vmInstance.InstanceID},
	}
	if future, err := vmssClient.UpdateInstances(r.ctx, nodeInfo.ResourceGroup, nodeInfo.VMScaleSetName, vmssInstanceUpdateOpts); err == nil {
		// wait for update
		if err := future.WaitForCompletionRef(r.ctx, vmssClient.Client); err != nil {
			return err
		}
	} else {
		return err
	}

	// trigger reimage call
	if doReimage {
		contextLogger.Info("scheduling Azure VMSS instance reimage")
		vmssInstanceReimage := compute.VirtualMachineScaleSetReimageParameters{
			InstanceIds: &[]string{nodeInfo.VMInstanceID},
		}
		if future, err := vmssClient.Reimage(r.ctx, nodeInfo.ResourceGroup, nodeInfo.VMScaleSetName, &vmssInstanceReimage); err == nil {
			// wait for update
			if err := future.WaitForCompletionRef(r.ctx, vmssClient.Client); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	return nil
}

func (r *AzureK8sAutopilot) checkVmProvisionState(provisioningState *string) (err error) {
	if r.Config.Repair.ProvisioningStateAll || provisioningState == nil {
		return
	}

	// checking vm provision state
	vmProvisionState := strings.ToLower(*provisioningState)
	if !stringArrayContains(r.Config.Repair.ProvisioningState, vmProvisionState) {
		err = errors.New(fmt.Sprintf("VM is in ProvisioningState \"%v\"", vmProvisionState))
	}

	return
}
