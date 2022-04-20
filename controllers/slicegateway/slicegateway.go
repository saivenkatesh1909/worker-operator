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

package slicegateway

import (
	"context"
	"os"
	"strconv"
	"time"

	"bitbucket.org/realtimeai/kubeslice-operator/controllers"
	"bitbucket.org/realtimeai/kubeslice-operator/internal/netop"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	meshv1beta1 "bitbucket.org/realtimeai/kubeslice-operator/api/v1beta1"
	"bitbucket.org/realtimeai/kubeslice-operator/internal/gwsidecar"
	"bitbucket.org/realtimeai/kubeslice-operator/internal/logger"
	"bitbucket.org/realtimeai/kubeslice-operator/internal/router"
)

var (
	gwSidecarImage           = os.Getenv("AVESHA_GW_SIDECAR_IMAGE")
	gwSidecarImagePullPolicy = os.Getenv("AVESHA_GW_SIDECAR_IMAGE_PULLPOLICY")

	openVpnServerImage      = os.Getenv("AVESHA_OPENVPN_SERVER_IMAGE")
	openVpnClientImage      = os.Getenv("AVESHA_OPENVPN_CLIENT_IMAGE")
	openVpnServerPullPolicy = os.Getenv("AVESHA_OPENVPN_SERVER_PULLPOLICY")
	openVpnClientPullPolicy = os.Getenv("AVESHA_OPENVPN_CLIENT_PULLPOLICY")
)

// labelsForSliceGwDeployment returns the labels for creating slice gw deployment
func labelsForSliceGwDeployment(name string, slice string) map[string]string {
	return map[string]string{
		"networkservicemesh.io/app": name,
		"kubeslice.io/pod-type":     "slicegateway",
		"kubeslice.io/slice":        slice}
}

// deploymentForGateway returns a gateway Deployment object
func (r *SliceGwReconciler) deploymentForGateway(g *meshv1beta1.SliceGateway) *appsv1.Deployment {
	if g.Status.Config.SliceGatewayHostType == "Server" {
		return r.deploymentForGatewayServer(g)
	} else {
		return r.deploymentForGatewayClient(g)
	}
}

