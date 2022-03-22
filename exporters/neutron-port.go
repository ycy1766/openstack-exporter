package exporters

import (
	"strconv"

	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/portsbinding"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
	"github.com/prometheus/client_golang/prometheus"
)

// NeutronPortExporter : extends BaseOpenStackExporter
type NeutronPortExporter struct {
	BaseOpenStackExporter
}

var defaultNeutronPortMetrics = []Metric{
	{Name: "port", Labels: []string{"uuid", "network_id", "mac_address", "device_owner", "status", "binding_vif_type", "admin_state_up"}, Fn: PortListPorts},
	{Name: "ports"},
	{Name: "ports_no_ips"},
	{Name: "ports_lb_not_active"},
}

// NewNeutronPortExporter : returns a pointer to NeutronPortExporter
func NewNeutronPortExporter(config *ExporterConfig) (*NeutronPortExporter, error) {
	exporter := NeutronPortExporter{
		BaseOpenStackExporter{
			Name:           "neutron",
			ExporterConfig: *config,
		},
	}

	for _, metric := range defaultNeutronPortMetrics {
		if exporter.isDeprecatedMetric(&metric) {
			continue
		}
		if !exporter.isSlowMetric(&metric) {
			exporter.AddMetric(metric.Name, metric.Fn, metric.Labels, metric.DeprecatedVersion, nil)
		}
	}

	return &exporter, nil
}
// PortPortBinding represents a port which includes port bindings
type PortPortBinding struct {
	ports.Port
	portsbinding.PortsBindingExt
}

// PortListPorts generates metrics about ports inside the OpenStack cloud
func PortListPorts(exporter *BaseOpenStackExporter, ch chan<- prometheus.Metric) error {
	var allPorts []PortPortBinding

	allPagesPorts, err := ports.List(exporter.Client, ports.ListOpts{}).AllPages()
	if err != nil {
		return err
	}

	err = ports.ExtractPortsInto(allPagesPorts, &allPorts)
	if err != nil {
		return err
	}

	portsWithNoIP := float64(0)
	lbaasPortsInactive := float64(0)

	for _, port := range allPorts {
		if port.Status == "ACTIVE" && len(port.FixedIPs) == 0 {
			portsWithNoIP++
		}

		if port.DeviceOwner == "neutron:LOADBALANCERV2" && port.Status != "ACTIVE" {
			lbaasPortsInactive++
		}

		ch <- prometheus.MustNewConstMetric(exporter.Metrics["port"].Metric,
			prometheus.GaugeValue, 1, port.ID, port.NetworkID, port.MACAddress, port.DeviceOwner, port.Status, port.VIFType, strconv.FormatBool(port.AdminStateUp))
	}

	// NOTE(mnaser): We should deprecate this and users can replace it by
	//               count(openstack_neutron_port)
	ch <- prometheus.MustNewConstMetric(exporter.Metrics["ports"].Metric,
		prometheus.GaugeValue, float64(len(allPorts)))

	// NOTE(mnaser): We should deprecate this and users can replace it by:
	//               count(openstack_neutron_port{device_owner="neutron:LOADBALANCERV2",status!="ACTIVE"})
	ch <- prometheus.MustNewConstMetric(exporter.Metrics["ports_lb_not_active"].Metric,
		prometheus.GaugeValue, lbaasPortsInactive)

	ch <- prometheus.MustNewConstMetric(exporter.Metrics["ports_no_ips"].Metric,
		prometheus.GaugeValue, portsWithNoIP)

	return nil
}

