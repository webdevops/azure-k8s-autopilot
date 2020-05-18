package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/containrrr/shoutrrr"
	"github.com/muesli/cache2go"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"regexp"
	"strings"
	"time"
)

var (
	azureSubscriptionRegexp   = regexp.MustCompile("^azure:///subscriptions/([^/]+)/resourceGroups/.*")
	azureResourceGroupRegexp  = regexp.MustCompile("^azure:///subscriptions/[^/]+/resourceGroups/([^/]+)/.*")
	azureVmssNameRegexp       = regexp.MustCompile("/providers/Microsoft.Compute/virtualMachineScaleSets/([^/]+)/.*")
	azureVmssInstanceIdRegexp = regexp.MustCompile("/providers/Microsoft.Compute/virtualMachineScaleSets/[^/]+/virtualMachines/([^/]+)$")
	azureVmNameRegexp         = regexp.MustCompile("/providers/Microsoft.Compute/virtualMachines/([^/]+)$")
)

type K8sAutoRepair struct {
	Interval     *time.Duration
	NotReadyThreshold *time.Duration
	LockDuration *time.Duration
	Limit        int
	DryRun       bool

	K8s struct {
		NodeLabelSelector string
	}

	Repair struct {
		VmssAction     string
		VmAction       string
	}

	Notification []string

	azureAuthorizer autorest.Authorizer
	k8sClient       *kubernetes.Clientset

	cache *cache2go.CacheTable
}

type K8sAutoRepairNodeAzureInfo struct {
	NodeName string
	NodeProviderId string

	Subscription  string
	ResourceGroup string

	IsVmss         bool
	VMScaleSetName string
	VMInstanceID   string

	VMname string
}

func (r *K8sAutoRepair) Init() {
	r.initAzure()
	r.initK8s()
	r.cache = cache2go.Cache("nodeCache")
}

func (r *K8sAutoRepair) initAzure() {
	var err error

	// setup azure authorizer
	r.azureAuthorizer, err = auth.NewAuthorizerFromEnvironment()
	if err != nil {
		panic(err)
	}
}

func (r *K8sAutoRepair) initK8s() {
	var err error
	var config *rest.Config

	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		// KUBECONFIG
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			panic(err.Error())
		}
	} else {
		// K8S in cluster
		config, err = rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
	}

	r.k8sClient, err = kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
}

func (r *K8sAutoRepair) Run() {
	go func() {
		for {
			time.Sleep(*r.Interval)
			Logger.Infoln("Checking cluster nodes")
			start := time.Now()
			r.checkAndRepairCluster()
			runtime := time.Now().Sub(start)
			Logger.Infof("Finished after %s", runtime.String())
		}
	}()
}

