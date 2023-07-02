package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

const (
	ClusterAutoscaleScaleDownExpireAnnotation  = "cluster-autoscaler.kubernetes.io/scale-down-disabled-expire"
	ClusterAutoscaleScaleDownDisableAnnotation = "cluster-autoscaler.kubernetes.io/scale-down-disabled"
)

type (
	Node struct {
		*v1.Node
		Client    *kubernetes.Clientset
		AzureVmss *armcompute.VirtualMachineScaleSetVM
	}
)

func (n *Node) Cleanup() error {
	if lockDuration, exists := n.AnnotationLockCheck(ClusterAutoscaleScaleDownExpireAnnotation); exists {
		if lockDuration == nil || lockDuration.Seconds() <= 0 {
			if err := n.AnnotationRemove(ClusterAutoscaleScaleDownExpireAnnotation, ClusterAutoscaleScaleDownDisableAnnotation); err != nil {
				return err
			}
		}
	}

	return nil
}

// check if node is an Azure node
func (n *Node) IsAzureProvider() bool {
	providerID := strings.ToLower(n.Spec.ProviderID)
	return strings.HasPrefix(providerID, "azure://")
}

// detect if node is ready/healthy
func (n *Node) GetHealthStatus() (status bool, lastHeartbeat time.Time) {
	for _, condition := range n.Status.Conditions {
		if strings.EqualFold(string(condition.Type), "Ready") && strings.EqualFold(string(condition.Status), "True") {
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

func (n *Node) AnnotationsSet(annotations map[string]string) (err error) {
	patches := []JsonPatch{}

	for name, value := range annotations {
		patches = append(patches, JsonPatchString{
			Op:    "replace",
			Path:  fmt.Sprintf("/metadata/annotations/%s", PatchPathEsacpe(name)),
			Value: value,
		})
	}

	return n.PatchSetApply(patches)
}

func (n *Node) AnnotationLockSet(name string, dur time.Duration, autoscalerScaledownTimeLock time.Duration) error {
	value := time.Now().Add(dur).Format(time.RFC3339)
	patches := []JsonPatch{JsonPatchString{
		Op:    "replace",
		Path:  fmt.Sprintf("/metadata/annotations/%s", PatchPathEsacpe(name)),
		Value: value,
	}}

	// add autoscaler scale-down block
	if autoscalerScaledownTimeLock.Seconds() > 0 {
		// expire annotation
		name = ClusterAutoscaleScaleDownExpireAnnotation
		patches = append(patches, JsonPatchString{
			Op:    "replace",
			Path:  fmt.Sprintf("/metadata/annotations/%s", PatchPathEsacpe(name)),
			Value: time.Now().Add(autoscalerScaledownTimeLock).Format(time.RFC3339),
		})

		// disable scaledown annotation
		name = ClusterAutoscaleScaleDownDisableAnnotation
		patches = append(patches, JsonPatchString{
			Op:    "replace",
			Path:  fmt.Sprintf("/metadata/annotations/%s", PatchPathEsacpe(name)),
			Value: "true",
		})
	}

	return n.PatchSetApply(patches)
}

func (n *Node) AnnotationLockRemove(name string) error {
	patches := []JsonPatch{JsonPatchString{
		Op:   "remove",
		Path: fmt.Sprintf("/metadata/annotations/%s", PatchPathEsacpe(name)),
	}}

	return n.PatchSetApply(patches)
}

func (n *Node) AnnotationRemove(names ...string) (err error) {
	patches := []JsonPatch{}
	for _, name := range names {
		patches = append(patches, JsonPatchString{
			Op:   "remove",
			Path: fmt.Sprintf("/metadata/annotations/%s", PatchPathEsacpe(name)),
		})
	}

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

			lockDuration := time.Since(lockTime)
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
