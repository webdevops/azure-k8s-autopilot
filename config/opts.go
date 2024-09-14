package config

import (
	"encoding/json"
	"time"
)

type (
	Opts struct {
		// logger
		Logger struct {
			Debug       bool `long:"log.debug"    env:"LOG_DEBUG"  description:"debug mode"`
			Development bool `long:"log.devel"    env:"LOG_DEVEL"  description:"development mode"`
			Json        bool `long:"log.json"     env:"LOG_JSON"   description:"Switch log output to json format"`
		}

		// general settings
		DryRun bool `long:"dry-run"           env:"DRY_RUN"   description:"Dry run (no redeploy triggered)"`

		// instance
		Instance struct {
			Nodename  *string `long:"instance.nodename"    env:"INSTANCE_NODENAME"   description:"Name of node where autopilot is running"`
			Namespace *string `long:"instance.namespace"   env:"INSTANCE_NAMESPACE"   description:"Name of namespace where autopilot is running"`
			Pod       *string `long:"instance.pod"         env:"INSTANCE_POD"         description:"Name of pod where autopilot is running"`
		}

		// azure
		Azure struct {
			Environment *string `long:"azure.environment"            env:"AZURE_ENVIRONMENT"                description:"Azure environment name" default:"AZUREPUBLICCLOUD"`
		}

		Autoscaler struct {
			ScaledownLockTime time.Duration `long:"repautoscaler.scaledown-locktime"       env:"AUTOSCALER_SCALEDOWN_LOCKTIME"       description:"Prevents cluster autoscaler from scaling down the affected node after update and repair"   default:"60m"`
		}

		// k8s
		K8S struct {
			NodeLabelSelector string `long:"kube.node.labelselector"     env:"KUBE_NODE_LABELSELECTOR"     description:"Node Label selector which nodes should be checked"        default:""`
		}

		// lease
		Lease struct {
			Enabled bool   `long:"lease.enable"  env:"LEASE_ENABLE"  description:"Enable lease (leader election; enabled by default in docker images)"`
			Name    string `long:"lease.name"    env:"LEASE_NAME"    description:"Name of lease lock"               default:"azure-k8s-autopilot-leader"`
		}

		// check settings
		Repair struct {
			Crontab              string        `long:"repair.crontab"                  env:"REPAIR_CRONTAB"                  description:"Crontab of check runs"                                   default:"@every 2m"`
			NotReadyThreshold    time.Duration `long:"repair.notready-threshold"       env:"REPAIR_NOTREADY_THRESHOLD"       description:"Threshold (duration) when the automatic repair should be tried (eg. after 10 mins of NotReady state after last successfull heartbeat)"        default:"10m"`
			Limit                int           `long:"repair.concurrency"              env:"REPAIR_CONCURRENCY"              description:"How many VMs should be redeployed concurrently"          default:"1"`
			LockDuration         time.Duration `long:"repair.lock-duration"            env:"REPAIR_LOCK_DURATION"            description:"Duration how long should be waited for another redeploy on the same node" default:"30m"`
			LockDurationError    time.Duration `long:"repair.lock-duration-error"      env:"REPAIR_LOCK_DURATION_ERROR"      description:"Duration how long should be waited for another redeploy  on the same node in case an error occurred" default:"5m"`
			AzureVmssAction      string        `long:"repair.azure.vmss.action"        env:"REPAIR_AZURE_VMSS_ACTION"        description:"Defines the action which should be tried to repair the node (VMSS)" default:"redeploy" choice:"restart"  choice:"redeploy" choice:"reimage" choice:"delete"`                             //nolint:staticcheck
			AzureVmAction        string        `long:"repair.azure.vm.action"          env:"REPAIR_AZURE_VM_ACTION"          description:"Defines the action which should be tried to repair the node (VM)"   default:"redeploy" choice:"restart"  choice:"redeploy"`                                                              //nolint:staticcheck
			ProvisioningState    []string      `long:"repair.azure.provisioningstate"  env:"REPAIR_AZURE_PROVISIONINGSTATE"  description:"Azure VM provisioning states where repair should be tried (eg. avoid repair in \"upgrading\" state; \"*\" to accept all states)"     default:"succeeded" default:"failed" env-delim:" "` //nolint:staticcheck
			ProvisioningStateAll bool
			NodeLockAnnotation   string `long:"repair.lock-annotation"           env:"REPAIR_LOCK_ANNOTATION"         description:"Node annotation for repair lock time"                                                                      default:"autopilot.webdevops.io/repair-lock"`
		}

		// upgrade settings
		Update struct {
			Crontab               string        `long:"update.crontab"                  env:"UPDATE_CRONTAB"                  description:"Crontab of check runs"                                 default:"@every 15m"`
			Limit                 int           `long:"update.concurrency"              env:"UPDATE_CONCURRENCY"              description:"How many VMs should be updated concurrently"           default:"1"`
			LockDuration          time.Duration `long:"update.lock-duration"            env:"UPDATE_LOCK_DURATION"            description:"Duration how long should be waited for another update on the same node" default:"15m"`
			LockDurationError     time.Duration `long:"update.lock-duration-error"      env:"UPDATE_LOCK_DURATION_ERROR"      description:"Duration how long should be waited for another update  on the same node in case an error occurred" default:"5m"`
			NodeLockAnnotation    string        `long:"update.lock-annotation"          env:"UPDATE_LOCK_ANNOTATION"          description:"Node annotation for update lock time"                                                                      default:"autopilot.webdevops.io/update-lock"`
			NodeOngoingAnnotation string        `long:"update.ongoing-annotation"       env:"UPDATE_ONGOING_ANNOTATION"       description:"Node annotation for ongoing update lock"                                                                   default:"autopilot.webdevops.io/update-ongoing"`
			NodeExcludeAnnotation string        `long:"update.exclude-annotation"       env:"UPDATE_EXCLUDE_ANNOTATION"       description:"Node annotation for excluding node for updates"                                                            default:"autopilot.webdevops.io/exclude"`
			AzureVmssAction       string        `long:"update.azure.vmss.action"        env:"UPDATE_AZURE_VMSS_ACTION"        description:"Defines the action which should be tried to update the node (VMSS)" default:"update+reimage" choice:"update" choice:"update+reimage" choice:"delete"`                                    //nolint:staticcheck
			ProvisioningState     []string      `long:"update.azure.provisioningstate"  env:"UPDATE_AZURE_PROVISIONINGSTATE"  description:"Azure VM provisioning states where update should be tried (eg. avoid repair in \"upgrading\" state; \"*\" to accept all states)"     default:"succeeded" default:"failed" env-delim:" "` //nolint:staticcheck
			ProvisioningStateAll  bool
			FailedThreshold       int `long:"update.failed-threshold"         env:"UPDATE_FAILED_THRESHOLD"         description:"Failed node threshold when node update is stopped"           default:"2"`
		}

		// drain settings
		Drain OptsDrain

		// notification
		Notification []string `long:"notification" env:"NOTIFICATION" description:"Shoutrrr url for notifications (https://containrrr.github.io/shoutrrr/)" env-delim:" " json:"-"`

		// server settings
		Server struct {
			// general options
			Bind         string        `long:"server.bind"              env:"SERVER_BIND"           description:"Server address"        default:":8080"`
			ReadTimeout  time.Duration `long:"server.timeout.read"      env:"SERVER_TIMEOUT_READ"   description:"Server read timeout"   default:"5s"`
			WriteTimeout time.Duration `long:"server.timeout.write"     env:"SERVER_TIMEOUT_WRITE"  description:"Server write timeout"  default:"10s"`
		}
	}

	OptsDrain struct {
		KubectlPath        string        `long:"drain.kubectl"               env:"DRAIN_KUBECTL"               description:"Path to kubectl binary" default:"kubectl"`
		Enable             bool          `long:"drain.enable"                env:"DRAIN_ENABLE"                description:"Enable drain handling"`
		DeleteEmptydirData bool          `long:"drain.delete-emptydir-data"  env:"DRAIN_DELETE_EMPTYDIR_DATA"  description:"Continue even if there are pods using emptyDir (local emptydir that will be deleted when the node is drained)"`
		Force              bool          `long:"drain.force"                 env:"DRAIN_FORCE"                 description:"Continue even if there are pods not managed by a ReplicationController, ReplicaSet, Job, DaemonSet or StatefulSet"`
		GracePeriod        int64         `long:"drain.grace-period"          env:"DRAIN_GRACE_PERIOD"          description:"Period of time in seconds given to each pod to terminate gracefully. If negative, the default value specified in the pod will be used."`
		IgnoreDaemonsets   bool          `long:"drain.ignore-daemonsets"     env:"DRAIN_IGNORE_DAEMONSETS"     description:"Ignore DaemonSet-managed pods."`
		PodSelector        string        `long:"drain.pod-selector"          env:"DRAIN_POD_SELECTOR"          description:"Label selector to filter pods on the node"`
		Timeout            time.Duration `long:"drain.timeout"               env:"DRAIN_TIMEOUT"               description:"The length of time to wait before giving up, zero means infinite" default:"0s"`
		WaitAfter          time.Duration `long:"drain.wait-after"            env:"DRAIN_WAIT_AFTER"            description:"Wait after drain to let Kubernetes detach volumes etc"   default:"30s"`
		DryRun             bool          `long:"drain.dry-run"               env:"DRAIN_DRY_RUN"               description:"Do not drain, uncordon or label any node"`
		DisableEviction    bool          `long:"drain.disable-eviction"      env:"DRAIN_DISABLE_EVICTION"      description:"Force drain to use delete, even if eviction is supported. This will bypass checking PodDisruptionBudgets, use with caution."`

		RetryWithoutEviction bool `long:"drain.retry-without-eviction"      env:"DRAIN_RETRY_WITHOUT_EVICTION"           description:"Retry drain without eviction if first drain failed"`
		IgnoreFailure        bool `long:"drain.ignore-failure"      env:"DRAIN_IGNORE_FAILURE"           description:"Ignore failed drain and continue with actions"`
	}
)

func (o *Opts) GetJson() []byte {
	jsonBytes, err := json.Marshal(o)
	if err != nil {
		panic(err)
	}
	return jsonBytes
}
