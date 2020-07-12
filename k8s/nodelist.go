package k8s

import (
	"fmt"
	v1 "k8s.io/api/core/v1"
)

type (
	NodeList struct {
		*v1.NodeList
	}
)

func (n *NodeList) GetNodes() (list []*Node) {
	for _, value := range n.Items {
		node := value
		list = append(list, &Node{Node: &node})
	}
	return
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
	for _, node := range n.Items {
		if node.Spec.ProviderID == providerId {
			ret = &Node{&node}
			break
		}
	}
	return
}
