package exporters

import (
	"math"
	"strconv"

	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/agents"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/floatingips"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/networkipavailabilities"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/subnetpools"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/subnets"
	"github.com/prometheus/client_golang/prometheus"
	"inet.af/netaddr"
)

// BaseNeutronExporter : extends BaseOpenStackExporter
type BaseNeutronExporter struct {
	BaseOpenStackExporter
}

var BasedefaultNeutronMetrics = []Metric{
	{Name: "floating_ips", Fn: BaseListFloatingIps},
	{Name: "floating_ips_associated_not_active"},
	{Name: "floating_ip", Labels: []string{"id", "floating_network_id", "router_id", "status", "project_id", "floating_ip_address"}},
	{Name: "networks", Fn: BaseListNetworks},
	{Name: "subnets", Fn: BaseListSubnets},
	{Name: "agent_state", Labels: []string{"id", "hostname", "service", "adminState", "availability_zone"}, Fn: BaseListAgentStates},
	{Name: "network_ip_availabilities_total", Labels: []string{"network_id", "network_name", "ip_version", "cidr", "subnet_name", "project_id"}, Fn: BaseListNetworkIPAvailabilities},
	{Name: "network_ip_availabilities_used", Labels: []string{"network_id", "network_name", "ip_version", "cidr", "subnet_name", "project_id"}},
	{Name: "subnets_total", Labels: []string{"ip_version", "prefix", "prefix_length", "project_id", "subnet_pool_id", "subnet_pool_name"}, Fn: BaseListSubnetsPerPool},
	{Name: "subnets_used", Labels: []string{"ip_version", "prefix", "prefix_length", "project_id", "subnet_pool_id", "subnet_pool_name"}},
	{Name: "subnets_free", Labels: []string{"ip_version", "prefix", "prefix_length", "project_id", "subnet_pool_id", "subnet_pool_name"}},
}

// NewNeutronBaseExporter : returns a pointer to BaseNeutronExporter
func NewNeutronBaseExporter(config *ExporterConfig) (*BaseNeutronExporter, error) {
	exporter := BaseNeutronExporter{
		BaseOpenStackExporter{
			Name:           "neutron",
			ExporterConfig: *config,
		},
	}

	for _, metric := range BasedefaultNeutronMetrics {
		if exporter.isDeprecatedMetric(&metric) {
			continue
		}
		if !exporter.isSlowMetric(&metric) {
			exporter.AddMetric(metric.Name, metric.Fn, metric.Labels, metric.DeprecatedVersion, nil)
		}
	}

	return &exporter, nil
}

// BaseListFloatingIps : count total number of instantiated FloatingIPs and those that are associated to private IP but not in ACTIVE state
func BaseListFloatingIps(exporter *BaseOpenStackExporter, ch chan<- prometheus.Metric) error {
	var allFloatingIPs []floatingips.FloatingIP

	allPagesFloatingIPs, err := floatingips.List(exporter.Client, floatingips.ListOpts{}).AllPages()
	if err != nil {
		return err
	}

	allFloatingIPs, err = floatingips.ExtractFloatingIPs(allPagesFloatingIPs)
	if err != nil {
		return err
	}

	failedFIPs := 0
	for _, fip := range allFloatingIPs {
		ch <- prometheus.MustNewConstMetric(exporter.Metrics["floating_ip"].Metric,
			prometheus.GaugeValue, 1, fip.ID, fip.FloatingNetworkID, fip.RouterID, fip.Status, fip.ProjectID, fip.FloatingIP)
		if fip.FixedIP != "" {
			if fip.Status != "ACTIVE" {
				failedFIPs = failedFIPs + 1
			}
		}
	}

	ch <- prometheus.MustNewConstMetric(exporter.Metrics["floating_ips"].Metric,
		prometheus.GaugeValue, float64(len(allFloatingIPs)))
	ch <- prometheus.MustNewConstMetric(exporter.Metrics["floating_ips_associated_not_active"].Metric,
		prometheus.GaugeValue, float64(failedFIPs))

	return nil
}

// BaseListAgentStates : list agent state per node
func BaseListAgentStates(exporter *BaseOpenStackExporter, ch chan<- prometheus.Metric) error {
	var allAgents []agents.Agent

	allPagesAgents, err := agents.List(exporter.Client, agents.ListOpts{}).AllPages()
	if err != nil {
		return err
	}

	allAgents, err = agents.ExtractAgents(allPagesAgents)
	if err != nil {
		return err
	}

	for _, agent := range allAgents {
		var state int = 0
		var id string
		var zone string

		if agent.Alive {
			state = 1
		}

		adminState := "down"
		if agent.AdminStateUp {
			adminState = "up"
		}

		id = agent.ID
		if id == "" {
			if id, err = exporter.ExporterConfig.UUIDGenFunc(); err != nil {
				return err
			}
		}

		zone = agent.AvailabilityZone

		ch <- prometheus.MustNewConstMetric(exporter.Metrics["agent_state"].Metric,
			prometheus.CounterValue, float64(state), id, agent.Host, agent.Binary, adminState, zone)
	}

	return nil
}

