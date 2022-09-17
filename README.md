# Azure Kubernetes Autopilot

[![license](https://img.shields.io/github/license/webdevops/azure-k8s-autopilot.svg)](https://github.com/webdevops/azure-k8s-autopilot/blob/master/LICENSE)
[![DockerHub](https://img.shields.io/badge/DockerHub-webdevops%2Fazure--k8s--autopilot-blue)](https://hub.docker.com/r/webdevops/azure-k8s-autopilot/)
[![Quay.io](https://img.shields.io/badge/Quay.io-webdevops%2Fazure--k8s--autopilot-blue)](https://quay.io/repository/webdevops/azure-k8s-autopilot)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/azure-k8s-autopilot)](https://artifacthub.io/packages/search?repo=azure-k8s-autopilot)

Kubernetess service for automatic maintenance of an Azure cluster.

- auto repair (repair nodes if NotReady; VM and VMSS support)
- auto update (update VMSS instances automatically to latest model; only VMSS)

Supports Azure AKS and custom Azure Kubernetes clusters.

Supports [shoutrrr](https://containrrr.github.io/shoutrrr/) notifications.

(Successor of `azure-k8s-autorepair`)

## Configuration

```
Usage:
  azure-k8s-autopilot [OPTIONS]

Application Options:
      --dry-run                                                    Dry run (no redeploy triggered) [$DRY_RUN]
      --instance.nodename=                                         Name of node where autopilot is running [$INSTANCE_NODENAME]
      --instance.namespace=                                        Name of namespace where autopilot is running
                                                                   [$INSTANCE_NAMESPACE]
      --instance.pod=                                              Name of pod where autopilot is running [$INSTANCE_POD]
      --azure.environment=                                         Azure environment name (default: AZUREPUBLICCLOUD)
                                                                   [$AZURE_ENVIRONMENT]
      --debug                                                      debug mode [$DEBUG]
  -v, --verbose                                                    verbose mode [$VERBOSE]
      --log.json                                                   Switch log output to json format [$LOG_JSON]
      --repautoscaler.scaledown-locktime=                          Prevents cluster autoscaler from scaling down the affected
                                                                   node after update and repair (default: 60m)
                                                                   [$AUTOSCALER_SCALEDOWN_LOCKTIME]
      --kube.node.labelselector=                                   Node Label selector which nodes should be checked
                                                                   [$KUBE_NODE_LABELSELECTOR]
      --lease.enable                                               Enable lease (leader election; enabled by default in docker
                                                                   images) [$LEASE_ENABLE]
      --lease.name=                                                Name of lease lock (default: azure-k8s-autopilot-leader)
                                                                   [$LEASE_NAME]
      --repair.crontab=                                            Crontab of check runs (default: @every 2m) [$REPAIR_CRONTAB]
      --repair.notready-threshold=                                 Threshold (duration) when the automatic repair should be
                                                                   tried (eg. after 10 mins of NotReady state after last
                                                                   successfull heartbeat) (default: 10m)
                                                                   [$REPAIR_NOTREADY_THRESHOLD]
      --repair.concurrency=                                        How many VMs should be redeployed concurrently (default: 1)
                                                                   [$REPAIR_CONCURRENCY]
      --repair.lock-duration=                                      Duration how long should be waited for another redeploy
                                                                   (default: 30m) [$REPAIR_LOCK_DURATION]
      --repair.lock-duration-error=                                Duration how long should be waited for another redeploy in
                                                                   case an error occurred (default: 5m)
                                                                   [$REPAIR_LOCK_DURATION_ERROR]
      --repair.azure.vmss.action=[restart|redeploy|reimage|delete] Defines the action which should be tried to repair the node
                                                                   (VMSS) (default: redeploy) [$REPAIR_AZURE_VMSS_ACTION]
      --repair.azure.vm.action=[restart|redeploy]                  Defines the action which should be tried to repair the node
                                                                   (VM) (default: redeploy) [$REPAIR_AZURE_VM_ACTION]
      --repair.azure.provisioningstate=                            Azure VM provisioning states where repair should be tried
                                                                   (eg. avoid repair in "upgrading" state; "*" to accept all
                                                                   states) (default: succeeded, failed)
                                                                   [$REPAIR_AZURE_PROVISIONINGSTATE]
      --repair.lock-annotation=                                    Node annotation for repair lock time (default:
                                                                   autopilot.webdevops.io/repair-lock) [$REPAIR_LOCK_ANNOTATION]
      --update.crontab=                                            Crontab of check runs (default: @every 15m) [$UPDATE_CRONTAB]
      --update.concurrency=                                        How many VMs should be updated concurrently (default: 1)
                                                                   [$UPDATE_CONCURRENCY]
      --update.lock-duration=                                      Duration how long should be waited for another update
                                                                   (default: 15m) [$UPDATE_LOCK_DURATION]
      --update.lock-duration-error=                                Duration how long should be waited for another update in case
                                                                   an error occurred (default: 5m) [$UPDATE_LOCK_DURATION_ERROR]
      --update.lock-annotation=                                    Node annotation for update lock time (default:
                                                                   autopilot.webdevops.io/update-lock) [$UPDATE_LOCK_ANNOTATION]
      --update.ongoing-annotation=                                 Node annotation for ongoing update lock (default:
                                                                   autopilot.webdevops.io/update-ongoing)
                                                                   [$UPDATE_ONGOING_ANNOTATION]
      --update.exclude-annotation=                                 Node annotation for excluding node for updates (default:
                                                                   autopilot.webdevops.io/exclude) [$UPDATE_EXCLUDE_ANNOTATION]
      --update.azure.vmss.action=[update|update+reimage|delete]    Defines the action which should be tried to update the node
                                                                   (VMSS) (default: update+reimage) [$UPDATE_AZURE_VMSS_ACTION]
      --update.azure.provisioningstate=                            Azure VM provisioning states where update should be tried
                                                                   (eg. avoid repair in "upgrading" state; "*" to accept all
                                                                   states) (default: succeeded, failed)
                                                                   [$UPDATE_AZURE_PROVISIONINGSTATE]
      --update.failed-threshold=                                   Failed node threshold when node update is stopped (default:
                                                                   2) [$UPDATE_FAILED_THRESHOLD]
      --drain.kubectl=                                             Path to kubectl binary (default: kubectl) [$DRAIN_KUBECTL]
      --drain.enable                                               Enable drain handling [$DRAIN_ENABLE]
      --drain.delete-emptydir-data                                 Continue even if there are pods using emptyDir (local
                                                                   emptydir that will be deleted when the node is drained)
                                                                   [$DRAIN_DELETE_EMPTYDIR_DATA]
      --drain.force                                                Continue even if there are pods not managed by a
                                                                   ReplicationController, ReplicaSet, Job, DaemonSet or
                                                                   StatefulSet [$DRAIN_FORCE]
      --drain.grace-period=                                        Period of time in seconds given to each pod to terminate
                                                                   gracefully. If negative, the default value specified in the
                                                                   pod will be used. [$DRAIN_GRACE_PERIOD]
      --drain.ignore-daemonsets                                    Ignore DaemonSet-managed pods. [$DRAIN_IGNORE_DAEMONSETS]
      --drain.pod-selector=                                        Label selector to filter pods on the node
                                                                   [$DRAIN_POD_SELECTOR]
      --drain.timeout=                                             The length of time to wait before giving up, zero
                                                                   means infinite (default: 0s) [$DRAIN_TIMEOUT]
      --drain.wait-after=                                          Wait after drain to let Kubernetes detach volumes
                                                                   etc (default: 30s) [$DRAIN_WAIT_AFTER]
      --drain.dry-run                                              Do not drain, uncordon or label any node
                                                                   [$DRAIN_DRY_RUN]
      --drain.disable-eviction                                     Force drain to use delete, even if eviction is
                                                                   supported. This will bypass checking
                                                                   PodDisruptionBudgets, use with caution.
                                                                   [$DRAIN_DISABLE_EVICTION]
      --notification=                                              Shoutrrr url for notifications
                                                                   (https://containrrr.github.io/shoutrrr/) [$NOTIFICATION]
      --server.bind=                                               Server address (default: :8080) [$SERVER_BIND]
      --server.timeout.read=                                       Server read timeout (default: 5s) [$SERVER_TIMEOUT_READ]
      --server.timeout.write=                                      Server write timeout (default: 10s) [$SERVER_TIMEOUT_WRITE]

Help Options:
  -h, --help                                                       Show this help message
```

for Azure API authentication (using ENV vars) see https://docs.microsoft.com/en-us/azure/developer/go/azure-sdk-authentication-service-principal

for Kubernetes ServiceAccont is discoverd automatically (or you can use env path `KUBECONFIG` to specify path to your kubeconfig file)

## Metrics

 (see `:8080/metrics`)

| Metric                         | Description                                     |
|:-------------------------------|:------------------------------------------------|
| `autopilot_repair_count`       | Count of repair actions                         |
| `autopilot_repair_node_status` | Node status                                     |
| `autopilot_repair_duration`    | Duration of repair task                         |
| `autopilot_update_count`       | Count of update actions                         |
| `autopilot_update_duration`    | Duration of last exec                           |

### AzureTracing metrics

(with 22.2.0 and later)

Azuretracing metrics collects latency and latency from azure-sdk-for-go and creates metrics and is controllable using
environment variables (eg. setting buckets, disabling metrics or disable autoreset).

| Metric                                   | Description                                                                            |
|------------------------------------------|----------------------------------------------------------------------------------------|
| `azurerm_api_ratelimit`                  | Azure ratelimit metrics (only on /metrics, resets after query due to limited validity) |
| `azurerm_api_request_*`                  | Azure request count and latency as histogram                                           |

#### Settings

| Environment variable                     | Example                            | Description                                                    |
|------------------------------------------|------------------------------------|----------------------------------------------------------------|
| `METRIC_AZURERM_API_REQUEST_BUCKETS`     | `1, 2.5, 5, 10, 30, 60, 90, 120`   | Sets buckets for `azurerm_api_request` histogram metric        |
| `METRIC_AZURERM_API_REQUEST_ENABLE`      | `false`                            | Enables/disables `azurerm_api_request_*` metric                |
| `METRIC_AZURERM_API_REQUEST_LABELS`      | `apiEndpoint, method, statusCode`  | Controls labels of `azurerm_api_request_*` metric              |
| `METRIC_AZURERM_API_RATELIMIT_ENABLE`    | `false`                            | Enables/disables `azurerm_api_ratelimit` metric                |
| `METRIC_AZURERM_API_RATELIMIT_AUTORESET` | `false`                            | Enables/disables `azurerm_api_ratelimit` autoreset after fetch |


| `azurerm_api_request` label | Status             | Description                                                                                              |
|-----------------------------|--------------------|----------------------------------------------------------------------------------------------------------|
| `apiEndpoint`               | enabled by default | hostname of endpoint (max 3 parts)                                                                       |
| `routingRegion`             | enabled by default | detected region for API call, either routing region from Azure Management API or Azure resource location |
| `subscriptionID`            | enabled by default | detected subscriptionID                                                                                  |
| `tenantID`                  | enabled by default | detected tenantID (extracted from jwt auth token)                                                        |
| `resourceProvider`          | enabled by default | detected Azure Management API provider                                                                   |
| `method`                    | enabled by default | HTTP method                                                                                              |
| `statusCode`                | enabled by default | HTTP status code                                                                                         |
