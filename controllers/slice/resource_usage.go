/*
 *  Copyright (c) 2022 Avesha, Inc. All rights reserved.
 *
 *  SPDX-License-Identifier: Apache-2.0
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package slice

import (
	"context"
	"fmt"
	"strconv"

	spokev1alpha1 "github.com/kubeslice/apis-ent/pkg/worker/v1alpha1"
	kubeslicev1beta1 "github.com/kubeslice/worker-operator/api/v1beta1"
	"github.com/kubeslice/worker-operator/internal/logger"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type metricServer struct {
}

func NewMetricServerClientProvider() (*metricServer, error) {
	return &metricServer{}, nil
}
func (m *metricServer) GetNamespaceMetrics(namespace string) (*v1beta1.PodMetricsList, error) {
	clientset, err := metricsv.NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		return nil, err
	}
	return clientset.MetricsV1beta1().PodMetricses(namespace).List(context.TODO(), metav1.ListOptions{})
}

func (r *SliceReconciler) reconcileNamespaceResourceUsage(ctx context.Context, slice *kubeslicev1beta1.Slice, currentTime, configUpdatedOn int64) (ctrl.Result, error) {
	log := logger.FromContext(ctx).WithValues("type", "resource_usage")
	// Get the list of existing namespaces that are part of slice
	namespacesInSlice := &corev1.NamespaceList{}
	listOpts := []client.ListOption{
		client.MatchingLabels(map[string]string{
			ApplicationNamespaceSelectorLabelKey: slice.Name,
		}),
	}
	err := r.List(ctx, namespacesInSlice, listOpts...)
	if err != nil {
		log.Error(err, "Failed to list namespaces")
		return ctrl.Result{}, err
	}
	log.Info("reconciling", "namespacesInSlice", namespacesInSlice)

	clientset, err := metricsv.NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		log.Error(err, "error creating client set")
	}
	currentAllNsCPU := resource.Quantity{}
	currentAllNsMem := resource.Quantity{}
	for _, namespace := range namespacesInSlice.Items {
		// metrics of all the pods of a namespace
		podMetricsList, err := r.MetricServerClient.GetNamespaceMetrics(namespace.Name)
		if err != nil {
			return ctrl.Result{}, err
		}
		cpu, mem := getCPUandMemoryMetricsResource(podMetricsList.Items)
		currentAllNsCPU.Add(cpu)
		currentAllNsMem.Add(mem)
	}

	if currentAllNsCPU.Equal(resource.Quantity{}) && currentAllNsMem.Equal(resource.Quantity{}) { // no current usage
		return ctrl.Result{}, nil
	}
	updateResourceUsage := false
	if slice.Status.SliceConfig.WorkerSliceResourceQuotaStatus == nil {
		slice.Status.SliceConfig.WorkerSliceResourceQuotaStatus = &spokev1alpha1.WorkerSliceResourceQuotaStatus{}
		updateResourceUsage = true
	} else if checkToUpdateControllerSliceResourceQuota(slice.Status.SliceConfig.WorkerSliceResourceQuotaStatus.
		ClusterResourceQuotaStatus.ResourcesUsage, currentAllNsCPU, currentAllNsMem) {
		updateResourceUsage = true
	}
	log.Info("CPU usage of all namespaces", "cpu", currentAllNsCPU)
	log.Info("Memory usage of all namespaces", "mem", currentAllNsMem)
	if updateResourceUsage {
		allNsResourceUsage := []spokev1alpha1.NamespaceResourceQuotaStatus{}
		for _, namespace := range namespacesInSlice.Items {
			// metrics of all the pods of a namespace
			podMetricsList, _ := clientset.MetricsV1beta1().PodMetricses(namespace.Name).List(context.TODO(), metav1.ListOptions{})
			cpuAsQuantity, memAsQuantity := getCPUandMemoryMetricsResource(podMetricsList.Items)
			cpuInMilliCores := resource.NewMilliQuantity(cpuAsQuantity.MilliValue(), resource.DecimalSI)
			memAsMI := strconv.Itoa(int(memAsQuantity.ScaledValue(resource.Mega)))
			allNsResourceUsage = append(allNsResourceUsage, spokev1alpha1.NamespaceResourceQuotaStatus{
				ResourceUsage: spokev1alpha1.Resource{
					Cpu:    *cpuInMilliCores,
					Memory: resource.MustParse(memAsMI + "Mi"),
				},
				Namespace: namespace.Name,
			})
		}

		cpuInMilliCoresAllNs := resource.NewMilliQuantity(currentAllNsCPU.MilliValue(), resource.DecimalSI)
		memAsMIAllNs := strconv.Itoa(int(currentAllNsMem.ScaledValue(resource.Mega)))
		slice.Status.SliceConfig.WorkerSliceResourceQuotaStatus.ClusterResourceQuotaStatus =
			spokev1alpha1.ClusterResourceQuotaStatus{
				NamespaceResourceQuotaStatus: allNsResourceUsage,
				ResourcesUsage: spokev1alpha1.Resource{ // all namespace collectively
					Cpu:    *cpuInMilliCoresAllNs,
					Memory: resource.MustParse(memAsMIAllNs + "Mi"),
				},
			}
		err := r.HubClient.UpdateResourceUsage(ctx, slice.Name, *slice.Status.SliceConfig.WorkerSliceResourceQuotaStatus)
		if err != nil {
			log.Error(err, "error updating hub worker slice resource quota")
			return ctrl.Result{}, err
		}
	}
	log.Info("updating resource usage time to slice status config")
	slice.Status.ConfigUpdatedOn = currentTime
	r.Status().Update(ctx, slice)
	return ctrl.Result{}, nil
}

func checkToUpdateControllerSliceResourceQuota(sliceUsage spokev1alpha1.Resource, currentcpu, currentmem resource.Quantity) bool {
	memUsage := sliceUsage.Memory.ScaledValue(resource.Kilo)
	fmt.Println("memUsage", memUsage)
	curremtMemUsage := currentmem.ScaledValue(resource.Kilo)
	fmt.Println("curremtMemUsage", curremtMemUsage)

	cpuUsage := sliceUsage.Cpu.ScaledValue(resource.Nano)
	fmt.Println("cpuUsage", cpuUsage)
	currentCPUUsage := resource.NewMilliQuantity(currentcpu.MilliValue(), resource.DecimalSI)
	fmt.Println("currentCPUUsage", currentCPUUsage)
	if calculatePercentageDiff(cpuUsage, currentCPUUsage.ScaledValue(resource.Nano)) > 5 || calculatePercentageDiff(memUsage, curremtMemUsage) > 5 {
		fmt.Println("updating resource usage")
		return true
	}
	return false
}

func getCPUandMemoryMetricsResource(podMetricsList []v1beta1.PodMetrics) (resource.Quantity, resource.Quantity) {
	nsTotalCPU := resource.Quantity{}
	nsTotalMem := resource.Quantity{}
	for _, podMetrics := range podMetricsList {
		for _, container := range podMetrics.Containers {
			usage := container.Usage
			nowCpu := usage.Cpu()
			nowMem := usage.Memory()
			nsTotalCPU.Add(*nowCpu)
			nsTotalMem.Add(*nowMem)
		}
	}
	return nsTotalCPU, nsTotalMem
}
func calculatePercentageDiff(a, b int64) int64 {
	return ((b - a) * 100) / a
}
