package resourceinfo

import (
	//"k8s.io/klog"
)

var (
	emptyResource = Resource{}
)

// cluster Level
type ClusterInfo struct {
	clusterName			string
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

func (n *NodeInfo) nodeRequestedResource() Resource {
	if n == nil {
		return emptyResource
	}
	return *n.requestedResource
}

func (n *NodeInfo) SetNodeRequestedResource(newResource *Resource) {
	n.requestedResource = newResource
}