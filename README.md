Azurer Kubernetes AutoRepair
============================

[![license](https://img.shields.io/github/license/webdevops/azure-k8s-autorepair.svg)](https://github.com/webdevops/azure-k8s-autorepair/blob/master/LICENSE)
[![Docker](https://img.shields.io/badge/docker-webdevops%2Fazure--k8s--autorepair-blue.svg?longCache=true&style=flat&logo=docker)](https://hub.docker.com/r/webdevops/azure-k8s-autorepair/)
[![Docker Build Status](https://img.shields.io/docker/cloud/build/webdevops/azure-k8s-autorepair)](https://hub.docker.com/r/webdevops/azure-k8s-autorepair/)

Services which checks node status and triggeres an automatic Azure VM redeployment to try to solve VM issues.

Configuration
-------------

```
Usage:
  azure-k8s-autorepair [OPTIONS]

Application Options:
  -v, --verbose              Verbose mode [$VERBOSE]
      --dry-run              Dry run (no redeploy triggered) [$DRY_RUN]
      --repair.interval=     Duration of check run (default: 30s) [$REPAIR_INTERVAL]
      --repair.waitduration= Duration to wait when redeploy will be triggered (default: 10m) [$REPAIR_WAIT_DURATION]
      --repair.concurrency=  How many VMs should be redeployed concurrently (default: 1) [$REPAIR_CONCURRENCY]
      --repair.lockduration= Duration how long should be waited for another redeploy (default: 15m) [$REPAIR_LOCK_DURATION]
      --bind=                Server address (default: :8080) [$SERVER_BIND]

Help Options:
  -h, --help                 Show this help message
```

for Azure API authentication (using ENV vars) see https://github.com/Azure/azure-sdk-for-go#authentication

for Kubernetes ServiceAccont is discoverd automatically (or you can use env path `KUBECONFIG` to specify path to your kubeconfig file)

Metrics
-------

no custom metrics for now
