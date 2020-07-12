package autopilot

import (
	"context"
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/webdevopos/azure-k8s-autopilot/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"time"
)


func (r *AzureK8sAutopilot) getK8sNodeList() (nodeList *k8s.NodeList, err error) {
	ctx := context.Background()

	opts := metav1.ListOptions{}
	opts.LabelSelector = r.Config.K8S.NodeLabelSelector
	list, k8sError := r.k8sClient.CoreV1().Nodes().List(ctx, opts)
	if k8sError != nil {
		err = k8sError
		return
	}

	nodeList = &k8s.NodeList{NodeList:list}

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

	patch := []k8s.PatchStringValue{{
		Op:    "replace",
		Path:  fmt.Sprintf("/metadata/annotations/%s", k8s.PatchPathEsacpe(annotationName)),
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
	patch := []k8s.PatchRemove{{
		Op:    "remove",
		Path:  fmt.Sprintf("/metadata/annotations/%s", k8s.PatchPathEsacpe(annotationName)),
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

func (r *AzureK8sAutopilot) k8sDrainNode(contextLogger *log.Entry, node *k8s.Node) error {
	if !r.Config.Drain.Enable {
		contextLogger.Infof("not draining node %s, disable", node.Name)
		return nil
	}

	kubectl := k8s.Kubectl{}
	kubectl.Conf = r.Config.Drain
	kubectl.SetNode(node.Name)
	kubectl.SetLogger(contextLogger)
	return kubectl.NodeDrain()
}

func (r *AzureK8sAutopilot) k8sUncordonNode(contextLogger *log.Entry, node *k8s.Node) error {
	kubectl := k8s.Kubectl{}
	kubectl.Conf = r.Config.Drain
	kubectl.SetNode(node.Name)
	kubectl.SetLogger(contextLogger)
	return kubectl.NodeUncordon()
}
