package exporters

import (
	"strconv"

	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/agents"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/routers"
	"github.com/prometheus/client_golang/prometheus"
)

// RouterNeutronExporter : extends BaseOpenStackExporter
type RouterNeutronExporter struct {
	BaseOpenStackExporter
}

var RouterdefaultNeutronMetrics = []Metric{
	{Name: "router", Labels: []string{"id", "name", "project_id", "admin_state_up", "status", "external_network_id"}},
	{Name: "routers", Fn: RouterListRouters},
	{Name: "routers_not_active"},
	{Name: "l3_agent_of_router", Labels: []string{"router_id", "l3_agent_id", "ha_state", "agent_alive", "agent_admin_up", "agent_host"}},
}

// NewNeutronRouterExporter : returns a pointer to RouterNeutronExporter
func NewNeutronRouterExporter(config *ExporterConfig) (*RouterNeutronExporter, error) {
	exporter := RouterNeutronExporter{
		BaseOpenStackExporter{
			Name:           "neutron",
			ExporterConfig: *config,
		},
	}

	for _, metric := range RouterdefaultNeutronMetrics {
		if exporter.isDeprecatedMetric(&metric) {
			continue
		}
		if !exporter.isSlowMetric(&metric) {
			exporter.AddMetric(metric.Name, metric.Fn, metric.Labels, metric.DeprecatedVersion, nil)
		}
	}

	return &exporter, nil
}

// RouterListAgentStates : list agent state per node
func RouterListAgentStates(exporter *BaseOpenStackExporter, ch chan<- prometheus.Metric) error {
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

// RouterListRouters : count total number of instantiated Routers and those that are not in ACTIVE state
func RouterListRouters(exporter *BaseOpenStackExporter, ch chan<- prometheus.Metric) error {
	var allRouters []routers.Router

	allPagesRouters, err := routers.List(exporter.Client, routers.ListOpts{}).AllPages()
	if err != nil {
		return err
	}

	allRouters, err = routers.ExtractRouters(allPagesRouters)
	if err != nil {
		return err
	}

	failedRouters := 0
	for _, router := range allRouters {
		if router.Status != "ACTIVE" {
			failedRouters = failedRouters + 1
		}
		allPagesL3Agents, err := routers.ListL3Agents(exporter.Client, router.ID).AllPages()
		if err != nil {
			return err
		}
		l3Agents, err := routers.ExtractL3Agents(allPagesL3Agents)
		if err != nil {
			return err
		}
		for _, agent := range l3Agents {
			var state int

			if agent.Alive {
				state = 1
			}

			ch <- prometheus.MustNewConstMetric(exporter.Metrics["l3_agent_of_router"].Metric,
				prometheus.GaugeValue, float64(state), router.ID, agent.ID,
				agent.HAState, strconv.FormatBool(agent.Alive), strconv.FormatBool(agent.AdminStateUp), agent.Host)
		}
		ch <- prometheus.MustNewConstMetric(exporter.Metrics["router"].Metric,
			prometheus.GaugeValue, 1, router.ID, router.Name, router.ProjectID,
			strconv.FormatBool(router.AdminStateUp), router.Status, router.GatewayInfo.NetworkID)
	}

	ch <- prometheus.MustNewConstMetric(exporter.Metrics["routers"].Metric,
		prometheus.GaugeValue, float64(len(allRouters)))
	ch <- prometheus.MustNewConstMetric(exporter.Metrics["routers_not_active"].Metric,
		prometheus.GaugeValue, float64(failedRouters))

	return nil
}

