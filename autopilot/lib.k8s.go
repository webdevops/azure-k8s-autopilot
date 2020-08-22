package autopilot

import (
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/webdevopos/azure-k8s-autopilot/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"time"
)

func (r *AzureK8sAutopilot) k8sNodeApplyPatch(node *k8s.Node, patches []k8s.JsonPatch) (err error) {
	patchBytes, patchErr := json.Marshal(patches)
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

func (r *AzureK8sAutopilot) k8sNodeSetLockAnnotation(node *k8s.Node, annotationName string, dur time.Duration) (err error) {
	lockValue := time.Now().Add(dur).Format(time.RFC3339)

	patches := []k8s.JsonPatch{k8s.JsonPatchString{
		Op:    "replace",
		Path:  fmt.Sprintf("/metadata/annotations/%s", k8s.PatchPathEsacpe(annotationName)),
		Value: lockValue,
	}}

	return r.k8sNodeApplyPatch(node, patches)
}

func (r *AzureK8sAutopilot) k8sNodeRemoveAnnotation(node *k8s.Node, annotationName string) (err error) {
	patches := []k8s.JsonPatch{k8s.JsonPatchString{
		Op:   "remove",
		Path: fmt.Sprintf("/metadata/annotations/%s", k8s.PatchPathEsacpe(annotationName)),
	}}

	return r.k8sNodeApplyPatch(node, patches)
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
