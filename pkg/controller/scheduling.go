package openmcpscheduler

import (
	"fmt"
	"time"
	"k8s.io/klog"
	kubesource "k8s.io/apimachinery/pkg/api/resource"
	ketiv1alpha1 "resource-controller/apis/keti/v1alpha1"

	_ "github.com/influxdata/influxdb1-client"  // this is important because of the buf in go mod
    client "github.com/influxdata/influxdb1-client/v2"
)

type ClusterInfo struct {
	requestedResource 	*Resource
	allocatableResource	*Resource

	nodeList			map[string]NodeInfo
}

// node Level
type NodeInfo struct {
	nodeName			string
	requestedResource 	*Resource
	allocatableResource	*Resource
}

type Resource struct {
	MilliCPU	int64
	Memory		int64
	Storage		int64
	Network		int64
}

var clusters map[string]ClusterInfo

func(cm *ClusterManager) Scheduling(pod *ketiv1alpha1.OpenMCPDeployment) map[string]int32 {
	klog.Infof("*********** Scheduling ***********")

	// return value (ex. cluster1:2, cluster2:1)
	replicas_cluster := map[string]int32{}

	// get data from influxdb
	getResources()

	// make resource to schedule pod into cluster
	newResource := newResourceFromPod(pod)
	klog.Infof("sdf", newResource)

	// temporary for test
	clusterList := []string{}
	for {
		clusterList = append(clusterList, fmt.Sprintf("cluster%d", (i+1)))
		i++
		if i == 49{
			break
		}
	}

	startTime := time.Now()
	elapsedTime := time.Since(startTime)
	klog.Infof("*********** %s ***********", elapsedTime)

	return replicas_cluster
}

func newResourceFromPod(pod *ketiv1alpha1.OpenMCPDeployment) *Resource {
	res := &Resource{}
	
	//_, container := range pod.Spec.Template.Spec.Template.Spec.Containers 
	cpu := kubesource.MustParse(pod.Spec.Template.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu().String())
	memory := kubesource.MustParse(pod.Spec.Template.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().String())

	res.MilliCPU = cpu.MilliValue()
	res.Memory = memory.Value()
	
	return res
}

const (
    myDB = "resource"
    host = "10.0.3.20:8086"
    username = "admin"
    password = "ketilinux"
)

func getResources() {
	clusters = append(clusters, cluster)

	var c client.Client
	c, _ = client.NewHTTPClient(client.HTTPConfig{
		Addr:		"http://" + host,
		Username:	username,
		Password:	password,
    })

	defer c.Close()
	
	q := client.Query {
		Command: "select cluster_name,node_name,pod_name,request from cpu",
		Database: myDB,
	}

	resp, err := c.Query(q)
	if err != nil || resp.Error() != nil {
		fmt.Println("[DB] cannot get result")
	}

	//     res, err := resp.Results[0].Series[0].Values[0][1].(json.Number).Int64()
	for i, value := range resp.Results[0].Series[0].Values {
		 clusters[value[i][1]]
	}
}