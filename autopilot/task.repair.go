package autopilot

import (
	"time"

	"go.uber.org/zap"

	"github.com/webdevopos/azure-k8s-autopilot/k8s"
)

func (r *AzureK8sAutopilot) repairRun(contextLogger *zap.SugaredLogger) {
	r.nodeList.Cleanup()
	nodeList := r.nodeList.NodeList()

	repairThresholdSeconds := r.Config.Repair.NotReadyThreshold.Seconds()

	contextLogger.Debugf("found %v nodes in cluster (%v in locked state)", len(nodeList), r.repair.nodeLock.ItemCount())

	for _, node := range nodeList {
		nodeContextLogger := contextLogger.With(zap.String("node", node.Name))

		nodeContextLogger.Debug("checking node")
		r.prometheus.repair.nodeStatus.WithLabelValues(node.Name).Set(0)

		// check if node is ready/healthy
		if nodeIsHealthy, nodeLastHeartbeat := node.GetHealthStatus(); !nodeIsHealthy {
			// node is NOT healthy
			nodeLastHeartbeatText := nodeLastHeartbeat.String()
			nodeLastHeartbeatAge := time.Since(nodeLastHeartbeat).Seconds()

			// ignore cordoned nodes, maybe maintenance work in progress
			if node.Spec.Unschedulable {
				nodeContextLogger.Infof("detected unhealthy node %s, ignoring because node is cordoned", node.Name)
				continue
			}

			// check if heartbeat already exceeded threshold
			if nodeLastHeartbeatAge < repairThresholdSeconds {
				nodeContextLogger.Infof("detected unhealthy node %s (last heartbeat: %s), deadline of %v not reached", node.Name, nodeLastHeartbeatText, r.Config.Repair.NotReadyThreshold.String())
				continue
			}

			r.prometheus.repair.nodeStatus.WithLabelValues(node.Name).Set(1)

			var err error

			// redeploy timeout lock
			if _, expiry, exists := r.repair.nodeLock.GetWithExpiration(node.Name); exists {
				nodeContextLogger.Infof("detected unhealthy node %s (last heartbeat: %s), locked (relased in %v)", node.Name, nodeLastHeartbeatText, expiry.Sub(time.Now())) //nolint:gosimple
				continue
			}

			// concurrency repair limit
			if r.Config.Repair.Limit > 0 && r.repair.nodeLock.ItemCount() >= r.Config.Repair.Limit {
				nodeContextLogger.Infof("detected unhealthy node %s (last heartbeat: %s), skipping due to concurrent repair limit", node.Name, nodeLastHeartbeatText)
				continue
			}

			nodeContextLogger.Infof("detected unhealthy node %s (last heartbeat: %s), starting repair", node.Name, nodeLastHeartbeatText)

			// parse node informations from provider ID
			nodeInfo, err := k8s.ExtractNodeInfo(node)
			if err != nil {
				contextLogger.Errorln(err.Error())
				continue
			}

			if r.DryRun {
				nodeContextLogger.Infof("node %s repair skipped, dry run", node.Name)
				if err := r.repair.nodeLock.Add(node.Name, true, r.Config.Repair.LockDuration); err != nil {
					nodeContextLogger.Error(err)
				}
				continue
			}

			// increase metric counter
			r.prometheus.repair.count.WithLabelValues().Inc()

			// check if self eviction is needed
			if r.checkSelfEviction(node) {
				return
			}

			if nodeInfo.IsVmss {
				// node is VMSS instance
				err = r.azureVmssInstanceRepair(nodeContextLogger, *nodeInfo)
			} else {
				// node is a VM
				err = r.azureVmRepair(nodeContextLogger, *nodeInfo)
			}

			if err != nil {
				r.prometheus.general.errors.WithLabelValues("azure").Inc()
				nodeContextLogger.Errorf("node %s repair failed: %s", node.Name, err.Error())
				// lock vm for next redeploy, can take up to 15 mins
				if err := r.repair.nodeLock.Add(node.Name, true, r.Config.Repair.LockDurationError); err != nil {
					nodeContextLogger.Error(err)
				}
				if k8sErr := node.AnnotationLockSet(r.Config.Repair.NodeLockAnnotation, r.Config.Repair.LockDurationError, r.Config.Autoscaler.ScaledownLockTime); k8sErr != nil {
					nodeContextLogger.Error(k8sErr)
				}
				continue
			} else {
				// lock vm for next redeploy, can take up to 15 mins
				if err := r.repair.nodeLock.Add(node.Name, true, r.Config.Repair.LockDuration); err != nil {
					nodeContextLogger.Error(err)
				}
				if k8sErr := node.AnnotationLockSet(r.Config.Repair.NodeLockAnnotation, r.Config.Repair.LockDuration, r.Config.Autoscaler.ScaledownLockTime); k8sErr != nil {
					nodeContextLogger.Error(k8sErr)
				}
				nodeContextLogger.Infof("node %s successfully repaired", node.Name)
			}
		} else {
			// node IS healthy
			nodeContextLogger.Debugf("detected healthy node %s", node.Name)
			r.repair.nodeLock.Delete(node.Name)
		}
	}
}
