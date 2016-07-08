package main

import (
	"flag"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/spf13/pflag"

	"github.com/kopeio/eip-controller/pkg/eipcontroller/eip"
	"github.com/kopeio/eip-controller/pkg/kope/kopeaws"
)

var (
	flags = pflag.NewFlagSet("", pflag.ExitOnError)

	resyncPeriod = flags.Duration("sync-period", 30*time.Second,
		`Relist and confirm cloud resources this often.`)

	flagClusterID  = flags.String("cluster-id", "", "cluster-id")
	flagElasticIPs = flags.StringSlice("eip", []string{}, "specify elastic ips to assign")
)

func main() {
	flags.AddGoFlagSet(flag.CommandLine)

	// Workaround for glog
	flag.Set("logtostderr", "true")
	flag.CommandLine.Parse([]string{})

	flags.Parse(os.Args)

	cloud, err := kopeaws.NewAWSCloud()
	if err != nil {
		glog.Fatalf("error building cloud: %v", err)
	}

	clusterID := *flagClusterID
	if clusterID == "" && cloud != nil {
		clusterID = cloud.ClusterID()
	}
	if clusterID == "" {
		glog.Fatalf("cluster-id flag must be set")
	}

	elasticIPs := *flagElasticIPs
	if len(elasticIPs) == 0 {
		glog.Fatalf("must specify at least one elastic ip with --eip")
	}

	c, err := eip.NewElasticIPController(cloud, *resyncPeriod, elasticIPs)
	if err != nil {
		glog.Fatalf("error building elastic ip controller: %v", err)
	}

	c.Run()
}
