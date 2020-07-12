package main

import (
	"fmt"
	"github.com/jessevdk/go-flags"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/webdevopos/azure-k8s-autopilot/autopilot"
	"net/http"
	"os"
	"runtime"
	"time"
)

const (
	Author = "webdevops.io"
)

var (
	argparser *flags.Parser

	// Git version information
	gitCommit = "<unknown>"
	gitTag    = "<unknown>"
)

var opts struct {
	// general settings
	DryRun bool `long:"dry-run"           env:"DRY_RUN"   description:"Dry run (no redeploy triggered)"`

	// logger
	Verbose bool `short:"v"  long:"verbose"      env:"VERBOSE"  description:"verbose mode"`
	LogJson bool `           long:"log.json"     env:"LOG_JSON" description:"Switch log output to json format"`

	// k8s
	K8sNodeLabelSelector  string `long:"k8s.node.labelselector"     env:"K8S_NODE_LABELSELECTOR"           description:"Node Label selector which nodes should be checked"                 default:""`

	// check settings
	RepairCrontab           *string       `long:"repair.crontab"                  env:"REPAIR_CRONTAB"                  description:"Crontab of check runs"                                   default:"@every 2m"`
	RepairNotReadyThreshold time.Duration `long:"repair.notready-threshold"       env:"REPAIR_NOTREADY_THRESHOLD"       description:"Threshold (duration) when the automatic repair should be tried (eg. after 10 mins of NotReady state after last successfull heartbeat)"        default:"10m"`
	RepairLimit             int           `long:"repair.concurrency"              env:"REPAIR_CONCURRENCY"              description:"How many VMs should be redeployed concurrently"          default:"1"`
	RepairLockDuration      time.Duration `long:"repair.lock-duration"            env:"REPAIR_LOCK_DURATION"            description:"Duration how long should be waited for another redeploy" default:"30m"`
	RepairLockDurationError time.Duration `long:"repair.lock-duration-error"      env:"REPAIR_LOCK_DURATION_ERROR"      description:"Duration how long should be waited for another redeploy in case an error occurred" default:"5m"`
	RepairAzureVmssAction   string        `long:"repair.azure.vmss.action"        env:"REPAIR_AZURE_VMSS_ACTION"        description:"Defines the action which should be tried to repair the node (VMSS)" default:"redeploy" choice:"restart"  choice:"redeploy" choice:"reimage"`
	RepairAzureVmAction     string        `long:"repair.azure.vm.action"          env:"REPAIR_AZURE_VM_ACTION"          description:"Defines the action which should be tried to repair the node (VM)"   default:"redeploy" choice:"restart"  choice:"redeploy"`
	RepairProvisioningState []string      `long:"repair.azure.provisioningstate"  env:"REPAIR_AZURE_PROVISIONINGSTATE"  description:"Azure VM provisioning states where repair should be tried (eg. avoid repair in \"upgrading\" state; \"*\" to accept all states)"     default:"succeeded" default:"failed" env-delim:" "`
	RepairNodeLockAnnotation string       `long:"repair.lock-annotation"           env:"REPAIR_LOCK_ANNOTATION"         description:"Node annotation for repair lock time"                                                                      default:"autopilot.webdevops.io/repair-lock"`

	// upgrade settings
	UpdateCrontab           *string       `long:"update.crontab"                  env:"UPDATE_CRONTAB"                  description:"Crontab of check runs"                                   default:"@every 15m"`
	UpdateLimit             int           `long:"update.concurrency"              env:"UPDATE_CONCURRENCY"              description:"How many VMs should be updated concurrently"          default:"1"`
	UpdateLockDuration      time.Duration `long:"update.lock-duration"            env:"UPDATE_LOCK_DURATION"            description:"Duration how long should be waited for another update" default:"30m"`
	UpdateLockDurationError time.Duration `long:"update.lock-duration-error"      env:"UPDATE_LOCK_DURATION_ERROR"      description:"Duration how long should be waited for another update in case an error occurred" default:"5m"`
	UpdateNodeLockAnnotation string       `long:"update.lock-annotation"          env:"UPDATE_LOCK_ANNOTATION"          description:"Node annotation for update lock time"                                                                      default:"autopilot.webdevops.io/update-lock"`

	// notification
	Notification []string `long:"notification" env:"NOTIFCATION" description:"Shoutrrr url for notifications (https://containrrr.github.io/shoutrrr/)" env-delim:" "`

	// server settings
	ServerBind string `long:"bind" env:"SERVER_BIND"  description:"Server address"  default:":8080"`
}

func main() {
	initArgparser()

	log.Infof("starting Azure K8S cluster autopilot v%s (%s; %s; by %v)", gitTag, gitCommit, runtime.Version(), Author)
	startAzureK8sAutorepair()

	log.Infof("starting http server on %s", opts.ServerBind)
	startHttpServer()
}

// init argparser and parse/validate arguments
func initArgparser() {
	argparser = flags.NewParser(&opts, flags.Default)
	_, err := argparser.Parse()

	// check if there is an parse error
	if err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		} else {
			fmt.Println(err)
			fmt.Println()
			argparser.WriteHelp(os.Stdout)
			os.Exit(1)
		}
	}

	// verbose level
	if opts.Verbose {
		log.SetLevel(log.DebugLevel)
	}

	// verbose level
	if opts.LogJson {
		log.SetFormatter(&log.JSONFormatter{})
	}
}

// Init and build Azure authorzier
func startAzureK8sAutorepair() {
	autorepair := autopilot.AzureK8sAutopilot{}

	// general
	autorepair.DryRun = opts.DryRun

	// repair
	autorepair.Repair.Crontab = opts.RepairCrontab
	autorepair.Repair.NotReadyThreshold = &opts.RepairNotReadyThreshold
	autorepair.Repair.LockDuration = &opts.RepairLockDuration
	autorepair.Repair.LockDurationError = &opts.RepairLockDurationError
	autorepair.Repair.Limit = opts.RepairLimit
	autorepair.Repair.NodeLockAnnotation = opts.UpdateNodeLockAnnotation

	// upgrade
	autorepair.Update.Crontab = opts.UpdateCrontab
	autorepair.Update.LockDuration = &opts.UpdateLockDuration
	autorepair.Update.LockDurationError = &opts.UpdateLockDurationError
	autorepair.Update.Limit = opts.UpdateLimit
	autorepair.Update.NodeLockAnnotation = opts.UpdateNodeLockAnnotation

	// k8s
	autorepair.K8s.NodeLabelSelector = opts.K8sNodeLabelSelector

	// repair
	autorepair.Repair.VmssAction = opts.RepairAzureVmssAction
	autorepair.Repair.VmAction = opts.RepairAzureVmAction
	autorepair.Repair.ProvisioningState = opts.RepairProvisioningState

	// notification
	autorepair.Notification = opts.Notification

	autorepair.Init()
	autorepair.Run()
}

// start and handle prometheus handler
func startHttpServer() {
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(opts.ServerBind, nil))
}
