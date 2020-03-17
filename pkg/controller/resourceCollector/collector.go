package resourcecollector

import (
    "fmt"
    "encoding/json"
    _ "github.com/influxdata/influxdb1-client"  // this is important because of the buf in go mod
    client "github.com/influxdata/influxdb1-client/v2"
    // resource "resource-controller/controllers/openmcpscheduler/pkg/controller/resourceInfo"
)

const (
    myDB = "resource"
    host = "10.0.3.20:8086"
    username = "admin"
    password = "ketilinux"
)

type Resource struct {
    MilliCPU	int64
    Memory	int64
}

type ClusterInfo struct {
    requestedResource *Resource
    capacityResource *Resource
}

var c client.Client	// influxDB client

func NewInfluxDBClient() {
    c, _ = client.NewHTTPClient(client.HTTPConfig{
		Addr:		"http://" + host,
		Username:	username,
		Password:	password,
    })
    /*
    if err != nil {
		fmt.Println("[DB] cannot create influxDB client!")
    }
    */
    defer c.Close()
}

func ClustersTotalCpuRequest(cluster string) float64{
  cmd := fmt.Sprintf("select sum(request) from cpu where cluster_name='%s'", cluster)
  res := getTotalResource(cmd)
  return res
}

func ClustersTotalMemoryRequest(cluster string) float64 {
    cmd := fmt.Sprintf("select sum(request) from memory where cluster_name='%s'", cluster)
    res := getTotalResource(cmd)
    return res
}

func getTotalResource(command string) float64{
    q := client.Query {
		  Command: command,
		  Database: myDB,
    }

    resp, err := c.Query(q)
    if err != nil || resp.Error() != nil {
		  fmt.Println("[DB] cannot get result")
    }

    res, err := resp.Results[0].Series[0].Values[0][1].(json.Number).Float64()
    if err != nil {
	    fmt.Println("[DB] cannot get value from database")
    }
    return res
}

/*
func main(){
    fmt.Println("[DB] create influxDBClient()")
    influxDBClient()
    fmt.Println("[DB] finish influxDBClient()")
    fmt.Println("test: ", getClustersTotalCpuRequest("cluster1"))
}
*/