func (r *SliceGwReconciler) deploymentForGatewayServer(g *meshv1beta1.SliceGateway) *appsv1.Deployment {
	ls := labelsForSliceGwDeployment(g.Name, g.Spec.SliceName)

	var replicas int32 = 1

	var vpnSecretDefaultMode int32 = 420
	var vpnFilesRestrictedMode int32 = 0644

	var privileged = true

	sidecarImg := "nexus.dev.aveshalabs.io/kubeslice/gw-sidecar:1.0.0"
	sidecarPullPolicy := corev1.PullAlways
	vpnImg := "nexus.dev.aveshalabs.io/avesha/openvpn-server.ubuntu.18.04:1.0.0"
	vpnPullPolicy := corev1.PullAlways
	baseFileName := os.Getenv("CLUSTER_NAME") + "-" + g.Spec.SliceName + "-" + g.Status.Config.SliceGatewayName + ".vpn.aveshasystems.com"

	if len(gwSidecarImage) != 0 {
		sidecarImg = gwSidecarImage
	}

	if len(gwSidecarImagePullPolicy) != 0 {
		sidecarPullPolicy = corev1.PullPolicy(gwSidecarImagePullPolicy)
	}

	if len(openVpnServerImage) != 0 {
		vpnImg = openVpnServerImage
	}

	if len(openVpnServerPullPolicy) != 0 {
		vpnPullPolicy = corev1.PullPolicy(openVpnServerPullPolicy)
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      g.Name,
			Namespace: g.Namespace,
			Annotations: map[string]string{
				"ns.networkservicemesh.io": "vl3-service-" + g.Spec.SliceName,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
					Annotations: map[string]string{
						"prometheus.io/port":   "18080",
						"prometheus.io/scrape": "true",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "nsmgr-acc",
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{{
									MatchExpressions: []corev1.NodeSelectorRequirement{{
										Key:      "avesha/node-type",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"gateway"},
									}},
								}},
							},
						},
					},
					Containers: []corev1.Container{{
						Name:            "avesha-sidecar",
						Image:           sidecarImg,
						ImagePullPolicy: sidecarPullPolicy,
						Env: []corev1.EnvVar{
							{
								Name:  "SLICE_NAME",
								Value: g.Spec.SliceName,
							},
							{
								Name:  "CLUSTER_ID",
								Value: controllers.ClusterName,
							},
							{
								Name:  "REMOTE_CLUSTER_ID",
								Value: g.Status.Config.SliceGatewayRemoteClusterID,
							},
							{
								Name:  "GATEWAY_ID",
								Value: g.Status.Config.SliceGatewayID,
							},
							{
								Name:  "REMOTE_GATEWAY_ID",
								Value: g.Status.Config.SliceGatewayRemoteGatewayID,
							},
							{
								Name:  "POD_TYPE",
								Value: "GATEWAY_POD",
							},
							{
								Name:  "NODE_IP",
								Value: controllers.NodeIP,
							},
							{
								Name:  "OPEN_VPN_MODE",
								Value: "SERVER",
							},
							{
								Name:  "MOUNT_PATH",
								Value: "config",
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Privileged:               &privileged,
							AllowPrivilegeEscalation: &privileged,
							Capabilities: &corev1.Capabilities{
								Add: []corev1.Capability{
									"NET_ADMIN",
								},
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "shared-volume",
								MountPath: "/config",
							},
							{
								Name:      "vpn-certs",
								MountPath: "/var/run/vpn",
							},
						},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								"memory": resource.MustParse("200Mi"),
								"cpu":    resource.MustParse("500m"),
							},
							Requests: corev1.ResourceList{
								"memory": resource.MustParse("50Mi"),
								"cpu":    resource.MustParse("50m"),
							},
						},
					}, {
						Name:            "avesha-openvpn-server",
						Image:           vpnImg,
						ImagePullPolicy: vpnPullPolicy,
						Command: []string{
							"/usr/local/bin/waitForConfigToRunCmd.sh",
						},
						Args: []string{
							"/etc/openvpn/openvpn.conf",
							"90",
							"ovpn_run",
						},
						SecurityContext: &corev1.SecurityContext{
							Privileged:               &privileged,
							AllowPrivilegeEscalation: &privileged,
							Capabilities: &corev1.Capabilities{
								Add: []corev1.Capability{
									"NET_ADMIN",
								},
							},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "shared-volume",
							MountPath: "/etc/openvpn",
						}},
					}},
					Volumes: []corev1.Volume{
						{
							Name: "shared-volume",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "vpn-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  g.Name,
									DefaultMode: &vpnSecretDefaultMode,
									Items: []corev1.KeyToPath{
										{
											Key:  "ovpnConfigFile",
											Path: "openvpn.conf",
										}, {
											Key:  "pkiCACertFile",
											Path: "pki/" + "ca.crt",
										}, {
											Key:  "pkiDhPemFile",
											Path: "pki/" + "dh.pem",
										}, {
											Key:  "pkiTAKeyFile",
											Path: "pki/" + baseFileName + "-" + "ta.key",
										}, {
											Key:  "pkiIssuedCertFile",
											Path: "pki/issued/" + baseFileName + ".crt",
											Mode: &vpnFilesRestrictedMode,
										}, {
											Key:  "pkiPrivateKeyFile",
											Path: "pki/private/" + baseFileName + ".key",
											Mode: &vpnFilesRestrictedMode,
										}, {
											Key:  "ccdFile",
											Path: "ccd/" + g.Status.Config.SliceGatewayRemoteGatewayID,
										},
									},
								},
							},
						},
					},
					Tolerations: []corev1.Toleration{{
						Key:      "avesha/node-type",
						Operator: "Equal",
						Effect:   "NoSchedule",
						Value:    "gateway",
					}, {
						Key:      "avesha/node-type",
						Operator: "Equal",
						Effect:   "NoExecute",
						Value:    "gateway",
					}},
				},
			},
		},
	}

	if len(controllers.ImagePullSecretName) != 0 {
		dep.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{
			Name: controllers.ImagePullSecretName,
		}}
	}

	// Set SliceGateway instance as the owner and controller
	ctrl.SetControllerReference(g, dep, r.Scheme)
	return dep
}