func (r *K8sAutoRepair) checkAndRepairCluster() {
	nodeList, err := r.getNodeList()

	if err != nil {
		Logger.Errorf("Unable to fetch K8s Node list: %v", err.Error())
		return
	}

	repairThresholdSeconds := r.NotReadyThreshold.Seconds()

	Logger.Verbosef("Found %v nodes in cluster", len(nodeList.Items))
	Logger.Verbosef("%v locked nodes (repair lock)", r.cache.Count())

nodeLoop:
	for _, node := range nodeList.Items {
		Logger.Verbosef("Checking node %v", node.Name)

		// detect if node is ready/healthy
		nodeIsHealthy := true
		nodeLastHeartbeatAge := float64(0)
		nodeLastHeartbeat := "<unknown>"
		for _, condition := range node.Status.Conditions {
			if condition.Type == "Ready" && condition.Status != "True" {
				nodeIsHealthy = false
				nodeLastHeartbeat = condition.LastHeartbeatTime.Time.String()
				nodeLastHeartbeatAge = time.Now().Sub(condition.LastHeartbeatTime.Time).Seconds()
			}
		}

		if !nodeIsHealthy {
			// node is NOT healthy

			// ignore cordoned nodes, maybe maintenance work in progress
			if node.Spec.Unschedulable {
				Logger.Infof("Detected unhealthy node %s, ignoring because node is cordoned", node.Name)
				continue nodeLoop
			}

			// check if heartbeat already exceeded threshold
			if nodeLastHeartbeatAge < repairThresholdSeconds {
				Logger.Infof("Detected unhealthy node %s (last heartbeat: %s), deadline not reached", node.Name, nodeLastHeartbeat)
				continue nodeLoop
			}

			nodeProviderId := node.Spec.ProviderID
			if strings.HasPrefix(nodeProviderId, "azure://") {
				// is an azure node

				var err error
				ctx := context.Background()

				// redeploy timeout lock
				if _, err = r.cache.Value(node.Name); err == nil {
					Logger.Infof("Detected unhealthy node %s (last heartbeat: %s), waiting for repair (locked)", node.Name, nodeLastHeartbeat)
					continue nodeLoop
				}

				// concurrency repair limit
				if r.Limit > 0 && r.cache.Count() >= r.Limit {
					Logger.Infof("Detected unhealthy node %s (last heartbeat: %s), skipping due to concurrent repair limit", node.Name, nodeLastHeartbeat)
					continue nodeLoop
				}

				Logger.Infof("Detected unhealthy node %s (last heartbeat: %s), starting repair", node.Name, nodeLastHeartbeat)

				// parse node informations from provider ID
				nodeInfo, err := r.buildNodeInfo(&node)
				if err != nil {
					Logger.Errorln(err.Error())
					continue nodeLoop
				}

				if r.DryRun {
					Logger.Infof("Node %s repair skipped, dry run", node.Name)
					r.cache.Add(node.Name, *r.LockDuration, true)
					continue nodeLoop
				}

				if nodeInfo.IsVmss {
					// node is VMSS instance
					err = r.repairAzureVmssInstance(ctx, *nodeInfo)
				} else {
					// node is a VM
					err = r.repairAzureVm(ctx, *nodeInfo)
				}

				if err != nil {
					Logger.Errorf("Node %s repair failed: %s", node.Name, err.Error())
					continue nodeLoop
				} else {
					// lock vm for next redeploy, can take up to 15 mins
					r.cache.Add(node.Name, *r.LockDuration, true)
					Logger.Infof("Node %s successfully scheduled for repair", node.Name)
				}
			}
		} else {
			// node is NOT healthy
			Logger.Verbosef("Detected healthy node %s", node.Name)
			r.cache.Delete(node.Name) //nolint:golint,errcheck
		}
	}
}

func (r *K8sAutoRepair) buildNodeInfo(node *v1.Node) (*K8sAutoRepairNodeAzureInfo, error) {
	nodeProviderId := node.Spec.ProviderID

	info := K8sAutoRepairNodeAzureInfo{}
	info.NodeName = node.Name
	info.NodeProviderId = nodeProviderId

	// extract Subscription
	if match := azureSubscriptionRegexp.FindStringSubmatch(nodeProviderId); len(match) == 2 {
		info.Subscription = match[1]
	} else {
		return nil, errors.New(fmt.Sprintf("Unable to detect Azure Subscription from Node ProviderId (Azure resource ID): %v", nodeProviderId))
	}

	// extract ResourceGroup
	if match := azureResourceGroupRegexp.FindStringSubmatch(nodeProviderId); len(match) == 2 {
		info.ResourceGroup = match[1]
	} else {
		return nil, errors.New(fmt.Sprintf("Unable to detect Azure ResourceGroup from Node ProviderId (Azure resource ID): %v", nodeProviderId))
	}

	if strings.Contains(nodeProviderId, "/Microsoft.Compute/virtualMachineScaleSets/") {
		// Is VMSS
		info.IsVmss = true

		// extract VMScaleSetName
		if match := azureVmssNameRegexp.FindStringSubmatch(nodeProviderId); len(match) == 2 {
			info.VMScaleSetName = match[1]
		} else {
			return nil, errors.New(fmt.Sprintf("Unable to detect Azure VMScaleSetName from Node ProviderId (Azure resource ID): %v", nodeProviderId))
		}

		// extract VmssInstanceId
		if match := azureVmssInstanceIdRegexp.FindStringSubmatch(nodeProviderId); len(match) == 2 {
			info.VMInstanceID = match[1]
		} else {
			return nil, errors.New(fmt.Sprintf("Unable to detect Azure VmssInstanceId from Node ProviderId (Azure resource ID): %v", nodeProviderId))
		}
	} else {
		// Is VM
		info.IsVmss = false

		// extract VMname
		if match := azureVmNameRegexp.FindStringSubmatch(nodeProviderId); len(match) == 2 {
			info.VMname = match[1]
		} else {
			return nil, errors.New(fmt.Sprintf("Unable to detect Azure VMname from Node ProviderId (Azure resource ID): %v", nodeProviderId))
		}
	}

	return &info, nil
}

