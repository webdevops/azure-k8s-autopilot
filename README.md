Azurer Kubernetes Autopilot
============================

[![license](https://img.shields.io/github/license/webdevops/azure-k8s-autopilot.svg)](https://github.com/webdevops/azure-k8s-autopilot/blob/master/LICENSE)
[![Docker](https://img.shields.io/badge/docker-webdevops%2Fazure--k8s--autopilot-blue.svg?longCache=true&style=flat&logo=docker)](https://hub.docker.com/r/webdevops/azure-k8s-autopilot/)
[![Docker Build Status](https://img.shields.io/docker/cloud/build/webdevops/azure-k8s-autopilot)](https://hub.docker.com/r/webdevops/azure-k8s-autopilot/)

Services which checks node status and triggeres an automatic Azure VM redeployment to try to solve VM issues.

Supports Azure AKS and custom Azure Kubernetes clusters.

Supports [shoutrrr](https://containrrr.github.io/shoutrrr/) notifications.

Configuration
-------------

```
Usage:
  azure-k8s-autopilot [OPTIONS]

Application Options:
  -v, --verbose                                             Verbose mode [$VERBOSE]
      --dry-run                                             Dry run (no redeploy triggered) [$DRY_RUN]
      --k8s.node.labelselector=                             Node Label selector which nodes should be checked [$K8S_NODE_LABELSELECTOR]
      --repair.interval=                                    Duration of check run (default: 30s) [$REPAIR_INTERVAL]
      --repair.notready-threshold=                          Threshold (duration) when the automatic repair should be tried (eg. after 10 mins of NotReady state after last successfull
                                                            heartbeat) (default: 10m) [$REPAIR_NOTREADY_THRESHOLD]
      --repair.concurrency=                                 How many VMs should be redeployed concurrently (default: 1) [$REPAIR_CONCURRENCY]
      --repair.lock-duration=                               Duration how long should be waited for another redeploy (default: 30m) [$REPAIR_LOCK_DURATION]
      --repair.lock-duration-error=                         Duration how long should be waited for another redeploy in case an error occurred (default: 5m)
                                                            [$REPAIR_LOCK_DURATION_ERROR]
      --repair.azure.vmss.action=[restart|redeploy|reimage] Defines the action which should be tried to repair the node (VMSS) (default: redeploy) [$REPAIR_AZURE_VMSS_ACTION]
      --repair.azure.vm.action=[restart|redeploy]           Defines the action which should be tried to repair the node (VM) (default: redeploy) [$REPAIR_AZURE_VM_ACTION]
      --repair.azure.provisioningstate=                     Azure VM provisioning states where repair should be tried (eg. avoid repair in "upgrading" state; "*" to accept all
                                                            states) (default: succeeded, failed) [$REPAIR_AZURE_PROVISIONINGSTATE]
      --notification=                                       Shoutrrr url for notifications (https://containrrr.github.io/shoutrrr/) [$NOTIFCATION]
      --bind=                                               Server address (default: :8080) [$SERVER_BIND]

Help Options:
  -h, --help                                                Show this help message

```

for Azure API authentication (using ENV vars) see https://github.com/Azure/azure-sdk-for-go#authentication

for Kubernetes ServiceAccont is discoverd automatically (or you can use env path `KUBECONFIG` to specify path to your kubeconfig file)

Metrics
-------

standard metrics only (see `:8080/metrics`)
