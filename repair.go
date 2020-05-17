package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/muesli/cache2go"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
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
	WaitDuration *time.Duration
	LockDuration *time.Duration
	Limit        int64
	DryRun       bool

	azureAuthorizer autorest.Authorizer
	k8sClient       *kubernetes.Clientset

	cache *cache2go.CacheTable
}

type K8sAutoRepairNodeAzureInfo struct {
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
			Logger.Infoln("Checking cluster nodes")
			r.checkAndRepairCluster()
			Logger.Infoln("Checking cluster finished")
			time.Sleep(*r.Interval)
		}
	}()
}

func (r *K8sAutoRepair) checkAndRepairCluster() {
	nodeList, err := r.getNodeList()

	if err != nil {
		Logger.Errorln(fmt.Sprintf("Unable to fetch K8s Node list: %v", err.Error()))
		return
	}

	redeployTriggerSeconds := r.WaitDuration.Seconds()

	repairCount := int64(0)

nodeLoop:
	for _, node := range nodeList.Items {
		for _, condition := range node.Status.Conditions {
			if condition.Type == "Ready" && condition.Status != "True" {
				// ignore cordoned nodes, maybe maintenance work in progress
				if node.Spec.Unschedulable {
					Logger.Info(fmt.Sprintf("Detected unhealthy node %s, ignoring because node is cordoned", node.Name))
					continue nodeLoop
				}

				// check if heartbeat already exceeded threshold
				nodeHeartbeatSinceSeconds := time.Now().Sub(condition.LastHeartbeatTime.Time).Seconds()
				if nodeHeartbeatSinceSeconds < redeployTriggerSeconds {
					continue nodeLoop
				}

				nodeProviderId := node.Spec.ProviderID
				if strings.HasPrefix(nodeProviderId, "azure://") {
					var err error
					ctx := context.Background()
					// is an azure node
					repairCount++

					// concurrency repair limit
					if r.Limit > 0 && repairCount > r.Limit {
						Logger.Info(fmt.Sprintf("Detected unhealthy node %s (last heartbeat: %s), skipping due to concurrency limit", node.Name, condition.LastHeartbeatTime.Time))
						continue nodeLoop
					}

					// redeploy timeout lock
					if _, err = r.cache.Value(node.Name); err == nil {
						Logger.Info(fmt.Sprintf("Detected unhealthy node %s (last heartbeat: %s), waiting for redeploy (locked)", node.Name, condition.LastHeartbeatTime.Time))
						continue nodeLoop
					}

					Logger.Info(fmt.Sprintf("Detected unhealthy node %s (last heartbeat: %s), starting redeploy", node.Name, condition.LastHeartbeatTime.Time))

					// parse node informations from provider ID
					nodeInfo, err := r.parseNodeProviderId(nodeProviderId)
					if err != nil {
						Logger.Errorln(err.Error())
						continue nodeLoop
					}

					if opts.DryRun {
						Logger.Infoln(fmt.Sprintf("Node %s redeployment skipped, dry run", node.Name))
						continue nodeLoop
					}

					if nodeInfo.IsVmss {
						// node is VMSS instance
						err = r.redeployAzureVmssInstance(ctx, *nodeInfo)
					} else {
						// node is a VM
						err = r.redeployAzureVm(ctx, *nodeInfo)
					}

					if err != nil {
						Logger.Errorln(fmt.Sprintf("Node %s redeployment failed: %s", node.Name, err.Error()))
						continue nodeLoop
					} else {
						// lock vm for next redeploy, can take up to 15 mins
						r.cache.Add(node.Name, *r.LockDuration, true)
						Logger.Infoln(fmt.Sprintf("Node %s successfully scheduled redeployment of VM", node.Name))
					}
				}
			}
		}
	}
}

func (r *K8sAutoRepair) parseNodeProviderId(nodeProviderId string) (*K8sAutoRepairNodeAzureInfo, error) {
	info := K8sAutoRepairNodeAzureInfo{}

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

func (r *K8sAutoRepair) redeployAzureVmssInstance(ctx context.Context, nodeInfo K8sAutoRepairNodeAzureInfo) error {
	vmssInstanceIds := compute.VirtualMachineScaleSetVMInstanceIDs{
		InstanceIds: &[]string{nodeInfo.VMInstanceID},
	}

	client := compute.NewVirtualMachineScaleSetsClient(nodeInfo.Subscription)
	client.Authorizer = r.azureAuthorizer
	_, err := client.Redeploy(ctx, nodeInfo.ResourceGroup, nodeInfo.VMScaleSetName, &vmssInstanceIds)
	return err
}

func (r *K8sAutoRepair) redeployAzureVm(ctx context.Context, nodeInfo K8sAutoRepairNodeAzureInfo) error {
	client := compute.NewVirtualMachinesClient(nodeInfo.Subscription)
	client.Authorizer = r.azureAuthorizer
	_, err := client.Redeploy(ctx, nodeInfo.ResourceGroup, nodeInfo.VMname)
	return err
}

func (r *K8sAutoRepair) getNodeList() (*v1.NodeList, error) {
	opts := metav1.ListOptions{}
	return r.k8sClient.CoreV1().Nodes().List(opts)
}
