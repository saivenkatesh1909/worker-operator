package workerclient

import (
	"github.com/stretchr/testify/mock"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

type MetricSeverClientEmulator struct {
	mock.Mock
}

func NewMetricSeverClientEmulator() (*MetricSeverClientEmulator, error) {
	return new(MetricSeverClientEmulator), nil
}

func (MetricSeverClientEmulator *MetricSeverClientEmulator) GetNamespaceMetrics(namespace string) (*v1beta1.PodMetricsList, error) {
	return nil, nil
}
