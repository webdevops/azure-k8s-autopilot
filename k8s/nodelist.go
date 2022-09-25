package k8s

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/patrickmn/go-cache"
	log "github.com/sirupsen/logrus"
	"github.com/webdevops/go-common/azuresdk/armclient"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

type (
	NodeList struct {
		NodeLabelSelector string
		Client            *kubernetes.Clientset
		AzureCacheTimeout *time.Duration

		AzureClient *armclient.ArmClient

		UserAgent string

		nodeWatcher watch.Interface
		azureCache  *cache.Cache
		ctx         context.Context
		list        map[string]*Node
		lock        sync.Mutex
		isStopped   bool
	}
)

func (n *NodeList) Start() {
	n.ctx = context.Background()
	if n.AzureCacheTimeout == nil {
		timeout := 10 * time.Minute
		n.AzureCacheTimeout = &timeout
	}

	n.azureCache = cache.New(*n.AzureCacheTimeout, 1*time.Minute)

	go func() {
		for {
			if n.isStopped {
				return
			}

			log.Info("(re)starting node watch")
			if err := n.startNodeWatch(); err != nil {
				log.Errorf("node watcher stopped: %v", err)
			}
		}
	}()
}

func (n *NodeList) Stop() {
	n.isStopped = true
	if n.nodeWatcher != nil {
		n.nodeWatcher.Stop()
	}
}

func (n *NodeList) ClearAzureCache() {
	log.Info("invalidating azure cache")
	n.azureCache.Flush()
}

func (n *NodeList) startNodeWatch() error {
	// init list
	n.lock.Lock()
	n.list = map[string]*Node{}
	n.lock.Unlock()

	timeout := int64(60 * 60 * 1)

	watchOpts := metav1.ListOptions{
		LabelSelector:  n.NodeLabelSelector,
		TimeoutSeconds: &timeout,
		Watch:          true,
	}
	nodeWatcher, err := n.Client.CoreV1().Nodes().Watch(n.ctx, watchOpts)
	if err != nil {
		log.Panic(err)
	}
	n.nodeWatcher = nodeWatcher
	defer nodeWatcher.Stop()

	for res := range nodeWatcher.ResultChan() {
		switch res.Type {
		// node added
		case watch.Added:
			n.lock.Lock()
			if node, ok := res.Object.(*corev1.Node); ok {
				node := &Node{Node: node, Client: n.Client}
				if node.IsAzureProvider() {
					n.list[node.Name] = node
				}
			}
			n.lock.Unlock()
		// node deleted
		case watch.Deleted:
			n.lock.Lock()
			if node, ok := res.Object.(*corev1.Node); ok {
				delete(n.list, node.Name)
			}
			n.lock.Unlock()
		// node modified
		case watch.Modified:
			n.lock.Lock()
			if node, ok := res.Object.(*corev1.Node); ok {
				node := &Node{Node: node, Client: n.Client}
				if node.IsAzureProvider() {
					n.list[node.Name] = node
				}
			}
			n.lock.Unlock()
		case watch.Error:
			log.Errorf("go watch error event %v", res.Object)
		}
	}

	return fmt.Errorf("terminated")
}

func (n *NodeList) Cleanup() {
	for _, v := range n.NodeList() {
		node := v
		node.Cleanup()
	}
}

func (n *NodeList) NodeList() (list []*Node) {
	list = []*Node{}

	n.lock.Lock()
	for _, v := range n.list {
		node := v
		list = append(list, node)
	}
	n.lock.Unlock()
	return
}

func (n *NodeList) NodeListWithAzure() (list []*Node, err error) {
	list = n.NodeList()

	if err := n.refreshAzureCache(); err != nil {
		return nil, err
	}

	for index, node := range list {
		if azureResource, exists := n.azureCache.Get(strings.ToLower(node.Spec.ProviderID)); exists {
			switch v := azureResource.(type) {
			case compute.VirtualMachineScaleSetVM:
				node.AzureVmss = &v
			}
		}
		list[index] = node
	}
	return
}

func (n *NodeList) refreshAzureCache() error {
	log.Infof("refresh azure cache")
	if err := n.refreshAzureVmssCache(); err != nil {
		return err
	}

	return nil
}

func (n *NodeList) refreshAzureVmssCache() error {
	vmssList, err := n.GetAzureVmssList()
	if err != nil {
		return err
	}

	for _, vmssInfo := range vmssList {
		vmssVmClient, err := armcompute.NewVirtualMachineScaleSetVMsClient(vmssInfo.Subscription, n.AzureClient.GetCred(), n.AzureClient.NewArmClientOptions())
		if err != nil {
			return err
		}

		pager := vmssVmClient.NewListPager(vmssInfo.ResourceGroup, vmssInfo.VMScaleSetName, nil)

		for pager.More() {
			result, err := pager.NextPage(n.ctx)
			if err != nil {
				return err
			}

			for _, vmssInstance := range result.Value {
				k8sProviderId := fmt.Sprintf(
					"azure://%s",
					strings.ToLower(*vmssInstance.ID),
				)

				n.azureCache.Delete(k8sProviderId)
				if err := n.azureCache.Add(k8sProviderId, vmssInstance, *n.AzureCacheTimeout); err != nil {
					log.Error(err)
				}
			}
		}
	}

	return nil
}

func (n *NodeList) NodeCountByProvisionState(provisionState string) (count int) {
	for _, node := range n.NodeList() {
		if node.AzureVmss != nil && node.AzureVmss.ProvisioningState != nil {
			if *node.AzureVmss.ProvisioningState == provisionState {
				count++
			}
		}
	}
	return
}

func (n *NodeList) GetAzureVmssList() (vmssList map[string]*NodeInfo, err error) {
	vmssList = map[string]*NodeInfo{}

	for _, node := range n.NodeList() {
		if node.IsAzureProvider() {
			// parse node information from provider ID
			nodeInfo, parseErr := ExtractNodeInfo(node)
			if parseErr != nil {
				err = parseErr
				return
			}

			if nodeInfo.IsVmss {
				vmssKey := fmt.Sprintf(
					"%s/%s/%s",
					nodeInfo.Subscription,
					nodeInfo.ResourceGroup,
					nodeInfo.VMScaleSetName,
				)
				vmssList[vmssKey] = nodeInfo
			}
		}
	}

	return
}