func (r *SliceGwReconciler) serviceForGateway(g *meshv1beta1.SliceGateway) *corev1.Service {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc-" + g.Name,
			Namespace: g.Namespace,
			Labels: map[string]string{
				"avesha.io/slice": g.Spec.SliceName,
			},
		},
		Spec: corev1.ServiceSpec{
			Type:     "NodePort",
			Selector: labelsForSliceGwDeployment(g.Name, g.Spec.SliceName),
			Ports: []corev1.ServicePort{{
				Port:       11194,
				Protocol:   corev1.ProtocolUDP,
				TargetPort: intstr.FromInt(11194),
			}},
		},
	}
	ctrl.SetControllerReference(g, svc, r.Scheme)
	return svc
}

func (r *SliceGwReconciler) deploymentForGatewayClient(g *meshv1beta1.SliceGateway) *appsv1.Deployment {
	var replicas int32 = 1
	var privileged = true

	var vpnSecretDefaultMode int32 = 0644

	certFileName := "openvpn_client.ovpn"
	sidecarImg := "nexus.dev.aveshalabs.io/kubeslice/gw-sidecar:1.0.0"
	sidecarPullPolicy := corev1.PullAlways

	vpnImg := "nexus.dev.aveshalabs.io/avesha/openvpn-client.alpine.amd64:1.0.0"
	vpnPullPolicy := corev1.PullAlways

	ls := labelsForSliceGwDeployment(g.Name, g.Spec.SliceName)

	if len(gwSidecarImage) != 0 {
		sidecarImg = gwSidecarImage
	}

	if len(gwSidecarImagePullPolicy) != 0 {
		sidecarPullPolicy = corev1.PullPolicy(gwSidecarImagePullPolicy)
	}

	if len(openVpnClientImage) != 0 {
		vpnImg = openVpnClientImage
	}

	if len(openVpnClientPullPolicy) != 0 {
		vpnPullPolicy = corev1.PullPolicy(openVpnClientPullPolicy)
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      g.Name,
			Namespace: g.Namespace,
			Annotations: map[string]string{
				"ns.networkservicemesh.io": "vl3-service-" + g.Spec.SliceName,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
					Annotations: map[string]string{
						"prometheus.io/port":   "18080",
						"prometheus.io/scrape": "true",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "nsmgr-acc",
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{{
									MatchExpressions: []corev1.NodeSelectorRequirement{{
										Key:      "avesha/node-type",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"gateway"},
									}},
								}},
							},
						},
					},
					Containers: []corev1.Container{{
						Name:            "avesha-sidecar",
						Image:           sidecarImg,
						ImagePullPolicy: sidecarPullPolicy,
						Env: []corev1.EnvVar{
							{
								Name:  "SLICE_NAME",
								Value: g.Spec.SliceName,
							},
							{
								Name:  "REMOTE_CLUSTER_ID",
								Value: g.Status.Config.SliceGatewayRemoteClusterID,
							},
							{
								Name:  "GATEWAY_ID",
								Value: g.Status.Config.SliceGatewayID,
							},
							{
								Name:  "REMOTE_GATEWAY_ID",
								Value: g.Status.Config.SliceGatewayRemoteGatewayID,
							},
							{
								Name:  "POD_TYPE",
								Value: "GATEWAY_POD",
							},
							{
								Name:  "OPEN_VPN_MODE",
								Value: "CLIENT",
							},
							{
								Name:  "GW_LOG_LEVEL",
								Value: os.Getenv("GW_LOG_LEVEL"),
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Privileged:               &privileged,
							AllowPrivilegeEscalation: &privileged,
							Capabilities: &corev1.Capabilities{
								Add: []corev1.Capability{
									"NET_ADMIN",
								},
							},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "shared-volume",
							MountPath: "/config",
						}},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								"memory": resource.MustParse("200Mi"),
								"cpu":    resource.MustParse("500m"),
							},
							Requests: corev1.ResourceList{
								"memory": resource.MustParse("50Mi"),
								"cpu":    resource.MustParse("50m"),
							},
						},
					}, {
						Name:            "avesha-openvpn-client",
						Image:           vpnImg,
						ImagePullPolicy: vpnPullPolicy,
						Command: []string{
							"/usr/local/bin/waitForConfigToRunCmd.sh",
						},
						Args: []string{
							"/vpnclient/" + certFileName,
							"90",
							"openvpn",
							"--remote",
							g.Status.Config.SliceGatewayRemoteNodeIP,
							"--port",
							strconv.Itoa(g.Status.Config.SliceGatewayRemoteNodePort),
							"--proto",
							"udp",
							"--config",
							"/vpnclient/" + certFileName,
						},
						SecurityContext: &corev1.SecurityContext{
							Privileged:               &privileged,
							AllowPrivilegeEscalation: &privileged,
							Capabilities: &corev1.Capabilities{
								Add: []corev1.Capability{
									"NET_ADMIN",
								},
							},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "shared-volume",
							MountPath: "/vpnclient",
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: "shared-volume",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName:  g.Name,
								DefaultMode: &vpnSecretDefaultMode,
								Items: []corev1.KeyToPath{
									{
										Key:  "ovpnConfigFile",
										Path: "openvpn_client.ovpn",
									},
								},
							},
						},
					}},
					Tolerations: []corev1.Toleration{{
						Key:      "avesha/node-type",
						Operator: "Equal",
						Effect:   "NoSchedule",
						Value:    "gateway",
					}, {
						Key:      "avesha/node-type",
						Operator: "Equal",
						Effect:   "NoExecute",
						Value:    "gateway",
					}},
				},
			},
		},
	}
	if len(controllers.ImagePullSecretName) != 0 {
		dep.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{
			Name: controllers.ImagePullSecretName,
		}}
	}

	// Set SliceGateway instance as the owner and controller
	ctrl.SetControllerReference(g, dep, r.Scheme)
	return dep
}