func (r *K8sAutoRepair) repairAzureVmssInstance(ctx context.Context, nodeInfo K8sAutoRepairNodeAzureInfo) error {
	var err error
	vmssInstanceIds := compute.VirtualMachineScaleSetVMInstanceIDs{
		InstanceIds: &[]string{nodeInfo.VMInstanceID},
	}

	vmssInstanceReimage := compute.VirtualMachineScaleSetReimageParameters{
		InstanceIds: &[]string{nodeInfo.VMInstanceID},
	}

	client := compute.NewVirtualMachineScaleSetsClient(nodeInfo.Subscription)
	client.Authorizer = r.azureAuthorizer

	Logger.Infof("Scheduling Azure VMSS instance for %s: %s", r.Repair.VmssAction, nodeInfo.NodeProviderId)
	r.sendNotificationf("Trigger automatic repair of K8s node %v (action: %v)", nodeInfo.NodeName, r.Repair.VmssAction)

	switch r.Repair.VmssAction {
	case "restart":
		_, err = client.Restart(ctx, nodeInfo.ResourceGroup, nodeInfo.VMScaleSetName, &vmssInstanceIds)
	case "redeploy":
		_, err = client.Redeploy(ctx, nodeInfo.ResourceGroup, nodeInfo.VMScaleSetName, &vmssInstanceIds)
	case "reimage":
		_, err = client.Reimage(ctx, nodeInfo.ResourceGroup, nodeInfo.VMScaleSetName, &vmssInstanceReimage)
	}
	return err
}

func (r *K8sAutoRepair) repairAzureVm(ctx context.Context, nodeInfo K8sAutoRepairNodeAzureInfo) error {
	var err error

	client := compute.NewVirtualMachinesClient(nodeInfo.Subscription)
	client.Authorizer = r.azureAuthorizer

	Logger.Infof("Scheduling Azure VM for %s: %s", r.Repair.VmAction, nodeInfo.NodeProviderId)
	r.sendNotificationf("Trigger automatic repair of K8s node %v (action: %v)", nodeInfo.NodeName, r.Repair.VmAction)

	switch r.Repair.VmAction {
	case "restart":
		_, err = client.Restart(ctx, nodeInfo.ResourceGroup, nodeInfo.VMname)
	case "redeploy":
		_, err = client.Redeploy(ctx, nodeInfo.ResourceGroup, nodeInfo.VMname)
	}
	return err
}

func (r *K8sAutoRepair) getNodeList() (*v1.NodeList, error) {
	opts := metav1.ListOptions{}
	opts.LabelSelector = r.K8s.NodeLabelSelector
	list, err := r.k8sClient.CoreV1().Nodes().List(opts)
	if err != nil {
		return list, err
	}

	// fetch all nodes
	for {
		if list.RemainingItemCount == nil || *list.RemainingItemCount == 0 {
			break
		}

		opts.Continue = list.Continue

		remainList, err := r.k8sClient.CoreV1().Nodes().List(opts)
		if err != nil {
			return list, err
		}

		list.Continue = remainList.Continue
		list.RemainingItemCount = remainList.RemainingItemCount
		list.Items = append(list.Items, remainList.Items...)
	}

	return list, nil
}

func (r *K8sAutoRepair) sendNotificationf(message string, args ...interface{}) () {
	r.sendNotification(fmt.Sprintf(message, args...))
}

func (r *K8sAutoRepair) sendNotification(message string) () {
	for _, url := range r.Notification {
		if err := shoutrrr.Send(url, message); err != nil {
			Logger.Errorf("Unable to send shoutrrr notification: %v", err.Error())
		}
	}
}
