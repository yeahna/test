/*
Copyright 2018 The Multicluster-Controller Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package openmcpscheduler // import "admiralty.io/multicluster-controller/examples/openmcpscheduler/pkg/controller/openmcpscheduler"

import (
	"context"
	"encoding/json"
	"fmt"
//	"sort"
//	"math/rand"
//	"time"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/kubefed/pkg/controller/util"
	"admiralty.io/multicluster-controller/pkg/reference"

	"resource-controller/apis"
    ketiv1alpha1 "resource-controller/apis/keti/v1alpha1"
	"admiralty.io/multicluster-controller/pkg/cluster"
	"admiralty.io/multicluster-controller/pkg/controller"
	"admiralty.io/multicluster-controller/pkg/reconcile"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"k8s.io/klog"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/rest"
	genericclient "sigs.k8s.io/kubefed/pkg/client/generic"
	fedv1b1 "sigs.k8s.io/kubefed/pkg/apis/core/v1beta1"
//	v1 "k8s.io/api/core/v1"
//	"k8s.io/client-go/kubernetes"
	kubesource "k8s.io/apimachinery/pkg/api/resource"

	clusterInfo "resource-controller/controllers/openmcpscheduler/pkg/controller/resourceCollector"

//	"math"
	"strings"
	"strconv"
)

type ClusterManager struct {
	Fed_namespace string
    Host_config *rest.Config
    Host_client genericclient.Client
    Cluster_list *fedv1b1.KubeFedClusterList
    Cluster_configs map[string]*rest.Config
    Cluster_clients map[string]genericclient.Client
}

type NodeInfo struct {
	capacity *Resource
	requestedResource *Resource
	allocatableResource *Resource
}

type Resource struct {
	cpu float64
	memory float64
	storage float64
	network float64
}

type score_Info map[string]float64

func NewController(live *cluster.Cluster, ghosts []*cluster.Cluster, ghostNamespace string) (*controller.Controller, error) {
	liveclient, err := live.GetDelegatingClient()
	if err != nil {
		return nil, fmt.Errorf("getting delegating client for live cluster: %v", err)
	}

	// client.Client reads and writes directly from/to an API server
	ghostclients := []client.Client{}
	for _, ghost := range ghosts {

		// 4 test
		//klog.Infof("[HERE] %s", ghost.GetClusterName())

		// DelegatingClient forms a Client by composing separate reader, writer and statusclient interfaces.
		// This way, you can have an Client that reads from a cache and write to the API server.
		ghostclient, err := ghost.GetDelegatingClient()
		if err != nil {
			return nil, fmt.Errorf("getting delegating client for ghost cluster: %v", err)
		}
		ghostclients = append(ghostclients, ghostclient)
	}

	co := controller.New(&reconciler{live: liveclient, ghosts: ghostclients, ghostNamespace: ghostNamespace}, controller.Options{})
	if err := apis.AddToScheme(live.GetScheme()); err != nil {
	    return nil, fmt.Errorf("adding APIs to live cluster's scheme: %v", err)
    }

	if err := co.WatchResourceReconcileObject(live, &ketiv1alpha1.OpenMCPDeployment{}, controller.WatchOptions{}); err != nil {
		return nil, fmt.Errorf("setting up Pod watch in live cluster: %v", err)
	}

	// Note: At the moment, all clusters share the same scheme under the hood
	// (k8s.io/client-go/kubernetes/scheme.Scheme), yet multicluster-controller gives each cluster a scheme pointer.
	// Therefore, if we needed a custom resource in multiple clusters, we would redundantly
	// add it to each cluster's scheme, which points to the same underlying scheme.

	for _, ghost := range ghosts {
		//fmt.Printf("%T, %s\n", ghost, ghost.GetClusterName())
		if err := co.WatchResourceReconcileController(ghost, &appsv1.Deployment{}, controller.WatchOptions{}); err != nil {
			return nil, fmt.Errorf("setting up PodGhost watch in ghost cluster: %v", err)
		}
	}
	return co, nil
}


type reconciler struct {
	live           client.Client
	ghosts         []client.Client
	ghostNamespace string
}

// controller use events to eventually trigger reconcile requests.
// reconcile use clients to access API objects.
func (r *reconciler) Reconcile(req reconcile.Request) (reconcile.Result, error) {
	klog.Infof("[SCHED] Reconcile!!")
	cm := NewClusterManager()

	// get OpenMCPDeployment
    instance := &ketiv1alpha1.OpenMCPDeployment{}
    err := r.live.Get(context.TODO(), req.NamespacedName, instance)

	if err != nil {
		if errors.IsNotFound(err) {
			err := cm.DeleteDeployments(req.NamespacedName)
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, err
	}

	if instance.Status.ClusterMaps == nil {
		klog.Infof("[SCHED] Detected!")

		// need scheduling
		//if instance.Status.SchedulingNeed == true && instance.Status.SchedulingComplete == false {
			klog.Infof("[SCHED] Scheduling Start!")

			// get replicas
			replicas := instance.Spec.Replicas

			// get requested resource (CPU, Mem)
			cpu_unit := instance.Spec.Template.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu().String()
			quantity_change_cpu := kubesource.MustParse(instance.Spec.Template.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu().String())
			quantity_change_mem := kubesource.MustParse(instance.Spec.Template.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().String())
			temp_cpu, _ := quantity_change_cpu.AsInt64()
			temp_mem, _ := quantity_change_mem.AsInt64()

			Request_cpu := float64(temp_cpu)
			Request_mem := float64(temp_mem) / 1024 / 1024 / 1024

			if strings.Contains(cpu_unit, "m") {
				Request_cpu = milliTocoreCPU(cpu_unit)
			}

			//cluster_replicas_map, failSelected := cm.Scheduling(replicas, Request_cpu, Request_mem)
			cluster_replicas_map, _ := cm.Scheduling(replicas, Request_cpu, Request_mem)
			klog.Infof("[CHECK] CHECK!! %v", cluster_replicas_map)
			// create deployment
			for _, cluster := range cm.Cluster_list.Items {

				if cluster_replicas_map[cluster.Name] == 0{
					continue
				}

				found := &appsv1.Deployment{}
	            cluster_client := cm.Cluster_clients[cluster.Name]

				err = cluster_client.Get(context.TODO(), found, instance.Namespace, instance.Name+"-deploy")
				if err != nil && errors.IsNotFound(err) {
					replica := cluster_replicas_map[cluster.Name]
					fmt.Println("Cluster '" + cluster.Name  + "' Deployed (", replica, " / ", replicas, ")")

					dep := r.deploymentForOpenMCPDeployment(req, instance, replica)
					err = cluster_client.Create(context.Background(), dep)
					if err != nil {
						return reconcile.Result{}, err
					}
				}
			}
			instance.Status.ClusterMaps = cluster_replicas_map
			instance.Status.Replicas = replicas

			instance.Status.SchedulingNeed = false
			instance.Status.SchedulingComplete = true

			// update OpenMCPDeployment to deploy
			err := r.live.Status().Update(context.TODO(), instance)
			if err != nil {
				klog.Infof("Failed to update instance status", err)
				return reconcile.Result{}, err
			}
		//}
	}
	return reconcile.Result{}, nil
}

func (r *reconciler) deploymentForOpenMCPDeployment(req reconcile.Request, m *ketiv1alpha1.OpenMCPDeployment, replica int32) *appsv1.Deployment {

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
		   Name:      m.Name+"-deploy",
           Namespace: m.Namespace,
        },
		Spec: m.Spec.Template.Spec,
    }

	if dep.Spec.Selector == nil{
		dep.Spec.Selector = &metav1.LabelSelector{}
	}

	dep.Spec.Selector.MatchLabels = m.Spec.Labels
	dep.Spec.Template.ObjectMeta.Labels = m.Spec.Labels
	dep.Spec.Replicas = &replica

	reference.SetMulticlusterControllerReference(dep, reference.NewMulticlusterOwnerReference(m, m.GroupVersionKind(), req.Context))

	return dep
}

func isInObject(child *appsv1.Deployment, parent string) bool {
	refKind_str := child.ObjectMeta.Annotations["multicluster.admiralty.io/controller-reference"]
	refKind_map := make(map[string]interface{})
	err := json.Unmarshal([]byte(refKind_str), &refKind_map)
	if err!= nil{
		panic(err)
	}
	if refKind_map["kind"] == parent{
		return true
	}
	return false
}

func (cm *ClusterManager) DeleteDeployments(nsn types.NamespacedName) error {
	dep := &appsv1.Deployment{}

	for _, cluster := range cm.Cluster_list.Items {
	    cluster_client := cm.Cluster_clients[cluster.Name]
		err := cluster_client.Get(context.Background(), dep, nsn.Namespace, nsn.Name+"-deploy")

		if err != nil && errors.IsNotFound(err) {
			fmt.Println("Not Found")
			continue
		}

		if !isInObject(dep, "OpenMCPDeployment"){
			continue
		}

		err = cluster_client.Delete(context.Background(), dep, nsn.Namespace, nsn.Name+"-deploy")
		if err != nil {
			return err
		}
    }
	return nil
}

func ListKubeFedClusters(client genericclient.Client, namespace string) *fedv1b1.KubeFedClusterList {
		clusterList := &fedv1b1.KubeFedClusterList{}
		// List retrieves list of objects for a given namespace and list options
        err := client.List(context.TODO(), clusterList, namespace)
        if err != nil {
                fmt.Println("Error retrieving list of federated clusters: %+v", err)
        }
        if len(clusterList.Items) == 0 {
                fmt.Println("No federated clusters found")
        }
        return clusterList
}

func KubeFedClusterConfigs(clusterList *fedv1b1.KubeFedClusterList, client genericclient.Client, fedNamespace string) map[string]*rest.Config {
        clusterConfigs := make(map[string]*rest.Config)
        for _, cluster := range clusterList.Items {
                config, _ := util.BuildClusterConfig(&cluster, client, fedNamespace)
                clusterConfigs[cluster.Name] = config
        }
        return clusterConfigs
}
func KubeFedClusterClients(clusterList *fedv1b1.KubeFedClusterList, cluster_configs map[string]*rest.Config) map[string]genericclient.Client {

        cluster_clients := make(map[string]genericclient.Client)
        for _, cluster := range clusterList.Items {
                clusterName := cluster.Name
                cluster_config := cluster_configs[clusterName]
                cluster_client := genericclient.NewForConfigOrDie(cluster_config)
                cluster_clients[clusterName] = cluster_client
        }
        return cluster_clients
}

func NewClusterManager() *ClusterManager {
		fed_namespace := "kube-federation-system"
		// return a config object which uses the service account kubernetes gives to pods
		// *rest.Config is for talking to a Kubernetes apiserver.
		host_config, _ := rest.InClusterConfig()
        host_client := genericclient.NewForConfigOrDie(host_config)
        cluster_list := ListKubeFedClusters(host_client, fed_namespace)
        cluster_configs := KubeFedClusterConfigs(cluster_list, host_client, fed_namespace)
        cluster_clients := KubeFedClusterClients(cluster_list, cluster_configs)

        cm := &ClusterManager{
                Fed_namespace: fed_namespace,
                Host_config: host_config,
                Host_client: host_client,
                Cluster_list: cluster_list,
                Cluster_configs: cluster_configs,
                Cluster_clients: cluster_clients,
        }
        return cm
}

func ClusterResourceFilter(request_cpu float64, request_mem float64, clusterNames []string) []string {
	clusterList := []string{}

    for _, name := range clusterNames {
        totalCpu := clusterInfo.ClustersTotalCpuRequest(name)
        totalMem := clusterInfo.ClustersTotalMemoryRequest(name)
		//klog.Infof("[CHECK] request_cpu %f, totalCpu %f", request_cpu, totalCpu)

		// example data is wrong 0.6->0.06
        if totalCpu + request_cpu <= 12 && totalMem + request_mem <= 400 {
            clusterList = append(clusterList, name)
            klog.Infof("[FILTER] ", name, " can be scheduled")
        }
    }
    return clusterList
}

// Schedule tries to schedule the giben pod to one of the clusters in the cluster list.
// If it succeeds, it will return the name of the cluster
func(cm *ClusterManager) Scheduling(replicas int32, request_cpu float64, request_mem float64) (map[string]int32, int64) {
	klog.Infof("[SCHED] requestcpu: " ,request_cpu)
	klog.Infof("[SCHED] resquestmem: ", request_mem)

	// read Running Cluster Information
	// But it is a test case, insert cluster1~cluster50
	var i int = 0
	clusterList := []string{}
	for {
		clusterList = append(clusterList, fmt.Sprintf("cluster%d", (i+1)))
		i++
		if i == 49{
			break
		}
	}

	// return value
	replicas_cluster := map[string]int32{}
	var failSelected int64 = 0
	var scoreInfo map[string]float64 = make(map[string]float64)

	for {
		// Filtering
		// it is simple code for test
		filteredClusterList := ClusterResourceFilter(request_cpu, request_mem, clusterList)
		klog.Infof("[SCHED] Finish Filtering", filteredClusterList)

		// MostRequestedScoring
		// it is simple code for test
		for _, name := range filteredClusterList {
			scoreInfo[name] += clusterInfo.ClustersTotalCpuRequest(name)
        	scoreInfo[name] += clusterInfo.ClustersTotalMemoryRequest(name)
		}
		klog.Infof("[SCHED] Finish MostRequestedScoring")

		for _, name := range filteredClusterList {
			scoreInfo[name] += (clusterInfo.ClustersTotalCpuRequest(name) - request_cpu)
        	scoreInfo[name] += (clusterInfo.ClustersTotalMemoryRequest(name) - request_mem)
		}
		klog.Infof("[SCHED] Finish LeastRequestedScoring")

		replicas--
		klog.Infof("[SCHED] %v", replicas_cluster)

		if replicas == 0{
			break
		}
	}

	klog.Infof("[SCHED] %v", scoreInfo)

	// for test
	replicas_cluster["cluster2"] += 3
    return replicas_cluster, failSelected
}

func milliTocoreCPU(value string) float64 {
	temp := strings.Split(value, "m")
	milli_value, _ := strconv.Atoi(temp[0])
	core_value := float64(milli_value)
	Request_cpu := core_value / 1000
	return Request_cpu
}