func (r *SliceGwReconciler) GetGwPodNameAndIP(ctx context.Context, sliceGw *meshv1beta1.SliceGateway) (string, string) {
	log := logger.FromContext(ctx).WithValues("type", "slicegateway")
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(sliceGw.Namespace),
		client.MatchingLabels(labelsForSliceGwDeployment(sliceGw.Name, sliceGw.Spec.SliceName)),
	}
	if err := r.List(ctx, podList, listOpts...); err != nil {
		log.Error(err, "Failed to list pods")
		return "", ""
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			return pod.Name, pod.Status.PodIP
		}
	}

	return "", ""
}

func isGatewayStatusChanged(slicegateway *meshv1beta1.SliceGateway, podName string, podIP string, status *gwsidecar.GwStatus) bool {
	return slicegateway.Status.PodName != podName ||
		slicegateway.Status.PodIP != podIP ||
		slicegateway.Status.LocalNsmIP != status.NsmStatus.LocalIP
}

func (r *SliceGwReconciler) ReconcileGwPodStatus(ctx context.Context, slicegateway *meshv1beta1.SliceGateway) (ctrl.Result, error, bool) {
	log := logger.FromContext(ctx).WithValues("type", "SliceGw")
	debugLog := log.V(1)

	podName, podIP := r.GetGwPodNameAndIP(ctx, slicegateway)
	if podName == "" || podIP == "" {
		log.Info("Gw pod not available yet, requeuing")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil, true
	}

	sidecarGrpcAddress := podIP + ":5000"

	status, err := gwsidecar.GetStatus(ctx, sidecarGrpcAddress)
	if err != nil {
		log.Error(err, "Unable to fetch gw status")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil, true
	}

	debugLog.Info("Got gw status", "result", status)

	if isGatewayStatusChanged(slicegateway, podName, podIP, status) {
		log.Info("gw status changed")
		slicegateway.Status.PodName = podName
		slicegateway.Status.PodIP = podIP
		slicegateway.Status.LocalNsmIP = status.NsmStatus.LocalIP
		slicegateway.Status.ConnectionContextUpdatedOn = 0
		err = r.Status().Update(ctx, slicegateway)
		if err != nil {
			log.Error(err, "Failed to update SliceGateway status for gateway status")
			return ctrl.Result{}, err, true
		}

		log.Info("gw status updated")
	}

	return ctrl.Result{}, nil, false
}

