package autopilot

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/webdevopos/azure-k8s-autopilot/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"strings"
	"time"
)

type (
	k8sPatchStringValue struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value string `json:"value"`
	}
	k8sPatchRemove struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
	}
)

func (r *AzureK8sAutopilot) getK8sNodeList() (nodeList *k8s.NodeList, err error) {
	ctx := context.Background()

	opts := metav1.ListOptions{}
	opts.LabelSelector = r.K8s.NodeLabelSelector
	list, k8sError := r.k8sClient.CoreV1().Nodes().List(ctx, opts)
	if k8sError != nil {
		err = k8sError
		return
	}

	nodeList = &k8s.NodeList{list}

	// fetch all nodes
	for {
		if list.RemainingItemCount == nil || *list.RemainingItemCount == 0 {
			break
		}

		opts.Continue = list.Continue

		remainList, k8sError := r.k8sClient.CoreV1().Nodes().List(ctx, opts)
		if k8sError != nil {
			err = k8sError
			return
		}

		list.Continue = remainList.Continue
		list.RemainingItemCount = remainList.RemainingItemCount
		nodeList.Items = append(nodeList.Items, remainList.Items...)
	}

	return
}

func (r *AzureK8sAutopilot) k8sSetNodeLockAnnotation(node *k8s.Node, annotationName string, dur time.Duration) (err error) {
	lockValue := time.Now().Add(dur).Format(time.RFC3339)

	annotationName = strings.ReplaceAll(annotationName, "~", "~0")
	annotationName = strings.ReplaceAll(annotationName, "/", "~1")

	patch := []k8sPatchStringValue{{
		Op:    "replace",
		Path:  fmt.Sprintf("/metadata/annotations/%s", annotationName),
		Value: lockValue,
	}}

	patchBytes, patchErr := json.Marshal(patch)
	if patchErr != nil {
		err = patchErr
		return
	}

	_, k8sError := r.k8sClient.CoreV1().Nodes().Patch(r.ctx, node.Name, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
	if k8sError != nil {
		err = k8sError
		return
	}

	return
}

func (r *AzureK8sAutopilot) k8sRemoveNodeLockAnnotation(node *k8s.Node, annotationName string) (err error) {
	annotationName = strings.ReplaceAll(annotationName, "~", "~0")
	annotationName = strings.ReplaceAll(annotationName, "/", "~1")

	patch := []k8sPatchRemove{{
		Op:    "remove",
		Path:  fmt.Sprintf("/metadata/annotations/%s", annotationName),
	}}

	patchBytes, patchErr := json.Marshal(patch)
	if patchErr != nil {
		err = patchErr
		return
	}
	fmt.Println(string(patchBytes))

	_, k8sError := r.k8sClient.CoreV1().Nodes().Patch(r.ctx, node.Name, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
	if k8sError != nil {
		err = k8sError
		return
	}

	return
}

func (r *AzureK8sAutopilot) k8sGetNodeLockAnnotation(node *k8s.Node, annotationName string) (dur *time.Duration, exists bool) {
	if val, ok := node.Annotations[annotationName]; ok {
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
