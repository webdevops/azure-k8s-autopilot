package autopilot

import (
	"log/slog"
	"time"

	"github.com/webdevops/go-common/log/slogger"

	"github.com/webdevopos/azure-k8s-autopilot/k8s"
)

func (r *AzureK8sAutopilot) repairRun(contextLogger *slogger.Logger) {
	r.nodeList.Cleanup()
	nodeList := r.nodeList.NodeList()

	repairThresholdSeconds := r.Config.Repair.NotReadyThreshold.Seconds()

	contextLogger.Debugf("found %v nodes in cluster (%v in locked state)", len(nodeList), r.repair.nodeLock.ItemCount())

	for _, node := range nodeList {
		nodeContextLogger := contextLogger.With(slog.String("node", node.Name))

		nodeContextLogger.Debug("checking node")
		r.prometheus.repair.nodeStatus.WithLabelValues(node.Name).Set(0)

		// check if node is ready/healthy
		if nodeIsHealthy, nodeLastHeartbeat := node.GetHealthStatus(); !nodeIsHealthy {
			// node is NOT healthy
			nodeLastHeartbeatText := nodeLastHeartbeat.String()
			nodeLastHeartbeatAge := time.Since(nodeLastHeartbeat).Seconds()

			// ignore cordoned nodes, maybe maintenance work in progress
			if node.Spec.Unschedulable {
				nodeContextLogger.Info("detected unhealthy node, ignoring because node is cordoned")
				continue
			}

			// check if heartbeat already exceeded threshold
			if nodeLastHeartbeatAge < repairThresholdSeconds {
				nodeContextLogger.Info("detected unhealthy node, but deadline not reached yet", slog.String("lastHeartbeat", nodeLastHeartbeatText), slog.Duration("deadline", r.Config.Repair.NotReadyThreshold))
				continue
			}

			r.prometheus.repair.nodeStatus.WithLabelValues(node.Name).Set(1)

			var err error

			// redeploy timeout lock
			if _, expiry, exists := r.repair.nodeLock.GetWithExpiration(node.Name); exists {
				nodeContextLogger.Info("detected unhealthy node, still locked", slog.String("lastHeartbeat", nodeLastHeartbeatText), slog.Time("lockTime", expiry)) //nolint:gosimple
				continue
			}

			// concurrency repair limit
			if r.Config.Repair.Limit > 0 && r.repair.nodeLock.ItemCount() >= r.Config.Repair.Limit {
				nodeContextLogger.Info("detected unhealthy node, skipping due to concurrent repair limit", slog.String("lastHeartbeat", nodeLastHeartbeatText))
				continue
			}

			nodeContextLogger.Info("detected unhealthy node, starting repair", slog.String("lastHeartbeat", nodeLastHeartbeatText))

			// parse node informations from provider ID
			nodeInfo, err := k8s.ExtractNodeInfo(node)
			if err != nil {
				contextLogger.Error(err.Error())
				continue
			}

			if r.Config.DryRun {
				nodeContextLogger.Info("node repair skipped, dry run")
				if err := r.repair.nodeLock.Add(node.Name, true, r.Config.Repair.LockDuration); err != nil {
					nodeContextLogger.Error(err.Error())
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
				nodeContextLogger.Error("node repair failed: %s", slog.Any("error", err))
				// lock vm for next redeploy, can take up to 15 mins
				if err := r.repair.nodeLock.Add(node.Name, true, r.Config.Repair.LockDurationError); err != nil {
					nodeContextLogger.Error(err.Error())
				}
				if k8sErr := node.AnnotationLockSet(r.Config.Repair.NodeLockAnnotation, r.Config.Repair.LockDurationError, r.Config.Autoscaler.ScaledownLockTime); k8sErr != nil {
					nodeContextLogger.Error(k8sErr.Error())
				}
				continue
			} else {
				// lock vm for next redeploy, can take up to 15 mins
				if err := r.repair.nodeLock.Add(node.Name, true, r.Config.Repair.LockDuration); err != nil {
					nodeContextLogger.Error(err.Error())
				}
				if k8sErr := node.AnnotationLockSet(r.Config.Repair.NodeLockAnnotation, r.Config.Repair.LockDuration, r.Config.Autoscaler.ScaledownLockTime); k8sErr != nil {
					nodeContextLogger.Error(k8sErr.Error())
				}
				nodeContextLogger.Infof("node successfully repaired")
			}
		} else {
			// node IS healthy
			nodeContextLogger.Debugf("detected healthy node")
			r.repair.nodeLock.Delete(node.Name)
		}
	}
}