// BaseListNetworks : Count total number of instantiated Networks
func BaseListNetworks(exporter *BaseOpenStackExporter, ch chan<- prometheus.Metric) error {
	var allNetworks []networks.Network

	allPagesNetworks, err := networks.List(exporter.Client, networks.ListOpts{}).AllPages()
	if err != nil {
		return err
	}

	allNetworks, err = networks.ExtractNetworks(allPagesNetworks)
	if err != nil {
		return err
	}
	ch <- prometheus.MustNewConstMetric(exporter.Metrics["networks"].Metric,
		prometheus.GaugeValue, float64(len(allNetworks)))

	return nil
}


// BaseListSubnets : count total number of instantiated Subnets
func BaseListSubnets(exporter *BaseOpenStackExporter, ch chan<- prometheus.Metric) error {
	var allSubnets []subnets.Subnet

	allPagesSubnets, err := subnets.List(exporter.Client, subnets.ListOpts{}).AllPages()
	if err != nil {
		return err
	}

	allSubnets, err = subnets.ExtractSubnets(allPagesSubnets)
	if err != nil {
		return err
	}
	ch <- prometheus.MustNewConstMetric(exporter.Metrics["subnets"].Metric,
		prometheus.GaugeValue, float64(len(allSubnets)))

	return nil
}

// BaseListNetworkIPAvailabilities : count total number of used IPs per Network
func BaseListNetworkIPAvailabilities(exporter *BaseOpenStackExporter, ch chan<- prometheus.Metric) error {
	var allNetworkIPAvailabilities []networkipavailabilities.NetworkIPAvailability

	allPagesNetworkIPAvailabilities, err := networkipavailabilities.List(exporter.Client, networkipavailabilities.ListOpts{}).AllPages()
	if err != nil {
		return err
	}

	allNetworkIPAvailabilities, err = networkipavailabilities.ExtractNetworkIPAvailabilities(allPagesNetworkIPAvailabilities)
	if err != nil {
		return err
	}

	for _, NetworkIPAvailabilities := range allNetworkIPAvailabilities {
		projectID := NetworkIPAvailabilities.ProjectID
		if projectID == "" && NetworkIPAvailabilities.TenantID != "" {
			projectID = NetworkIPAvailabilities.TenantID
		}

		for _, SubnetIPAvailability := range NetworkIPAvailabilities.SubnetIPAvailabilities {
			totalIPs, err := strconv.ParseFloat(SubnetIPAvailability.TotalIPs, 64)
			if err != nil {
				return err
			}
			ch <- prometheus.MustNewConstMetric(exporter.Metrics["network_ip_availabilities_total"].Metric,
				prometheus.GaugeValue, totalIPs, NetworkIPAvailabilities.NetworkID,
				NetworkIPAvailabilities.NetworkName, strconv.Itoa(SubnetIPAvailability.IPVersion), SubnetIPAvailability.CIDR,
				SubnetIPAvailability.SubnetName, projectID)

			usedIPs, err := strconv.ParseFloat(SubnetIPAvailability.UsedIPs, 64)
			if err != nil {
				return err
			}
			ch <- prometheus.MustNewConstMetric(exporter.Metrics["network_ip_availabilities_used"].Metric,
				prometheus.GaugeValue, usedIPs, NetworkIPAvailabilities.NetworkID,
				NetworkIPAvailabilities.NetworkName, strconv.Itoa(SubnetIPAvailability.IPVersion), SubnetIPAvailability.CIDR,
				SubnetIPAvailability.SubnetName, projectID)
		}
	}

	return nil
}
// BasesubnetpoolWithSubnets : subnetpools.SubnetPool augmented with its subnets
type BasesubnetpoolWithSubnets struct {
	subnetpools.SubnetPool
	subnets []netaddr.IPPrefix
}

// IPPrefixes : returns a BasesubnetpoolWithSubnets's prefixes converted to netaddr.IPPrefix structs.
func (s *BasesubnetpoolWithSubnets) IPPrefixes() ([]netaddr.IPPrefix, error) {
	result := make([]netaddr.IPPrefix, len(s.Prefixes))
	for i, prefix := range s.Prefixes {
		ipPrefix, err := netaddr.ParseIPPrefix(prefix)
		if err != nil {
			return nil, err
		}
		result[i] = ipPrefix
	}

	return result, nil
}

