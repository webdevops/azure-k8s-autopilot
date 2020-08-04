package k8s

import (
	v1 "k8s.io/api/core/v1"
	"strings"
	"time"
)

type (
	Node struct {
		*v1.Node
	}
)

// check if node is an Azure node
func (n *Node) IsAzureProvider() bool {
	return strings.HasPrefix(n.Spec.ProviderID, "azure://")
}

// detect if node is ready/healthy
func (n *Node) GetHealthStatus() (status bool, lastHeartbeat time.Time) {
	for _, condition := range n.Status.Conditions {
		if condition.Type == "Ready" && condition.Status == "True" {
			status = true
			lastHeartbeat = condition.LastHeartbeatTime.Time
		} else {
			status = false
			lastHeartbeat = condition.LastHeartbeatTime.Time
		}
	}
	return
}
