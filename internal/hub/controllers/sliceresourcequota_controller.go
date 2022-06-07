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

package controllers

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	spokev1alpha1 "github.com/kubeslice/apis-ent/pkg/worker/v1alpha1"
	kubeslicev1beta1 "github.com/kubeslice/worker-operator/api/v1beta1"
	"github.com/kubeslice/worker-operator/internal/logger"
	"github.com/kubeslice/worker-operator/pkg/events"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type SliceResourceQuotaReconciler struct {
	client.Client
	Log           logr.Logger
	MeshClient    client.Client
	EventRecorder *events.EventRecorder
	ClusterName   string
}

func (r *SliceResourceQuotaReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := r.Log.WithValues("sliceresourcequota", req.NamespacedName)
	ctx = logger.WithLogger(ctx, log)
	sliceRQ := &spokev1alpha1.WorkerSliceResourceQuota{}
	err := r.Get(ctx, req.NamespacedName, sliceRQ)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Return and don't requeue
			log.Info("Slice resource quota not found in hub. Ignoring since object must be deleted")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	log.Info("got sliceResourceQuota from hub", "sliceGw", sliceRQ.Name)

	// Return if the slice gw resource does not belong to our cluster
	if sliceRQ.Spec.ClusterName != r.ClusterName {
		log.Info("sliceResourceQuota doesn't belong to this cluster", "sliceGw", sliceRQ.Name, "cluster", clusterName, "slice resourcequota cluster", sliceRQ.Spec.ClusterName)
		return reconcile.Result{}, nil
	}

	sliceName := sliceRQ.Spec.SliceName
	meshSlice := &kubeslicev1beta1.Slice{}
	sliceRef := client.ObjectKey{
		Name:      sliceName,
		Namespace: ControlPlaneNamespace,
	}

	err = r.MeshClient.Get(ctx, sliceRef, meshSlice)
	if err != nil {
		log.Error(err, "slice object not present for slice resource quota. Waiting...", "sliceResourceQuota", sliceRQ.Name)
		return reconcile.Result{
			RequeueAfter: 30 * time.Second,
		}, nil
	}
	meshSlice.Status.SliceConfig.WorkerSliceResourceQuotaSpec = &sliceRQ.Spec
	err = r.MeshClient.Status().Update(ctx, meshSlice)
	if err != nil {
		log.Error(err, "unable to update sliceGw status in spoke cluster", "slice", meshSlice.Name)
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, err
}

func (r *SliceResourceQuotaReconciler) InjectClient(c client.Client) error {
	r.Client = c
	return nil
}
