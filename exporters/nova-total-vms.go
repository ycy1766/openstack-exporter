package exporters

import (

        "fmt"
        "os"

        "github.com/gophercloud/gophercloud/openstack/compute/apiversions"
        "github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/availabilityzones"
        "github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/extendedserverattributes"
        "github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
        "github.com/prometheus/client_golang/prometheus"
)

var TotalVmsserver_status = []string{
	"ACTIVE",
	"BUILD",             // The server has not finished the original build process.
	"BUILD(spawning)",   // The server has not finished the original build process but networking works (HP Cloud specific)
	"DELETED",           // The server is deleted.
	"ERROR",             // The server is in error.
	"HARD_REBOOT",       // The server is hard rebooting.
	"PASSWORD",          // The password is being reset on the server.
	"REBOOT",            // The server is in a soft reboot state.
	"REBUILD",           // The server is currently being rebuilt from an image.
	"RESCUE",            // The server is in rescue mode.
	"RESIZE",            // Server is performing the differential copy of data that changed during its initial copy.
	"SHUTOFF",           // The virtual machine (VM) was powered down by the user, but not through the OpenStack Compute API.
	"SUSPENDED",         // The server is suspended, either by request or necessity.
	"UNKNOWN",           // The state of the server is unknown. Contact your cloud provider.
	"VERIFY_RESIZE",     // System is awaiting confirmation that the server is operational after a move or resize.
	"MIGRATING",         // The server is migrating. This is caused by a live migration (moving a server that is active) action.
	"PAUSED",            // The server is paused.
	"REVERT_RESIZE",     // The resize or migration of a server failed for some reason. The destination server is being cleaned up and the original source server is restarting.
	"SHELVED",           // The server is in shelved state. Depends on the shelve offload time, the server will be automatically shelved off loaded.
	"SHELVED_OFFLOADED", // The shelved server is offloaded (removed from the compute host) and it needs unshelved action to be used again.
	"SOFT_DELETED",      // The server is marked as deleted but will remain in the cloud for some configurable amount of time.
}

func TotalVmsmapServerStatus(current string) int {
	for idx, status := range TotalVmsserver_status {
		if current == status {
			return idx
		}
	}
	return -1
}

type TotalVmsNovaExporter struct {
	BaseOpenStackExporter
}

var TotalVmsdefaultNovaMetrics = []Metric{
	{Name: "total_vms", Fn: TotalVmsListAllServers},
        {Name: "TotalVmsserver_status", Labels: []string{"id", "status", "name", "tenant_id", "user_id", "address_ipv4",
                "address_ipv6", "host_id", "hypervisor_hostname", "uuid", "availability_zone", "flavor_id"}},
}

func NewNovaTotalVmsExporter(config *ExporterConfig) (*TotalVmsNovaExporter, error) {
	exporter := TotalVmsNovaExporter{
		BaseOpenStackExporter{
			Name:           "nova",
			ExporterConfig: *config,
		},
	}
	for _, metric := range TotalVmsdefaultNovaMetrics {
		if exporter.isDeprecatedMetric(&metric) {
			continue
		}
		if !exporter.isSlowMetric(&metric) {
			exporter.AddMetric(metric.Name, metric.Fn, metric.Labels, metric.DeprecatedVersion, nil)
		}
	}

	envMicroversion, present := os.LookupEnv("OS_COMPUTE_API_VERSION")
	if present {
		exporter.Client.Microversion = envMicroversion
	} else {

		microversion, err := apiversions.Get(config.Client, "v2.1").Extract()
		if err == nil {
			exporter.Client.Microversion = microversion.Version
		}
	}

	return &exporter, nil
}
func TotalVmsListAllServers(exporter *BaseOpenStackExporter, ch chan<- prometheus.Metric) error {
	type ServerWithExt struct {
		servers.Server
		availabilityzones.ServerAvailabilityZoneExt
		extendedserverattributes.ServerAttributesExt
	}

	var allServers []ServerWithExt

	allPagesServers, err := servers.List(exporter.Client, servers.ListOpts{AllTenants: true}).AllPages()
	if err != nil {
		return err
	}

	err = servers.ExtractServersInto(allPagesServers, &allServers)
	if err != nil {
		return err
	}

	ch <- prometheus.MustNewConstMetric(exporter.Metrics["total_vms"].Metric,
		prometheus.GaugeValue, float64(len(allServers)))

	// Server status metrics
	for _, server := range allServers {
		ch <- prometheus.MustNewConstMetric(exporter.Metrics["TotalVmsserver_status"].Metric,
			prometheus.GaugeValue, float64(TotalVmsmapServerStatus(server.Status)), server.ID, server.Status, server.Name, server.TenantID,
			server.UserID, server.AccessIPv4, server.AccessIPv6, server.HostID, server.HypervisorHostname, server.ID, server.AvailabilityZone, fmt.Sprintf("%v", server.Flavor["id"]))
	}

	return nil
}

