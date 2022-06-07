/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// WorkerSliceResourceQuotaSpec defines the desired state of WorkerSliceResourceQuota
type WorkerSliceResourceQuotaSpec struct {
	//SliceName defines the name of the slice for the WorkerResourceQuota
	SliceName string `json:"sliceName,omitempty"`
	//ClusterName defines the name of the cluster for the WorkerResourceQuota
	ClusterName string `json:"clusterName,omitempty"`
	//ResourceQuotaProfile defines the resource quota profile for the slice
	ResourceQuotaProfile ResourceQuotaProfile `json:"resourceQuotaProfile,omitempty"`
}

//ResourceQuotaProfile is the configuration for resource quota of a slice
type ResourceQuotaProfile struct {
	//SliceQuota defines the configuration for slice quota of a resource quota
	SliceQuota SliceQuota `json:"sliceQuota,omitempty"`
	//ClusterQuota defines the configuration for cluster quota of a resource quota
	ClusterQuota ClusterQuota `json:"clusterQuota,omitempty"`
}

//SliceQuota defines the configuration for slice quota of a resource quota
type SliceQuota struct {
	//Resources defines the configuration for resources for SliceQuota
	Resources Resource `json:"resources,omitempty"`
}

//ClusterQuota defines the configuration for cluster quota of a resource quota
type ClusterQuota struct {
	//Resources defines the configuration for resources for ClusterQuota
	Resources Resource `json:"resources,omitempty"`
	//NamespaceQuota defines the configuration for namespace quota of a ClusterQuota
	NamespaceQuota []NamespaceQuota `json:"namespaceQuota,omitempty"`
}

//NamespaceQuota defines the configuration for namespace quota of a ClusterQuota
type NamespaceQuota struct {
	//Namespace defines the namespace of the NamespaceQuota
	Namespace string `json:"namespace,omitempty"`
	//Resources defines the configuration for resources for NamespaceQuota
	Resources Resource `json:"resources,omitempty"`
	//EnforceQuota defines the enforceQuota status flag for NamespaceQuota
	//+kubebuilder:default:=false
	//+kubebuilder:validation:Optional
	EnforceQuota bool `json:"enforceQuota"`
}

//Resource defines the configuration for resources for NamespaceQuota
type Resource struct {
	//Memory defines utilization of Memory resource
	Memory resource.Quantity `json:"memory,omitempty"`
	//Cpu defines utilization of Cpu resource
	Cpu resource.Quantity `json:"cpu,omitempty"`
}

// WorkerSliceResourceQuotaStatus defines the observed state of WorkerSliceResourceQuota
type WorkerSliceResourceQuotaStatus struct {
	ClusterResourceQuotaStatus ClusterResourceQuotaStatus `json:"clusterResourceQuotaStatus,omitempty"`
}

type ClusterResourceQuotaStatus struct {
	ResourcesUsage               Resource                       `json:"resourceUsage,omitempty"`
	NamespaceResourceQuotaStatus []NamespaceResourceQuotaStatus `json:"namespaceResourceQuotaStatus,omitempty"`
}

type NamespaceResourceQuotaStatus struct {
	Namespace     string   `json:"namespace,omitempty"`
	ResourceUsage Resource `json:"resourceUsage,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// WorkerSliceResourceQuota is the Schema for the workersliceresourcequota API
type WorkerSliceResourceQuota struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkerSliceResourceQuotaSpec   `json:"spec,omitempty"`
	Status WorkerSliceResourceQuotaStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// WorkerSliceResourceQuotaList contains a list of WorkerSliceResourceQuota
type WorkerSliceResourceQuotaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkerSliceResourceQuota `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WorkerSliceResourceQuota{}, &WorkerSliceResourceQuotaList{})
}
