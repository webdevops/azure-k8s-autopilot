package main

import (
	"fmt"
	"github.com/jessevdk/go-flags"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	Author = "webdevops.io"
)

var (
	argparser *flags.Parser
	Verbose   bool
	Logger    *DaemonLogger

	// Git version information
	gitCommit = "<unknown>"
	gitTag    = "<unknown>"
)

var opts struct {
	// general settings
	Verbose []bool `long:"verbose" short:"v" env:"VERBOSE"   description:"Verbose mode"`
	DryRun  bool   `long:"dry-run"           env:"DRY_RUN"   description:"Dry run (no redeploy triggered)"`

	// k8s
	K8sNodeLabelSelector string `long:"k8s.node.labelselector"   env:"K8S_NODE_LABELSELECTOR"           description:"Node Label selector which nodes should be checked"                                   default:""`

	// check settings
	RepairInterval            time.Duration `long:"repair.interval"              env:"REPAIR_INTERVAL"             description:"Duration of check run"                                   default:"30s"`
	RepairWaitDuration        time.Duration `long:"repair.waitduration"          env:"REPAIR_WAIT_DURATION"        description:"Duration to wait when redeploy will be triggered"        default:"10m"`
	RepairLimit               int           `long:"repair.concurrency"           env:"REPAIR_CONCURRENCY"          description:"How many VMs should be redeployed concurrently"          default:"1"`
	RepairLockDuration        time.Duration `long:"repair.lockduration"          env:"REPAIR_LOCK_DURATION"        description:"Duration how long should be waited for another redeploy" default:"30m"`
	RepairAzureVmssAction     string        `long:"repair.azure.vmss.action"     env:"REPAIR_AZURE_VMSS_ACTION"    description:"Defines the action which should be tried to repair the node (VMSS)" default:"redeploy" choice:"restart"  choice:"redeploy" choice:"reimage"`
	RepairAzureVmAction       string        `long:"repair.azure.vm.action"       env:"REPAIR_AZURE_VM_ACTION"      description:"Defines the action which should be tried to repair the node (VM)"   default:"redeploy" choice:"restart"  choice:"redeploy"`

	// server settings
	ServerBind string `long:"bind" env:"SERVER_BIND"  description:"Server address"  default:":8080"`
}

func main() {
	initArgparser()

	// set verbosity
	Verbose = len(opts.Verbose) >= 1

	Logger = NewLogger(log.Lshortfile, Verbose)
	defer Logger.Close()

	// set verbosity
	Verbose = len(opts.Verbose) >= 1

	Logger.Infof("Init Azure K8S cluster AutoRepair v%s (%s; by %v)", gitTag, gitCommit, Author)
	startAzureK8sAutorepair()

	Logger.Infof("Starting http server on %s", opts.ServerBind)
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
}

// Init and build Azure authorzier
func startAzureK8sAutorepair() {
	autorepair := K8sAutoRepair{}

	// general
	autorepair.Interval = &opts.RepairInterval
	autorepair.WaitDuration = &opts.RepairWaitDuration
	autorepair.LockDuration = &opts.RepairLockDuration
	autorepair.Limit = opts.RepairLimit
	autorepair.DryRun = opts.DryRun

	// k8s
	autorepair.K8s.NodeLabelSelector = opts.K8sNodeLabelSelector

	// repair
	autorepair.Repair.VmssAction = opts.RepairAzureVmssAction
	autorepair.Repair.VmAction = opts.RepairAzureVmAction

	autorepair.Init()
	autorepair.Run()
}

// start and handle prometheus handler
func startHttpServer() {
	http.Handle("/metrics", promhttp.Handler())
	Logger.Fatal(http.ListenAndServe(opts.ServerBind, nil))
}
