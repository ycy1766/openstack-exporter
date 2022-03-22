package exporters

import (

	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/security/groups"
	"github.com/prometheus/client_golang/prometheus"
)

// SGNeutronExporter : extends BaseOpenStackExporter
type SGNeutronExporter struct {
	BaseOpenStackExporter
}

var SGdefaultNeutronMetrics = []Metric{
	{Name: "security_groups", Fn: SGListSecGroups},
}

// NewNeutronSGExporter : returns a pointer to SGNeutronExporter
func NewNeutronSGExporter(config *ExporterConfig) (*SGNeutronExporter, error) {
	exporter := SGNeutronExporter{
		BaseOpenStackExporter{
			Name:           "neutron",
			ExporterConfig: *config,
		},
	}

	for _, metric := range SGdefaultNeutronMetrics {
		if exporter.isDeprecatedMetric(&metric) {
			continue
		}
		if !exporter.isSlowMetric(&metric) {
			exporter.AddMetric(metric.Name, metric.Fn, metric.Labels, metric.DeprecatedVersion, nil)
		}
	}

	return &exporter, nil
}

// SGListSecGroups : count total number of instantiated Security Groups
func SGListSecGroups(exporter *BaseOpenStackExporter, ch chan<- prometheus.Metric) error {
	var allSecurityGroups []groups.SecGroup

	allPagesSecurityGroups, err := groups.List(exporter.Client, groups.ListOpts{}).AllPages()
	if err != nil {
		return err
	}

	allSecurityGroups, err = groups.ExtractGroups(allPagesSecurityGroups)
	if err != nil {
		return err
	}
	ch <- prometheus.MustNewConstMetric(exporter.Metrics["security_groups"].Metric,
		prometheus.GaugeValue, float64(len(allSecurityGroups)))

	return nil
}

