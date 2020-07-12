package autopilot

import (
	log "github.com/sirupsen/logrus"
	"github.com/webdevopos/azure-k8s-autopilot/k8s"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"strings"
	"time"
)

func (r *AzureK8sAutopilot) repairRun(contextLogger *log.Entry) {
	nodeList, err := r.getK8sNodeList()
	if err != nil {
		contextLogger.Errorf("unable to fetch K8s Node list: %v", err.Error())
		return
	}

	r.syncNodeLockCache(contextLogger, nodeList, r.Config.Repair.NodeLockAnnotation, r.nodeRepairLock)

	repairThresholdSeconds := r.Config.Repair.NotReadyThreshold.Seconds()

	r.nodeRepairLock.DeleteExpired()

	contextLogger.Debugf("found %v nodes in cluster (%v in locked state)", len(nodeList.Items), r.nodeRepairLock.ItemCount())

nodeLoop:
	for _, node := range nodeList.GetNodes() {
		nodeContextLogger := contextLogger.WithField("node", node.Name)

		nodeContextLogger.Debug("checking node")
		r.prometheus.repair.nodeStatus.WithLabelValues(node.Name).Set(0)

		// detect if node is ready/healthy
		nodeIsHealthy := true
		nodeLastHeartbeatAge := float64(0)
		nodeLastHeartbeat := "<unknown>"
		for _, condition := range node.Status.Conditions {
			if condition.Type == "Ready" && condition.Status != "True" {
				nodeIsHealthy = false
				nodeLastHeartbeat = condition.LastHeartbeatTime.Time.String()
				nodeLastHeartbeatAge = time.Now().Sub(condition.LastHeartbeatTime.Time).Seconds()
			}
		}

		if !nodeIsHealthy {
			// node is NOT healthy

			// ignore cordoned nodes, maybe maintenance work in progress
			if node.Spec.Unschedulable {
				nodeContextLogger.Infof("detected unhealthy node %s, ignoring because node is cordoned", node.Name)
				continue nodeLoop
			}

			// check if heartbeat already exceeded threshold
			if nodeLastHeartbeatAge < repairThresholdSeconds {
				nodeContextLogger.Infof("detected unhealthy node %s (last heartbeat: %s), deadline of %v not reached", node.Name, nodeLastHeartbeat, r.Config.Repair.NotReadyThreshold.String())
				continue nodeLoop
			}

			nodeProviderId := node.Spec.ProviderID
			if strings.HasPrefix(nodeProviderId, "azure://") {
				// is an azure node
				r.prometheus.repair.nodeStatus.WithLabelValues(node.Name).Set(1)

				var err error

				// redeploy timeout lock
				if _, expiry, exists := r.nodeRepairLock.GetWithExpiration(node.Name); exists == true {
					nodeContextLogger.Infof("detected unhealthy node %s (last heartbeat: %s), locked (relased in %v)", node.Name, nodeLastHeartbeat, expiry.Sub(time.Now()))
					continue nodeLoop
				}

				// concurrency repair limit
				if r.Config.Repair.Limit > 0 && r.nodeRepairLock.ItemCount() >= r.Config.Repair.Limit {
					nodeContextLogger.Infof("detected unhealthy node %s (last heartbeat: %s), skipping due to concurrent repair limit", node.Name, nodeLastHeartbeat)
					continue nodeLoop
				}

				nodeContextLogger.Infof("detected unhealthy node %s (last heartbeat: %s), starting repair", node.Name, nodeLastHeartbeat)

				// parse node informations from provider ID
				nodeInfo, err := k8s.ExtractNodeInfo(node)
				if err != nil {
					contextLogger.Errorln(err.Error())
					continue nodeLoop
				}

				if r.DryRun {
					nodeContextLogger.Infof("node %s repair skipped, dry run", node.Name)
					r.nodeRepairLock.Add(node.Name, true, r.Config.Repair.LockDuration) //nolint:golint,errcheck
					continue nodeLoop
				}

				if nodeInfo.IsVmss {
					// node is VMSS instance
					err = r.azureVmssInstanceRepair(nodeContextLogger, *nodeInfo)
				} else {
					// node is a VM
					err = r.azureVmRepair(nodeContextLogger, *nodeInfo)
				}
				r.prometheus.repair.count.WithLabelValues().Inc()

				if err != nil {
					nodeContextLogger.Errorf("node %s repair failed: %s", node.Name, err.Error())
					r.nodeRepairLock.Add(node.Name, true, r.Config.Repair.LockDurationError) //nolint:golint,errcheck
					if k8sErr := r.k8sSetNodeLockAnnotation(node, r.Config.Repair.NodeLockAnnotation, r.Config.Repair.LockDurationError); k8sErr != nil {
						nodeContextLogger.Error(k8sErr)
					}
					continue nodeLoop
				} else {
					// lock vm for next redeploy, can take up to 15 mins
					r.nodeRepairLock.Add(node.Name, true, r.Config.Repair.LockDuration) //nolint:golint,errcheck
					if k8sErr := r.k8sSetNodeLockAnnotation(node, r.Config.Repair.NodeLockAnnotation, r.Config.Repair.LockDuration); k8sErr != nil {
						nodeContextLogger.Error(k8sErr)
					}
					nodeContextLogger.Infof("node %s successfully scheduled for repair", node.Name)
				}
			}
		} else {
			// node is NOT healthy
			nodeContextLogger.Debugf("detected healthy node %s", node.Name)
			r.nodeRepairLock.Delete(node.Name)
		}
	}
}
