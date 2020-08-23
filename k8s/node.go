package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"strings"
	"time"
)

type (
	Node struct {
		*v1.Node
		Client    *kubernetes.Clientset
		AzureVmss *compute.VirtualMachineScaleSetVM
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

func (n *Node) AnnotationExists(name string) bool {
	_, exists := n.Annotations[name]
	return exists
}

func (n *Node) AnnotationSet(name, value string) (err error) {
	patches := []JsonPatch{JsonPatchString{
		Op:    "replace",
		Path:  fmt.Sprintf("/metadata/annotations/%s", PatchPathEsacpe(name)),
		Value: value,
	}}

	return n.PatchSetApply(patches)
}


func (n *Node) AnnotationLockSet(name string, dur time.Duration) error {
	return n.AnnotationSet(name, time.Now().Add(dur).Format(time.RFC3339))
}

func (n *Node) AnnotationRemove(name string) (err error) {
	patches := []JsonPatch{JsonPatchString{
		Op:   "remove",
		Path: fmt.Sprintf("/metadata/annotations/%s", PatchPathEsacpe(name)),
	}}

	return n.PatchSetApply(patches)
}

func (n *Node) AnnotationLockCheck(name string) (dur *time.Duration, exists bool) {
	if val, ok := n.Annotations[name]; ok {
		exists = true

		if val != "" {
			lockTime, parseErr := time.Parse(time.RFC3339, val)
			if parseErr != nil {
				return
			}

			lockDuration := lockTime.Sub(time.Now())
			dur = &lockDuration
		}
	}

	return
}

func (n *Node) PatchSetApply(patches []JsonPatch) (err error) {
	ctx := context.Background()
	patchBytes, patchErr := json.Marshal(patches)
	if patchErr != nil {
		err = patchErr
		return
	}

	_, k8sError := n.Client.CoreV1().Nodes().Patch(ctx, n.Name, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
	if k8sError != nil {
		err = k8sError
		return
	}

	return
}
