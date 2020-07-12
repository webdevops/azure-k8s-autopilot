package config

import "time"

type (
	Opts struct {
		// general settings
		DryRun bool `long:"dry-run"           env:"DRY_RUN"   description:"Dry run (no redeploy triggered)"`

		// logger
		Logger struct {
			Debug   bool `           long:"debug"        env:"DEBUG"    description:"debug mode"`
			Verbose bool `short:"v"  long:"verbose"      env:"VERBOSE"  description:"verbose mode"`
			LogJson bool `           long:"log.json"     env:"LOG_JSON" description:"Switch log output to json format"`
		}

		// k8s
		K8S struct {
			NodeLabelSelector string `long:"k8s.node.labelselector"     env:"K8S_NODE_LABELSELECTOR"           description:"Node Label selector which nodes should be checked"                 default:""`
		}

		// check settings
		Repair struct {
			Crontab              string        `long:"repair.crontab"                  env:"REPAIR_CRONTAB"                  description:"Crontab of check runs"                                   default:"@every 2m"`
			NotReadyThreshold    time.Duration `long:"repair.notready-threshold"       env:"REPAIR_NOTREADY_THRESHOLD"       description:"Threshold (duration) when the automatic repair should be tried (eg. after 10 mins of NotReady state after last successfull heartbeat)"        default:"10m"`
			Limit                int           `long:"repair.concurrency"              env:"REPAIR_CONCURRENCY"              description:"How many VMs should be redeployed concurrently"          default:"1"`
			LockDuration         time.Duration `long:"repair.lock-duration"            env:"REPAIR_LOCK_DURATION"            description:"Duration how long should be waited for another redeploy" default:"30m"`
			LockDurationError    time.Duration `long:"repair.lock-duration-error"      env:"REPAIR_LOCK_DURATION_ERROR"      description:"Duration how long should be waited for another redeploy in case an error occurred" default:"5m"`
			AzureVmssAction      string        `long:"repair.azure.vmss.action"        env:"REPAIR_AZURE_VMSS_ACTION"        description:"Defines the action which should be tried to repair the node (VMSS)" default:"redeploy" choice:"restart"  choice:"redeploy" choice:"reimage"`
			AzureVmAction        string        `long:"repair.azure.vm.action"          env:"REPAIR_AZURE_VM_ACTION"          description:"Defines the action which should be tried to repair the node (VM)"   default:"redeploy" choice:"restart"  choice:"redeploy"`
			ProvisioningState    []string      `long:"repair.azure.provisioningstate"  env:"REPAIR_AZURE_PROVISIONINGSTATE"  description:"Azure VM provisioning states where repair should be tried (eg. avoid repair in \"upgrading\" state; \"*\" to accept all states)"     default:"succeeded" default:"failed" env-delim:" "`
			ProvisioningStateAll bool
			NodeLockAnnotation   string `long:"repair.lock-annotation"           env:"REPAIR_LOCK_ANNOTATION"         description:"Node annotation for repair lock time"                                                                      default:"autopilot.webdevops.io/repair-lock"`
		}

		// upgrade settings
		Update struct {
			Crontab              string        `long:"update.crontab"                  env:"UPDATE_CRONTAB"                  description:"Crontab of check runs"                                 default:"@every 15m"`
			Limit                int           `long:"update.concurrency"              env:"UPDATE_CONCURRENCY"              description:"How many VMs should be updated concurrently"           default:"1"`
			LockDuration         time.Duration `long:"update.lock-duration"            env:"UPDATE_LOCK_DURATION"            description:"Duration how long should be waited for another update" default:"15m"`
			LockDurationError    time.Duration `long:"update.lock-duration-error"      env:"UPDATE_LOCK_DURATION_ERROR"      description:"Duration how long should be waited for another update in case an error occurred" default:"5m"`
			ProvisioningState    []string      `long:"update.azure.provisioningstate"  env:"UPDATE_AZURE_PROVISIONINGSTATE"  description:"Azure VM provisioning states where update should be tried (eg. avoid repair in \"upgrading\" state; \"*\" to accept all states)"     default:"succeeded" default:"failed" env-delim:" "`
			ProvisioningStateAll bool
			NodeLockAnnotation   string `long:"update.lock-annotation"          env:"UPDATE_LOCK_ANNOTATION"          description:"Node annotation for update lock time"                                                                      default:"autopilot.webdevops.io/update-lock"`
		}

		// drain settings
		Drain OptsDrain

		// notification
		Notification []string `long:"notification" env:"NOTIFCATION" description:"Shoutrrr url for notifications (https://containrrr.github.io/shoutrrr/)" env-delim:" "`

		// server settings
		ServerBind string `long:"bind" env:"SERVER_BIND"  description:"Server address"  default:":8080"`
	}

	OptsDrain struct {
		KubectlPath      string        `long:"drain.kubectl"            env:"DRAIN_KUBECTL"            description:"Path to kubectl binary" default:"kubectl"`
		Enable           bool          `long:"drain.enable"             env:"DRAIN_ENABLE"             description:"Enable drain handling"`
		DeleteLocalData  bool          `long:"drain.delete-local-data"  env:"DRAIN_DELETE_LOCAL_DATA"  description:"Continue even if there are pods using emptyDir (local data that will be deleted when the node is drained)"`
		Force            bool          `long:"drain.force"              env:"DRAIN_FORCE"              description:"Continue even if there are pods not managed by a ReplicationController, ReplicaSet, Job, DaemonSet or StatefulSet"`
		GracePeriod      int64         `long:"drain.grace-period"       env:"DRAIN_GRACE_PERIOD"       description:"Period of time in seconds given to each pod to terminate gracefully. If negative, the default value specified in the pod will be used."`
		IgnoreDaemonsets bool          `long:"drain.ignore-daemonsets"  env:"DRAIN_IGNORE_DAEMONSETS"  description:"Ignore DaemonSet-managed pods."`
		PodSelector      string        `long:"drain.pod-selector"       env:"DRAIN_POD_SELECTOR"       description:"Label selector to filter pods on the node"`
		Timeout          time.Duration `long:"drain.timeout"            env:"DRAIN_TIMEOUT"            description:"The length of time to wait before giving up, zero means infinite" default:"0s"`
		DryRun           bool          `long:"drain.dry-run"            env:"DRAIN_DRY_RUN"            description:"Do not drain, uncordon or label any node"`
	}
)
