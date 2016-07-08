package eip

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/glog"
	"github.com/kopeio/eip-controller/pkg/kope/kopeaws"
	"time"
)

type ElasticIPController struct {
	cloud *kopeaws.AWSCloud

	period time.Duration

	instances map[string]*instance
	sequence  int

	// addressPool holds the set of addresses we will assign to instances
	addressPool map[string]*ec2.Address
}

func NewElasticIPController(cloud *kopeaws.AWSCloud, period time.Duration, elasticIPs []string) (*ElasticIPController, error) {
	c := &ElasticIPController{
		cloud:     cloud,
		instances: make(map[string]*instance),
		period:    period,
	}

	allAddresses, err := cloud.DescribeAddresses()
	if err != nil {
		return nil, err
	}

	allAddressesMap := make(map[string]*ec2.Address)
	for _, address := range allAddresses {
		ip := aws.StringValue(address.PublicIp)
		allAddressesMap[ip] = address
	}

	c.addressPool = make(map[string]*ec2.Address)
	for _, eip := range elasticIPs {
		address := allAddressesMap[eip]
		if address == nil {
			return nil, fmt.Errorf("address not found: %q", eip)
		}
		c.addressPool[eip] = address
	}
	return c, nil
}

// instance describes the state of an EC2 instance that we know about
type instance struct {
	ID string

	// sequence is a counter, so we can delete instances
	sequence int

	// status is the status from EC2, from the most recent call to DescribeInstances
	status *ec2.Instance

	// poolNode is true if this is a suitable node
	poolNode bool

	// canHaveElasticIP indicates whether this instance is suitable for an elastic ip based on its current state
	canHaveElasticIP bool

	// goodness is a heuristic to try to assign to more healthy nodes
	// The higher the score, the better a candidate it is
	goodness int

	// elasticIPs is the list of elastic IP assigned
	elasticIPs map[string]bool
}

func (c *ElasticIPController) Run() {
	for {
		// TODO: Watch the nodes API and poll immediately if we see a node change state?
		if err := c.runOnce(); err != nil {
			glog.Errorf("error running sync loop: %v", err)
		}
		time.Sleep(c.period)
	}
}