func (r *SliceGwReconciler) SendConnectionContextToGwPod(ctx context.Context, slicegateway *meshv1beta1.SliceGateway) (ctrl.Result, error, bool) {
	log := logger.FromContext(ctx).WithValues("type", "SliceGw")

	_, podIP := r.GetGwPodNameAndIP(ctx, slicegateway)
	if podIP == "" {
		log.Info("Gw podIP not available yet, requeuing")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil, true
	}

	sidecarGrpcAddress := podIP + ":5000"

	connCtx := &gwsidecar.GwConnectionContext{
		RemoteSliceGwVpnIP:     slicegateway.Status.Config.SliceGatewayRemoteVpnIP,
		RemoteSliceGwNsmSubnet: slicegateway.Status.Config.SliceGatewayRemoteSubnet,
	}

	err := gwsidecar.SendConnectionContext(ctx, sidecarGrpcAddress, connCtx)
	if err != nil {
		log.Error(err, "Unable to send conn ctx to gw")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil, true
	}

	slicegateway.Status.ConnectionContextUpdatedOn = time.Now().Unix()
	err = r.Status().Update(ctx, slicegateway)
	if err != nil {
		log.Error(err, "Failed to update SliceGateway status for conn ctx update")
		return ctrl.Result{}, err, true
	}

	return ctrl.Result{}, nil, false
}

func (r *SliceGwReconciler) SendConnectionContextToSliceRouter(ctx context.Context, slicegateway *meshv1beta1.SliceGateway) (ctrl.Result, error, bool) {
	log := logger.FromContext(ctx).WithValues("type", "SliceGw")

	_, podIP, err := controllers.GetSliceRouterPodNameAndIP(ctx, r.Client, slicegateway.Spec.SliceName)
	if err != nil {
		log.Error(err, "Unable to get slice router pod info")
		return ctrl.Result{}, err, true
	}
	if podIP == "" {
		log.Info("Slice router podIP not available yet, requeuing")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil, true
	}

	if slicegateway.Status.Config.SliceGatewayRemoteSubnet == "" ||
		slicegateway.Status.LocalNsmIP == "" {
		log.Info("Waiting for remote subnet and local nsm IP. Delaying conn ctx update to router")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil, true
	}

	sidecarGrpcAddress := podIP + ":5000"

	connCtx := &router.SliceRouterConnCtx{
		RemoteSliceGwNsmSubnet: slicegateway.Status.Config.SliceGatewayRemoteSubnet,
		LocalNsmGwPeerIP:       slicegateway.Status.LocalNsmIP,
	}

	err = router.SendConnectionContext(ctx, sidecarGrpcAddress, connCtx)
	if err != nil {
		log.Error(err, "Unable to send conn ctx to slice router")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil, true
	}

	return ctrl.Result{}, nil, false
}

func (r *SliceGwReconciler) SyncNetOpConnectionContextAndQos(ctx context.Context, slice *meshv1beta1.Slice, slicegw *meshv1beta1.SliceGateway, sliceGwNodePort int32) error {
	log := logger.FromContext(ctx).WithValues("type", "SliceGw")
	debugLog := log.V(1)

	for i := range r.NetOpPods {
		n := &r.NetOpPods[i]
		debugLog.Info("syncing netop pod", "podName", n.PodName)
		sidecarGrpcAddress := n.PodIP + ":5000"

		err := netop.SendConnectionContext(ctx, sidecarGrpcAddress, slicegw, sliceGwNodePort)
		if err != nil {
			log.Error(err, "Failed to send conn ctx to netop. PodIp: %v, PodName: %v", n.PodIP, n.PodName)
			return err
		}
		err = netop.UpdateSliceQosProfile(ctx, sidecarGrpcAddress, slice)
		if err != nil {
			log.Error(err, "Failed to send qos to netop. PodIp: %v, PodName: %v", n.PodIP, n.PodName)
			return err
		}
	}
	debugLog.Info("netop pods sync complete", "pods", r.NetOpPods)
	return nil
}
