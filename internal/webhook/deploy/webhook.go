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

package deploy

import (
	"context"
	"encoding/json"
	"net/http"

	//	"github.com/kubeslice/worker-operator/controllers"
	"github.com/kubeslice/worker-operator/internal/logger"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	admissionWebhookAnnotationInjectKey       = "avesha.io/slice"
	admissionWebhookAnnotationStatusKey       = "avesha.io/status"
	podinjectkey                              = "avesha.io/pod-type"
	admissionWebhookSliceNamespaceSelectorKey = "kubeslice.io/slice"
	controlPlaneNamespace                     = "kubeslice-system"
	nsmkey                                    = "ns.networkservicemesh.io"
)

var (
	log = logger.NewLogger().WithName("Webhook").V(1)
)

type WebhookInterface interface {
	GetSliceNamespaceIsolationPolicy(ctx context.Context, slice string) (bool, error)
	SliceAppNamespaceConfigured(ctx context.Context, slice string, namespace string) (bool, error)
	GetNamespaceLabels(ctx context.Context, client client.Client, namespace string) (map[string]string, error)
}

type WebhookServer struct {
	Client   client.Client
	decoder  *admission.Decoder
	WhClient WebhookInterface
}

func (wh *WebhookServer) Handle(ctx context.Context, req admission.Request) admission.Response {
	deploy := &appsv1.Deployment{}
	err := wh.decoder.Decode(req, deploy)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	log := logger.FromContext(ctx)

	if mutate, sliceName := wh.MutationRequired(deploy.ObjectMeta); !mutate {
		log.Info("mutation not required", "pod metadata", deploy.Spec.Template.ObjectMeta)
	} else {
		log.Info("mutating deploy", "pod metadata", deploy.Spec.Template.ObjectMeta)
		deploy = Mutate(deploy, sliceName)
		log.Info("mutated deploy", "pod metadata", deploy.Spec.Template.ObjectMeta)
	}

	marshaled, err := json.Marshal(deploy)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaled)
}

func (wh *WebhookServer) InjectDecoder(d *admission.Decoder) error {
	wh.decoder = d
	return nil
}

func Mutate(deploy *appsv1.Deployment, sliceName string) *appsv1.Deployment {
	// Add injection status to deployment annotations
	deploy.Annotations[admissionWebhookAnnotationStatusKey] = "injected"

	if deploy.Spec.Template.ObjectMeta.Annotations == nil {
		deploy.Spec.Template.ObjectMeta.Annotations = map[string]string{}
	}

	// Add vl3 annotation to pod template
	annotations := deploy.Spec.Template.ObjectMeta.Annotations
	annotations[nsmkey] = "vl3-service-" + sliceName

	// Add slice identifier labels to pod template
	labels := deploy.Spec.Template.ObjectMeta.Labels
	labels[podinjectkey] = "app"
	labels[admissionWebhookAnnotationInjectKey] = sliceName

	return deploy
}

func (wh *WebhookServer) MutationRequired(metadata metav1.ObjectMeta) (bool, string) {
	annotations := metadata.GetAnnotations()
	//early exit if metadata in nil
	//we allow empty annotation, but namespace should not be empty
	if len(annotations) == 0 && metadata.GetNamespace() == "" {
		return false, ""
	}
	if annotations[admissionWebhookAnnotationStatusKey] == "injected" {
		log.Info("Deployment is already injected")
		return false, ""
	}

	if metadata.GetLabels()[podinjectkey] == "app" {
		log.Info("Pod is already injected")
		return false, ""
	}
	//if slice annotation is present,and the namespaceIsolation is not enabled, we inject
	sliceNameInAnnotation, sliceAnnotationPresent := metadata.GetAnnotations()[admissionWebhookAnnotationInjectKey]
	if sliceAnnotationPresent {
		// Ignore namespace isolation related checks if the object is part of the kubeslice control plane namespace.
		if metadata.Namespace == controlPlaneNamespace {
			return true, sliceNameInAnnotation
		}
		// Check if namespace isolation is enabled for this slice
		nsIsolationEnabled, err := wh.WhClient.GetSliceNamespaceIsolationPolicy(context.Background(), sliceNameInAnnotation)
		if err != nil {
			log.Error(err, "Could not get namespace isolation policy", "slice", sliceNameInAnnotation)
			return false, ""
		}
		//if nsIsolationEnabled is not enabled at this point in time,we inject
		if !nsIsolationEnabled {
			return true, sliceNameInAnnotation
		}
	}
	// slice annotation is not present, we check if this namespace is part of slice appNS
	// Do not auto onboard control plane namespace. Ideally, we should not have any deployment/pod in the control plane ns
	// connect to a slice. But for exceptional cases, return from here before updating the app ns list in the slice config.
	if metadata.Namespace == controlPlaneNamespace {
		return false, ""
	}

	nsLabels, err := wh.WhClient.GetNamespaceLabels(context.Background(), wh.Client, metadata.Namespace)
	if err != nil {
		log.Error(err, "Error getting namespace labels")
		return false, ""
	}
	if nsLabels == nil {
		log.Info("Namespace has no labels")
		return false, ""
	}

	sliceNameInNs, sliceLabelPresent := nsLabels[admissionWebhookSliceNamespaceSelectorKey]
	if !sliceLabelPresent {
		log.Info("Namespace has no slice labels")
		return false, ""
	}

	if sliceNameInAnnotation != "" && sliceNameInAnnotation != sliceNameInNs {
		return false, ""
	}

	nsConfigured, err := wh.WhClient.SliceAppNamespaceConfigured(context.Background(), sliceNameInNs, metadata.Namespace)
	if err != nil {
		log.Error(err, "Failed to get app namespace info for slice",
			"slice", sliceNameInNs, "namespace", metadata.Namespace)
		return false, ""
	}
	if !nsConfigured {
		log.Info("Namespace not part of slice", "namespace", metadata.Namespace, "slice", sliceNameInNs)
		return false, ""
	}
	// The annotation avesha.io/slice:SLICENAME is present, enable mutation
	return true, sliceNameInNs
}