func (c *ElasticIPController) runOnce() error {
	// TODO: This is expensive when we have a large cluster
	instances, err := c.cloud.DescribeInstances()
	if err != nil {
		return err
	}

	c.sequence = c.sequence + 1
	sequence := c.sequence

	for _, awsInstance := range instances {
		id := aws.StringValue(awsInstance.InstanceId)
		if id == "" {
			glog.Warningf("skipping instance with empty instanceid: %v", awsInstance)
			continue
		}

		i := c.instances[id]
		if i == nil {
			i = &instance{
				ID: id,
			}
			c.instances[id] = i
		}

		i.status = awsInstance
		i.sequence = sequence

		elasticIPs := make(map[string]bool)
		for _, ni := range i.status.NetworkInterfaces {
			if ni.Association == nil || aws.StringValue(ni.Association.PublicIp) == "" {
				continue
			}

			eip := aws.StringValue(ni.Association.PublicIp)
			if c.addressPool[eip] == nil {
				// Only consider EIPs in the pool
				// TODO: This isn't quite right - what if there are other EIPs.
				// but we need to distinguish EIPs from auto-assigned public IPs
				// We _could_ just keep a map of all EIPs
				continue
			}

			elasticIPs[eip] = true
		}
		i.elasticIPs = elasticIPs

		master := false
		for _, tag := range awsInstance.Tags {
			tagKey := aws.StringValue(tag.Key)
			if tagKey == "k8s.io/role/master" {
				glog.V(2).Infof("instance %q is master; won't treat as part of pool", id)
				master = true
			}
		}

		// TODO: More criteria other than not assigning to the master?
		i.poolNode = !master
	}

	if err != nil {
		return fmt.Errorf("error doing EC2 describe instances: %v", err)
	}

	addressMap := make(map[string]*instance)

	poolNodeCount := 0
	canHaveElasticIPCount := 0
	for _, i := range c.instances {
		id := i.ID

		if i.sequence != sequence {
			glog.Infof("Instance deleted: %q", id)
			delete(c.instances, id)
			continue
		}

		i.goodness = 0
		instanceStateName := aws.StringValue(i.status.State.Name)
		switch instanceStateName {
		case "pending":
			i.canHaveElasticIP = false
		case "running":
			i.canHaveElasticIP = true
			i.goodness++
		case "shutting-down":
			i.canHaveElasticIP = false
		case "terminated":
			i.canHaveElasticIP = false
		case "stopping":
			i.canHaveElasticIP = false
		case "stopped":
			i.canHaveElasticIP = false

		default:
			glog.Warningf("unknown instance state for instance %q: %q", id, instanceStateName)
		}

		if i.poolNode {
			poolNodeCount++
			if i.canHaveElasticIP {
				canHaveElasticIPCount++
			}
		}

		// We will still detach our managed IPs from nodes that are no longer in pool
		canHaveElasticIP := i.poolNode && i.canHaveElasticIP

		if !canHaveElasticIP && len(i.elasticIPs) != 0 {
			if !i.poolNode {
				glog.Infof("Node %q no longer part of pool; will remove elastic ips", id)
			}
			if !i.canHaveElasticIP {
				glog.Infof("Node %q state is %q; will remove elastic ips", id, instanceStateName)
			}

			for elasticIP := range i.elasticIPs {
				foundNetworkInterface := false
				for _, ni := range i.status.NetworkInterfaces {
					if ni.Association == nil || aws.StringValue(ni.Association.PublicIp) != elasticIP {
						continue
					}

					foundNetworkInterface = true

					address, err := c.cloud.DescribeAddress(elasticIP)
					if err != nil {
						glog.Warningf("failed to describe address %q: %v", elasticIP, err)
						continue
					}

					err = c.cloud.DisassociateAddress(i.ID, elasticIP, aws.StringValue(address.AssociationId))
					if err != nil {
						glog.Warningf("failed to remove address %q from %q: %v", elasticIP, i.ID, err)
						continue
					} else {
						// Update the status in-place
						delete(i.elasticIPs, elasticIP)
					}
				}
				if !foundNetworkInterface {
					glog.Warningf("Want to disassociate address %q from %q, but was not found", elasticIP, i.ID)
				}
			}
		}

		for eip := range i.elasticIPs {
			addressMap[eip] = i
		}

		glog.V(2).Infof("Instance %q is in the pool; state=%q; eips=%v", id, instanceStateName, i.elasticIPs)
	}

	glog.Infof("Found %d instances, %d are in the pool, %d can have elastic ips", len(c.instances), poolNodeCount, canHaveElasticIPCount)

	// Now make sure that all our ips are assigned
	for eip, address := range c.addressPool {
		i := addressMap[eip]
		if i != nil {
			glog.V(2).Infof("EIP %q is assigned to %q", eip, i.ID)
			continue
		}

		// Pick an instance
		var chosen *instance
		for _, i := range c.instances {
			// Don't assign to the master or other non-pool nodes
			if !i.poolNode {
				continue
			}

			// Don't assign to nodes that aren't in an appropriate state
			if !i.canHaveElasticIP {
				continue
			}

			// Don't pick an instance which already has EIPs
			// TODO: we could actually support multiple interfaces
			if len(i.elasticIPs) != 0 {
				continue
			}

			if chosen == nil || i.goodness > chosen.goodness {
				chosen = i
			}
		}

		if chosen == nil {
			glog.Warningf("No instance available to assign EIP: %q", eip)
			continue
		}

		glog.Infof("Assigning IP %q to instance %q", eip, chosen.ID)

		// TODO: Support associating with a secondary interface??
		err := c.cloud.AssociateAddress(chosen.ID, eip, aws.StringValue(address.AllocationId))
		if err != nil {
			glog.Warningf("failed to assign address %q to %q: %v", eip, chosen.ID, err)
		} else {
			chosen.elasticIPs[eip] = true
		}
	}

	return nil
}
