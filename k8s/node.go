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

func (n *Node) IsAzureProvider() bool {
	return strings.HasPrefix(n.Spec.ProviderID, "azure://")
}

func (n *Node) GetHealthStatus() (status bool, lastHeartbeat time.Time) {
	// detect if node is ready/healthy
	for _, condition := range n.Status.Conditions {
		if condition.Type == "Ready" && condition.Status != "True" {
			status = false
			lastHeartbeat = condition.LastHeartbeatTime.Time
		}
	}
	return
}
