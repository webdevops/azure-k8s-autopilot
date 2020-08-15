package autopilot

import (
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/webdevopos/azure-k8s-autopilot/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"time"
)

func (r *AzureK8sAutopilot) startNodeWatch() error {
	// init list
	r.nodeList.lock.Lock()
	r.nodeList.list = map[string]k8s.Node{}
	r.nodeList.lock.Unlock()

	timeout := int64(60 * 60 * 1)

	watchOpts := metav1.ListOptions{
		LabelSelector:  r.Config.K8S.NodeLabelSelector,
		TimeoutSeconds: &timeout,
		Watch:          true,
	}
	nodeWatcher, err := r.k8sClient.CoreV1().Nodes().Watch(r.ctx, watchOpts)
	if err != nil {
		log.Panic(err)
	}
	defer nodeWatcher.Stop()

	for res := range nodeWatcher.ResultChan() {
		switch res.Type {
		case watch.Added:
			r.nodeList.lock.Lock()
			if node, ok := res.Object.(*corev1.Node); ok {
				r.nodeList.list[node.Name] = k8s.Node{Node: node}
			}
			r.nodeList.lock.Unlock()
		case watch.Deleted:
			r.nodeList.lock.Lock()
			if node, ok := res.Object.(*corev1.Node); ok {
				delete(r.nodeList.list, node.Name)
			}
			r.nodeList.lock.Unlock()
		case watch.Modified:
			r.nodeList.lock.Lock()
			if node, ok := res.Object.(*corev1.Node); ok {
				r.nodeList.list[node.Name] = k8s.Node{Node: node}
			}
			r.nodeList.lock.Unlock()
		case watch.Error:
			log.Errorf("go watch error event %v", res.Object)
		}
	}

	return fmt.Errorf("terminated")
}

func (r *AzureK8sAutopilot) getK8sNodeList() (nodeList *k8s.NodeList, err error) {
	nodeList = &k8s.NodeList{}

	r.nodeList.lock.Lock()
	for _, val := range r.nodeList.list {
		node := val
		nodeList.List = append(nodeList.List, &node)
	}
	r.nodeList.lock.Unlock()
	return
}

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
