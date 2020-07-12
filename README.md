Azurer Kubernetes Autopilot
============================

[![license](https://img.shields.io/github/license/webdevops/azure-k8s-autopilot.svg)](https://github.com/webdevops/azure-k8s-autopilot/blob/master/LICENSE)
[![Docker](https://img.shields.io/badge/docker-webdevops%2Fazure--k8s--autopilot-blue.svg?longCache=true&style=flat&logo=docker)](https://hub.docker.com/r/webdevops/azure-k8s-autopilot/)
[![Docker Build Status](https://img.shields.io/docker/cloud/build/webdevops/azure-k8s-autopilot)](https://hub.docker.com/r/webdevops/azure-k8s-autopilot/)

Kubernetess service for automatic maintenance of an Azure cluster.

- auto repair (repair nodes if NotReady; VM and VMSS support)
- auto update (update VMSS instances automatically to latest model; only VMSS)

Supports Azure AKS and custom Azure Kubernetes clusters.

Supports [shoutrrr](https://containrrr.github.io/shoutrrr/) notifications.

(Successor of `azure-k8s-autorepair`)

Configuration
-------------

```
Usage:
  azure-k8s-autopilot [OPTIONS]

Application Options:
      --dry-run                                             Dry run (no redeploy triggered) [$DRY_RUN]
      --debug                                               debug mode [$DEBUG]
  -v, --verbose                                             verbose mode [$VERBOSE]
      --log.json                                            Switch log output to json format [$LOG_JSON]
      --k8s.node.labelselector=                             Node Label selector which nodes should be checked [$K8S_NODE_LABELSELECTOR]
      --repair.crontab=                                     Crontab of check runs (default: @every 2m) [$REPAIR_CRONTAB]
      --repair.notready-threshold=                          Threshold (duration) when the automatic repair should be tried (eg. after 10 mins of NotReady state after last successfull heartbeat)
                                                            (default: 10m) [$REPAIR_NOTREADY_THRESHOLD]
      --repair.concurrency=                                 How many VMs should be redeployed concurrently (default: 1) [$REPAIR_CONCURRENCY]
      --repair.lock-duration=                               Duration how long should be waited for another redeploy (default: 30m) [$REPAIR_LOCK_DURATION]
      --repair.lock-duration-error=                         Duration how long should be waited for another redeploy in case an error occurred (default: 5m) [$REPAIR_LOCK_DURATION_ERROR]
      --repair.azure.vmss.action=[restart|redeploy|reimage] Defines the action which should be tried to repair the node (VMSS) (default: redeploy) [$REPAIR_AZURE_VMSS_ACTION]
      --repair.azure.vm.action=[restart|redeploy]           Defines the action which should be tried to repair the node (VM) (default: redeploy) [$REPAIR_AZURE_VM_ACTION]
      --repair.azure.provisioningstate=                     Azure VM provisioning states where repair should be tried (eg. avoid repair in "upgrading" state; "*" to accept all states) (default:
                                                            succeeded, failed) [$REPAIR_AZURE_PROVISIONINGSTATE]
      --repair.lock-annotation=                             Node annotation for repair lock time (default: autopilot.webdevops.io/repair-lock) [$REPAIR_LOCK_ANNOTATION]
      --update.crontab=                                     Crontab of check runs (default: @every 15m) [$UPDATE_CRONTAB]
      --update.concurrency=                                 How many VMs should be updated concurrently (default: 1) [$UPDATE_CONCURRENCY]
      --update.lock-duration=                               Duration how long should be waited for another update (default: 15m) [$UPDATE_LOCK_DURATION]
      --update.lock-duration-error=                         Duration how long should be waited for another update in case an error occurred (default: 5m) [$UPDATE_LOCK_DURATION_ERROR]
      --update.azure.provisioningstate=                     Azure VM provisioning states where update should be tried (eg. avoid repair in "upgrading" state; "*" to accept all states) (default:
                                                            succeeded, failed) [$UPDATE_AZURE_PROVISIONINGSTATE]
      --update.lock-annotation=                             Node annotation for update lock time (default: autopilot.webdevops.io/update-lock) [$UPDATE_LOCK_ANNOTATION]
      --drain.kubectl=                                      Path to kubectl binary (default: kubectl) [$DRAIN_KUBECTL]
      --drain.enable                                        Enable drain handling [$DRAIN_ENABLE]
      --drain.not-before=                                   Dont drain before this time (default: 5m) [$DRAIN_NOT_BEFORE]
      --drain.delete-local-data                             Continue even if there are pods using emptyDir (local data that will be deleted when the node is drained) [$DRAIN_DELETE_LOCAL_DATA]
      --drain.force                                         Continue even if there are pods not managed by a ReplicationController, ReplicaSet, Job, DaemonSet or StatefulSet [$DRAIN_FORCE]
      --drain.grace-period=                                 Period of time in seconds given to each pod to terminate gracefully. If negative, the default value specified in the pod will be used.
                                                            [$DRAIN_GRACE_PERIOD]
      --drain.ignore-daemonsets                             Ignore DaemonSet-managed pods. [$DRAIN_IGNORE_DAEMONSETS]
      --drain.pod-selector=                                 Label selector to filter pods on the node [$DRAIN_POD_SELECTOR]
      --drain.timeout=                                      The length of time to wait before giving up, zero means infinite (default: 0s) [$DRAIN_TIMEOUT]
      --drain.dry-run                                       Do not drain, uncordon or label any node [$DRAIN_DRY_RUN]
      --notification=                                       Shoutrrr url for notifications (https://containrrr.github.io/shoutrrr/) [$NOTIFCATION]
      --bind=                                               Server address (default: :8080) [$SERVER_BIND]

Help Options:
  -h, --help                                                Show this help message
```

for Azure API authentication (using ENV vars) see https://github.com/Azure/azure-sdk-for-go#authentication

for Kubernetes ServiceAccont is discoverd automatically (or you can use env path `KUBECONFIG` to specify path to your kubeconfig file)

Metrics
-------

 (see `:8080/metrics`)

| Metric                         | Description                                     |
|:-------------------------------|:------------------------------------------------|
| `autopilot_repair_count`       | Count of repair actions                         |
| `autopilot_repair_node_status` | Node status                                     |
| `autopilot_repair_duration`    | Duration of repair task                         |
| `autopilot_update_count`       | Count of update actions                         |
| `autopilot_update_duration`    | Duration of last exec                           |
