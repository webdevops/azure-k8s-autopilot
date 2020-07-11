package autopilot

import (
	"context"
	"errors"
	"fmt"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

func (r *AzureK8sAutopilot) buildNodeInfo(node *v1.Node) (*AzureK8sAutopilotNodeAzureInfo, error) {
	nodeProviderId := node.Spec.ProviderID

	info := AzureK8sAutopilotNodeAzureInfo{}
	info.NodeName = node.Name
	info.NodeProviderId = nodeProviderId
	info.ProviderId = strings.TrimPrefix(nodeProviderId, "azure://")

	// extract Subscription
	if match := azureSubscriptionRegexp.FindStringSubmatch(nodeProviderId); len(match) == 2 {
		info.Subscription = match[1]
	} else {
		return nil, errors.New(fmt.Sprintf("unable to detect Azure Subscription from Node ProviderId (Azure resource ID): %v", nodeProviderId))
	}

	// extract ResourceGroup
	if match := azureResourceGroupRegexp.FindStringSubmatch(nodeProviderId); len(match) == 2 {
		info.ResourceGroup = match[1]
	} else {
		return nil, errors.New(fmt.Sprintf("unable to detect Azure ResourceGroup from Node ProviderId (Azure resource ID): %v", nodeProviderId))
	}

	if strings.Contains(nodeProviderId, "/Microsoft.Compute/virtualMachineScaleSets/") {
		// Is VMSS
		info.IsVmss = true

		// extract VMScaleSetName
		if match := azureVmssNameRegexp.FindStringSubmatch(nodeProviderId); len(match) == 2 {
			info.VMScaleSetName = match[1]
		} else {
			return nil, errors.New(fmt.Sprintf("unable to detect Azure VMScaleSetName from Node ProviderId (Azure resource ID): %v", nodeProviderId))
		}

		// extract VmssInstanceId
		if match := azureVmssInstanceIdRegexp.FindStringSubmatch(nodeProviderId); len(match) == 2 {
			info.VMInstanceID = match[1]
		} else {
			return nil, errors.New(fmt.Sprintf("unable to detect Azure VmssInstanceId from Node ProviderId (Azure resource ID): %v", nodeProviderId))
		}
	} else {
		// Is VM
		info.IsVmss = false

		// extract VMname
		if match := azureVmNameRegexp.FindStringSubmatch(nodeProviderId); len(match) == 2 {
			info.VMname = match[1]
		} else {
			return nil, errors.New(fmt.Sprintf("unable to detect Azure VMname from Node ProviderId (Azure resource ID): %v", nodeProviderId))
		}
	}

	return &info, nil
}

func (r *AzureK8sAutopilot) getK8sNodeList() (*v1.NodeList, error) {
	ctx := context.Background()

	opts := metav1.ListOptions{}
	opts.LabelSelector = r.K8s.NodeLabelSelector
	list, err := r.k8sClient.CoreV1().Nodes().List(ctx, opts)
	if err != nil {
		return list, err
	}

	// fetch all nodes
	for {
		if list.RemainingItemCount == nil || *list.RemainingItemCount == 0 {
			break
		}

		opts.Continue = list.Continue

		remainList, err := r.k8sClient.CoreV1().Nodes().List(ctx, opts)
		if err != nil {
			return list, err
		}

		list.Continue = remainList.Continue
		list.RemainingItemCount = remainList.RemainingItemCount
		list.Items = append(list.Items, remainList.Items...)
	}

	return list, nil
}
