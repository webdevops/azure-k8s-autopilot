package k8s

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	azureSubscriptionRegexp   = regexp.MustCompile("^azure:///subscriptions/([^/]+)/resourceGroups/.*")
	azureResourceGroupRegexp  = regexp.MustCompile("^azure:///subscriptions/[^/]+/resourceGroups/([^/]+)/.*")
	azureVmssNameRegexp       = regexp.MustCompile("/providers/Microsoft.Compute/virtualMachineScaleSets/([^/]+)/.*")
	azureVmssInstanceIdRegexp = regexp.MustCompile("/providers/Microsoft.Compute/virtualMachineScaleSets/[^/]+/virtualMachines/([^/]+)$")
	azureVmNameRegexp         = regexp.MustCompile("/providers/Microsoft.Compute/virtualMachines/([^/]+)$")
)

type (
	NodeInfo struct {
		NodeName       string
		NodeProviderId string
		ProviderId     string

		Subscription  string
		ResourceGroup string

		IsVmss         bool
		VMScaleSetName string
		VMInstanceID   string

		VMname string
	}
)

func ExtractNodeInfo(node *Node) (*NodeInfo, error) {
	nodeProviderId := node.Spec.ProviderID

	info := NodeInfo{}
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
