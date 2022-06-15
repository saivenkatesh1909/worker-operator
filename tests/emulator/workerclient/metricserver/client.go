package workerclient

import (
	"github.com/stretchr/testify/mock"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

type MetricSeverClientEmulator struct {
	mock.Mock
}

func NewMetricSeverClientEmulator() (*MetricSeverClientEmulator, error) {
	return new(MetricSeverClientEmulator), nil
}

func (MetricSeverClientEmulator *MetricSeverClientEmulator) GetNamespaceMetrics(namespace string) (*v1beta1.PodMetricsList, error) {
	sampleMetrics := v1beta1.PodMetrics{
		Containers: []v1beta1.ContainerMetrics{
			{
				Name: "test-container-1",
				Usage: v1.ResourceList{
					"cpu":    resource.Quantity{},
					"memory": resource.Quantity{},
				},
			},
		},
	}
	allPodMetics := v1beta1.PodMetricsList{}
	allPodMetics.Items = append(allPodMetics.Items, sampleMetrics)
	return &allPodMetics, nil
}