// BasesubnetpoolsWithSubnets : builds a slice of BasesubnetpoolWithSubnets from subnetpools.SubnetPool and subnets.Subnet structs
func BasesubnetpoolsWithSubnets(pools []subnetpools.SubnetPool, subnets []subnets.Subnet) ([]BasesubnetpoolWithSubnets, error) {
	subnetPrefixes := make(map[string][]netaddr.IPPrefix)
	for _, subnet := range subnets {
		if subnet.SubnetPoolID != "" {
			subnetPrefix, err := netaddr.ParseIPPrefix(subnet.CIDR)
			if err != nil {
				return nil, err
			}
			subnetPrefixes[subnet.SubnetPoolID] = append(subnetPrefixes[subnet.SubnetPoolID], subnetPrefix)
		}
	}

	result := make([]BasesubnetpoolWithSubnets, len(pools))
	for i, pool := range pools {
		result[i] = BasesubnetpoolWithSubnets{pool, subnetPrefixes[pool.ID]}
	}
	return result, nil
}

// BasecalculateFreeSubnets : Count how many CIDRs of length prefixLength there are in poolPrefix after removing subnetsInPool
func BasecalculateFreeSubnets(poolPrefix *netaddr.IPPrefix, subnetsInPool []netaddr.IPPrefix, prefixLength int) (float64, error) {
	builder := netaddr.IPSetBuilder{}
	builder.AddPrefix(*poolPrefix)

	for _, subnet := range subnetsInPool {
		builder.RemovePrefix(subnet)
	}

	ipset, err := builder.IPSet()
	if err != nil {
		return 0, err
	}
	count := 0.0
	for _, prefix := range ipset.Prefixes() {
		if int(prefix.Bits()) > prefixLength {
			continue
		}
		count += math.Pow(2, float64(prefixLength-int(prefix.Bits())))
	}
	return count, nil
}

// BasecalculateUsedSubnets : find all subnets that overlap with ipPrefix and count the different subnet sizes.
// Finally, return the count that matches prefixLength.
func BasecalculateUsedSubnets(subnets []netaddr.IPPrefix, ipPrefix netaddr.IPPrefix, prefixLength int) float64 {
	result := make(map[int]int)
	for _, subnet := range subnets {
		if !ipPrefix.Overlaps(subnet) {
			continue
		}

		result[int(subnet.Bits())]++
	}
	return float64(result[prefixLength])
}

// BaseListSubnetsPerPool : Count used/free/total number of subnets per subnet pool

func BaseListSubnetsPerPool(exporter *BaseOpenStackExporter, ch chan<- prometheus.Metric) error {
	allPagesSubnets, err := subnets.List(exporter.Client, subnets.ListOpts{}).AllPages()
	if err != nil {
		return err
	}

	allSubnets, err := subnets.ExtractSubnets(allPagesSubnets)
	if err != nil {
		return err
	}

	allPagesSubnetPools, err := subnetpools.List(exporter.Client, subnetpools.ListOpts{}).AllPages()
	if err != nil {
		return err
	}

	allSubnetPools, err := subnetpools.ExtractSubnetPools(allPagesSubnetPools)
	if err != nil {
		return err
	}

	subnetPools, err := BasesubnetpoolsWithSubnets(allSubnetPools, allSubnets)
	if err != nil {
		return err
	}

	for _, subnetPool := range subnetPools {
		ipPrefixes, err := subnetPool.IPPrefixes()
		if err != nil {
			return err
		}
		for _, ipPrefix := range ipPrefixes {
			for prefixLength := subnetPool.MinPrefixLen; prefixLength <= subnetPool.MaxPrefixLen; prefixLength++ {
				if prefixLength < int(ipPrefix.Bits()) {
					continue
				}

				totalSubnets := math.Pow(2, float64(prefixLength-int(ipPrefix.Bits())))
				ch <- prometheus.MustNewConstMetric(exporter.Metrics["subnets_total"].Metric,
					prometheus.GaugeValue, totalSubnets, strconv.Itoa(subnetPool.IPversion), ipPrefix.String(), strconv.Itoa(prefixLength),
					subnetPool.ProjectID, subnetPool.ID, subnetPool.Name)

				usedSubnets := BasecalculateUsedSubnets(subnetPool.subnets, ipPrefix, prefixLength)
				ch <- prometheus.MustNewConstMetric(exporter.Metrics["subnets_used"].Metric,
					prometheus.GaugeValue, usedSubnets, strconv.Itoa(subnetPool.IPversion), ipPrefix.String(), strconv.Itoa(prefixLength),
					subnetPool.ProjectID, subnetPool.ID, subnetPool.Name)

				freeSubnets, err := BasecalculateFreeSubnets(&ipPrefix, subnetPool.subnets, prefixLength)
				if err != nil {
					return err
				}
				ch <- prometheus.MustNewConstMetric(exporter.Metrics["subnets_free"].Metric,
					prometheus.GaugeValue, freeSubnets, strconv.Itoa(subnetPool.IPversion), ipPrefix.String(), strconv.Itoa(prefixLength),
					subnetPool.ProjectID, subnetPool.ID, subnetPool.Name)
			}
		}
	}

	return nil
}
