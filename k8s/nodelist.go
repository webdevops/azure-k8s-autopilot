package k8s

import (
	"fmt"
)

type (
	NodeList struct {
		List []*Node
	}
)

func (n *NodeList) GetNodes() (list []*Node) {
	return n.List
}

func (n *NodeList) GetAzureVmssList() (vmssList map[string]*NodeInfo, err error) {
	vmssList = map[string]*NodeInfo{}

	for _, node := range n.GetNodes() {
		if node.IsAzureProvider() {
			// parse node informations from provider ID
			nodeInfo, parseErr := ExtractNodeInfo(node)
			if parseErr != nil {
				err = parseErr
				return
			}

			if nodeInfo.IsVmss {
				vmssKey := fmt.Sprintf(
					"%s/%s/%s",
					nodeInfo.Subscription,
					nodeInfo.ResourceGroup,
					nodeInfo.VMScaleSetName,
				)
				vmssList[vmssKey] = nodeInfo
			}
		}
	}

	return
}

func (n *NodeList) FindNodeByProviderId(providerId string) (ret *Node) {
	for _, node := range n.List {
		if node.Spec.ProviderID == providerId {
			ret = node
			break
		}
	}
	return
}
