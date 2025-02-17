package exporters

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/utils/openstack/clientconfig"
	"github.com/hashicorp/go-uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
)

type Metric struct {
	Name              string
	Labels            []string
	Fn                ListFunc
	Slow              bool
	DeprecatedVersion string
}

const (
	//nolint: deadcode, unused
	BYTE = 1 << (10 * iota)
	//nolint: deadcode, unused
	KILOBYTE
	MEGABYTE
	GIGABYTE
	//nolint: deadcode, unused
	TERABYTE
)

type OpenStackExporter interface {
	prometheus.Collector

	GetName() string
	AddMetric(name string, fn ListFunc, labels []string, deprecatedVersion string, constLabels prometheus.Labels)
	MetricIsDisabled(name string) bool
}

func EnableExporter(service, prefix, cloud string, disabledMetrics []string, endpointType string, collectTime bool, disableSlowMetrics bool, disableDeprecatedMetrics bool, disableCinderAgentUUID bool, uuidGenFunc func() (string, error)) (*OpenStackExporter, error) {
	exporter, err := NewExporter(service, prefix, cloud, disabledMetrics, endpointType, collectTime, disableSlowMetrics, disableDeprecatedMetrics, disableCinderAgentUUID, uuidGenFunc)
	if err != nil {
		return nil, err
	}
	return &exporter, nil
}

type PrometheusMetric struct {
	Metric *prometheus.Desc
	Fn     ListFunc
}

type ExporterConfig struct {
	Client                   *gophercloud.ServiceClient
	Prefix                   string
	DisabledMetrics          []string
	CollectTime              bool
	UUIDGenFunc              func() (string, error)
	DisableSlowMetrics       bool
	DisableDeprecatedMetrics bool
	DisableCinderAgentUUID   bool
}

type BaseOpenStackExporter struct {
	ExporterConfig
	Name    string
	Metrics map[string]*PrometheusMetric
}

type ListFunc func(exporter *BaseOpenStackExporter, ch chan<- prometheus.Metric) error

var endpointOpts map[string]gophercloud.EndpointOpts

func (exporter *BaseOpenStackExporter) GetName() string {
	return fmt.Sprintf("%s_%s", exporter.Prefix, exporter.Name)
}

func (exporter *BaseOpenStackExporter) MetricIsDisabled(name string) bool {
	for _, metric := range exporter.DisabledMetrics {
		if metric == fmt.Sprintf("%s-%s", exporter.Name, name) {
			return true
		}
	}
	return false
}

func (exporter *BaseOpenStackExporter) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range exporter.Metrics {
		ch <- metric.Metric
	}
}

func (exporter *BaseOpenStackExporter) RunCollection(metric *PrometheusMetric, metricName string, ch chan<- prometheus.Metric) error {
	log.Infof("Collecting metrics for exporter: %s, metric: %s", exporter.GetName(), metricName)
	now := time.Now()
	err := metric.Fn(exporter, ch)
	if err != nil {
		return fmt.Errorf("failed to collect metric: %s, error: %s", metricName, err)
	}

	log.Infof("Collected metrics for exporter: %s, metric: %s", exporter.GetName(), metricName)
	if exporter.CollectTime {
		ch <- prometheus.MustNewConstMetric(exporter.Metrics["openstack_metric_collect_seconds"].Metric, prometheus.GaugeValue, time.Since(now).Seconds(), metricName)
	}
	return nil
}

func (exporter *BaseOpenStackExporter) Collect(ch chan<- prometheus.Metric) {
	metricsDown := 0
	metricsCount := len(exporter.Metrics)

	for name, metric := range exporter.Metrics {
		if metric.Fn == nil {
			log.Debugf("No function handler set for metric: %s", name)
			metricsCount--
			continue
		}

		if err := exporter.RunCollection(metric, name, ch); err != nil {
			log.Errorf("Failed to collect metric for exporter: %s, error: %s", exporter.Name, err)
			metricsDown++
		}
	}

	//If all metrics collections fails for a given service, we'll flag it as down.
	if metricsDown >= metricsCount {
		ch <- prometheus.MustNewConstMetric(exporter.Metrics["up"].Metric, prometheus.GaugeValue, 0)
	} else {
		ch <- prometheus.MustNewConstMetric(exporter.Metrics["up"].Metric, prometheus.GaugeValue, 1)
	}

}

func (exporter *BaseOpenStackExporter) isSlowMetric(metric *Metric) bool {
	return exporter.ExporterConfig.DisableSlowMetrics && metric.Slow
}

func (exporter *BaseOpenStackExporter) isDeprecatedMetric(metric *Metric) bool {
	return exporter.ExporterConfig.DisableDeprecatedMetrics && len(metric.DeprecatedVersion) > 0
}

func (exporter *BaseOpenStackExporter) AddMetric(name string, fn ListFunc, labels []string, deprecatedVersion string, constLabels prometheus.Labels) {

	if exporter.MetricIsDisabled(name) {
		log.Warnf("metric: %s has been disabled on %s exporter, not collecting metrics", name, exporter.Name)
		return
	}

	if len(deprecatedVersion) > 0 {
		log.Warnf("metric: %s has been deprecated on %s exporter in version %s and it will be removed in next release", name, exporter.Name, deprecatedVersion)
	}

	if exporter.Metrics == nil {
		exporter.Metrics = make(map[string]*PrometheusMetric)
		exporter.Metrics["up"] = &PrometheusMetric{
			Metric: prometheus.NewDesc(
				prometheus.BuildFQName(exporter.GetName(), "", "up"),
				"up", nil, constLabels),
			Fn: nil,
		}
		exporter.Metrics["openstack_metric_collect_seconds"] = &PrometheusMetric{
			Metric: prometheus.NewDesc(
				"openstack_metric_collect_seconds", "Time needed to collect metric from OpenStack API", []string{"openstack_metric"}, prometheus.Labels{"openstack_service": exporter.GetName()}),
			Fn: nil,
		}
	}

	if constLabels == nil {
		constLabels = prometheus.Labels{}
	}

	// @TODO: get the region. constLabels["region"] = exporter.

	if _, ok := exporter.Metrics[name]; !ok {
		log.Infof("Adding metric: %s to exporter: %s", name, exporter.Name)
		exporter.Metrics[name] = &PrometheusMetric{
			Metric: prometheus.NewDesc(
				prometheus.BuildFQName(exporter.GetName(), "", name),
				name, labels, constLabels),
			Fn: fn,
		}
	}
}

