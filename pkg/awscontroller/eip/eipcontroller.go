package eip

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/glog"
	"time"
	"github.com/kopeio/eip-controller/pkg/kope/kopeaws"
)

type ElasticIPController struct {
	cloud       *kopeaws.AWSCloud

	period      time.Duration

	instances   map[string]*instance
	sequence    int

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
	ID               string
	sequence         int
	status           *ec2.Instance

	// canHaveElasticIP indicates whether this instance is suitable for an elastic ip
	canHaveElasticIP bool

	// elasticIP is the elastic IP assigned, if one is assigned
	elasticIP        string
}

func (c *ElasticIPController) Run() {
	for {
		if err := c.runOnce(); err != nil {
			glog.Errorf("error running sync loop: %v", err)
		}
		time.Sleep(c.period)
	}
}

func (c *ElasticIPController) runOnce() error {
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
	}

	if err != nil {
		return fmt.Errorf("error doing EC2 describe instances: %v", err)
	}

	addressMap := make(map[string]*instance)

	for _, i := range c.instances {
		id := i.ID

		if i.sequence != sequence {
			glog.Infof("Instance deleted: %q", id)
			delete(c.instances, id)
			continue
		}

		instanceStateName := aws.StringValue(i.status.State.Name)
		switch instanceStateName {
		case "pending":
			i.canHaveElasticIP = false
		case "running":
			i.canHaveElasticIP = true
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

		if !i.canHaveElasticIP && i.elasticIP != "" {
			found := false
			for _, ni := range i.status.NetworkInterfaces {
				if ni.Association == nil || aws.StringValue(ni.Association.PublicIp) != i.elasticIP {
					continue
				}

				found = true
				err := c.cloud.DisassociateAddress(i.ID, i.elasticIP, aws.StringValue(ni.Attachment.AttachmentId))
				if err != nil {
					glog.Warningf("failed to remove address %q from %q: %v", i.elasticIP, i.ID, err)
				} else {
					// Update the status in-place
					i.elasticIP = ""
				}
			}
			if !found {
				glog.Warningf("Want to disassociate address %q from %q, but was not found", i.elasticIP, i.ID)
			}
		}

		if i.elasticIP != "" {
			addressMap[i.elasticIP] = i
		}

	}

	glog.Infof("Found %d instances", len(c.instances))

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
			if i.elasticIP == "" {
				chosen = i
				break
			}
		}

		if chosen == nil {
			glog.Warningf("No instance available to assign EIP: %q", eip)
			continue
		}

		// TODO: Support associating with a secondary interface??

		err := c.cloud.AssociateAddress(chosen.ID, eip, aws.StringValue(address.AllocationId))
		if err != nil {
			glog.Warningf("failed to assign address %q to %q: %v", eip, chosen.ID, err)
		} else {
			chosen.elasticIP = eip
		}
	}

	return nil
}