func NewExporter(name, prefix, cloud string, disabledMetrics []string, endpointType string, collectTime bool, disableSlowMetrics bool, disableDeprecatedMetrics bool, disableCinderAgentUUID bool, uuidGenFunc func() (string, error)) (OpenStackExporter, error) {
	var exporter OpenStackExporter
	var err error
	var transport *http.Transport

	opts := clientconfig.ClientOpts{Cloud: cloud}

	config, err := clientconfig.GetCloudFromYAML(&opts)
	if err != nil {
		return nil, err
	}

	if !*config.Verify {
		log.Infoln("SSL verification disabled on transport")
		tlsConfig := &tls.Config{InsecureSkipVerify: true}
		transport = &http.Transport{TLSClientConfig: tlsConfig}
	}

	client, err := NewServiceClient(name, &opts, transport, endpointType)
	if err != nil {
		return nil, err
	}

	if uuidGenFunc == nil {
		uuidGenFunc = uuid.GenerateUUID
	}

	exporterConfig := ExporterConfig{
		Client:                   client,
		Prefix:                   prefix,
		DisabledMetrics:          disabledMetrics,
		CollectTime:              collectTime,
		UUIDGenFunc:              uuidGenFunc,
		DisableSlowMetrics:       disableSlowMetrics,
		DisableDeprecatedMetrics: disableDeprecatedMetrics,
		DisableCinderAgentUUID:   disableCinderAgentUUID,
	}

	switch name {
	case "network":
		{
			exporter, err = NewNeutronExporter(&exporterConfig)
			if err != nil {
				return nil, err
			}
		}
        case "network-base":
                {
                        exporter, err = NewNeutronBaseExporter(&exporterConfig)
                        if err != nil {
                                return nil, err
                        }
                }
        case "network-sg":
                {
                        exporter, err = NewNeutronSGExporter(&exporterConfig)
                        if err != nil {
                                return nil, err
                        }
                }
	case "network-port":
		{
			exporter, err = NewNeutronPortExporter(&exporterConfig)
			if err != nil {
				return nil, err
			}
		}
        case "network-router":
                {
                        exporter, err = NewNeutronRouterExporter(&exporterConfig)
                        if err != nil {
                                return nil, err
                        }
                }
	case "compute":
		{
			exporter, err = NewNovaExporter(&exporterConfig)
			if err != nil {
				return nil, err
			}
		}

        case "compute-base":
                {
                        exporter, err = NewNovaBaseExporter(&exporterConfig)
                        if err != nil {
                                return nil, err
                        }
                }
        case "compute-limit":
                {
                        exporter, err = NewNovaLimitExporter(&exporterConfig)
                        if err != nil {
                                return nil, err
                        }
                }
	case "compute-total-vms":
		{
			exporter, err = NewNovaTotalVmsExporter(&exporterConfig)
			if err != nil {
				return nil, err
			}
		}
	case "image":
		{
			exporter, err = NewGlanceExporter(&exporterConfig)
			if err != nil {
				return nil, err
			}
		}
	case "volume":
		{
			exporter, err = NewCinderExporter(&exporterConfig)
			if err != nil {
				return nil, err
			}
		}
	case "identity":
		{
			exporter, err = NewKeystoneExporter(&exporterConfig)
			if err != nil {
				return nil, err
			}
		}
	case "object-store":
		{
			exporter, err = NewObjectStoreExporter(&exporterConfig)
			if err != nil {
				return nil, err
			}
		}
	case "load-balancer":
		{
			exporter, err = NewLoadbalancerExporter(&exporterConfig)
			if err != nil {
				return nil, err
			}
		}
	case "container-infra":
		{
			exporter, err = NewContainerInfraExporter(&exporterConfig)
			if err != nil {
				return nil, err
			}
		}
	case "dns":
		{
			exporter, err = NewDesignateExporter(&exporterConfig)
			if err != nil {
				return nil, err
			}
		}
	case "baremetal":
		{
			exporter, err = NewIronicExporter(&exporterConfig)
			if err != nil {
				return nil, err
			}
		}
	case "gnocchi":
		{
			exporter, err = NewGnocchiExporter(&exporterConfig)
			if err != nil {
				return nil, err
			}
		}
	case "database":
		{
			exporter, err = NewTroveExporter(&exporterConfig)
			if err != nil {
				return nil, err
			}
		}
	case "orchestration":
		{
			exporter, err = NewHeatExporter(&exporterConfig)
			if err != nil {
				return nil, err
			}
		}
	case "placement":
		{
			exporter, err = NewPlacementExporter(&exporterConfig)
			if err != nil {
				return nil, err
			}
		}
	default:
		{
			return nil, fmt.Errorf("couldn't find a handler for %s exporter", name)
		}
	}

	return exporter, nil
}
