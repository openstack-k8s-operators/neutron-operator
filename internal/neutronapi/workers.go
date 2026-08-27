/*
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

package neutronapi

import (
	"fmt"

	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/pod"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	"github.com/openstack-k8s-operators/lib-common/modules/users"
	neutronv1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// These commands deliberately do not pass --config-file for
// /usr/share/neutron/neutron-dist.conf or /etc/neutron/neutron.conf: unlike
// kolla, this operator never materializes those paths, and oslo.config
// aborts if an explicitly named --config-file doesn't exist. --config-dir
// alone (loading 01-neutron.conf then 02-neutron-custom.conf in order) is
// sufficient -- see DbSyncCommand and OSPRH-28742's db-sync-config.json.

// NeutronRPCCommand runs the neutron-rpc-server process, which handles AMQP
// RPC calls (e.g. from neutron agents) independently from the WSGI API
// process. WSGI strategy only.
const NeutronRPCCommand = "neutron-rpc-server --config-dir /etc/neutron/neutron.conf.d"

// NeutronPeriodicWorkersCommand runs neutron's periodic background tasks
// (e.g. stale resource cleanup). WSGI strategy only.
const NeutronPeriodicWorkersCommand = "neutron-periodic-workers --config-dir /etc/neutron/neutron.conf.d"

// NeutronOVNMaintenanceCommand runs the ML2/OVN mechanism driver's
// maintenance tasks. WSGI strategy only, and only when ovn is one of the
// configured Ml2MechanismDrivers.
const NeutronOVNMaintenanceCommand = "neutron-ovn-maintenance-worker --config-dir /etc/neutron/neutron.conf.d"

// workerVolumesAndMounts builds the Volumes/VolumeMounts shared by the
// non-API Deployments (neutron-rpc, neutron-worker): neutron.conf.d, CA
// bundle, memcached mTLS and the OVN DB client cert. This mirrors what
// Deployment() mounts into the neutron-api container, minus the
// httpd-specific config/TLS mounts those processes don't need.
func workerVolumesAndMounts(
	instance *neutronv1.NeutronAPI,
	memcached *memcachedv1.Memcached,
) ([]corev1.Volume, []corev1.VolumeMount) {
	volumes := GetVolumes(instance.Name, instance.Spec.ExtraMounts, NeutronAPIPropagation)
	policyOverwrite := len(instance.Spec.DefaultConfigOverwrite["policy.yaml"]) > 0
	volumeMounts := GetVolumeMounts(instance.Spec.ExtraMounts, NeutronAPIPropagation, policyOverwrite)

	if instance.Spec.TLS.CaBundleSecretName != "" {
		volumes = append(volumes, instance.Spec.TLS.CreateVolume())
		volumeMounts = append(volumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
	}

	if memcached.Status.MTLSCert != "" {
		volumes = append(volumes, memcached.CreateMTLSVolume())
		certMountPath := memcachedv1.CertPathDst
		keyMountPath := memcachedv1.KeyPathDst
		volumeMounts = append(volumeMounts, memcached.CreateMTLSVolumeMounts(&certMountPath, &keyMountPath)...)
	}

	if instance.IsOVNEnabled() && instance.Spec.TLS.Ovn.Enabled() {
		svc := tls.Service{
			SecretName: *instance.Spec.TLS.Ovn.SecretName,
			CertMount:  ptr.To("/etc/pki/tls/certs/ovndb.crt"),
			KeyMount:   ptr.To("/etc/pki/tls/private/ovndb.key"),
			CaMount:    ptr.To("/etc/pki/tls/certs/ovndbca.crt"),
		}
		volumes = append(volumes, svc.CreateVolume("ovndb"))
		volumeMounts = append(volumeMounts, svc.CreateVolumeMounts("ovndb")...)
	}

	return volumes, volumeMounts
}

// RPCDeployment builds the Deployment that runs neutron-rpc-server. Callers
// are expected to skip calling this entirely when rpc_workers=0 is set in
// instance.Spec.CustomServiceConfig (see neutronv1.GetRPCWorkers), rather
// than creating it with Replicas=0: the RPC worker has no HTTP endpoint to
// scale behind, so "disabled" means "the Deployment does not exist".
func RPCDeployment(
	instance *neutronv1.NeutronAPI,
	configHash string,
	labels map[string]string,
	annotations map[string]string,
	topology *topologyv1.Topology,
	memcached *memcachedv1.Memcached,
) *appsv1.Deployment {
	volumes, volumeMounts := workerVolumesAndMounts(instance, memcached)

	envVars := map[string]env.Setter{}
	envVars["CONFIG_HASH"] = env.SetValue(configHash)
	envVars["OS_NEUTRON_CONFIG_DIR"] = env.SetValue("/etc/neutron/neutron.conf.d")
	envVars["OS_NEUTRON_CONFIG_FILES"] = env.SetValue("01-neutron.conf")
	if instance.Spec.CustomServiceConfig != "" {
		envVars["OS_NEUTRON_CONFIG_FILES"] = env.SetValue("01-neutron.conf;02-neutron-custom.conf")
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", ServiceName, RPCDeploymentSuffix),
			Namespace: instance.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: ptr.To(int32(1)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
					Labels:      labels,
				},
				Spec: corev1.PodSpec{
					SecurityContext:              pod.RestrictivePodSecurityContext(users.NeutronUID, users.NeutronGID),
					ServiceAccountName:           instance.RbacResourceName(),
					AutomountServiceAccountToken: ptr.To(false),
					Containers: []corev1.Container{
						{
							Name:                     ServiceName + "-" + RPCDeploymentSuffix,
							Command:                  []string{"/bin/bash"},
							Args:                     []string{"-c", NeutronRPCCommand},
							Image:                    instance.Spec.ContainerImage,
							SecurityContext:          pod.RestrictiveSecurityContext(users.NeutronUID, users.NeutronGID),
							Env:                      env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts:             volumeMounts,
							Resources:                instance.Spec.Resources,
							TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

	applyPlacement(instance, deployment, topology)
	return deployment
}

// WorkerDeployment builds the Deployment that runs neutron's background
// workers: periodic tasks, plus (when ovn is a configured ML2 mechanism
// driver) the OVN maintenance worker, as separate containers of the same
// pod. Unlike RPCDeployment there is no customServiceConfig knob to disable
// this Deployment; the OVN container is simply omitted when OVN is disabled.
func WorkerDeployment(
	instance *neutronv1.NeutronAPI,
	configHash string,
	labels map[string]string,
	annotations map[string]string,
	topology *topologyv1.Topology,
	memcached *memcachedv1.Memcached,
) *appsv1.Deployment {
	volumes, volumeMounts := workerVolumesAndMounts(instance, memcached)

	envVars := map[string]env.Setter{}
	envVars["CONFIG_HASH"] = env.SetValue(configHash)
	envVars["OS_NEUTRON_CONFIG_DIR"] = env.SetValue("/etc/neutron/neutron.conf.d")
	envVars["OS_NEUTRON_CONFIG_FILES"] = env.SetValue("01-neutron.conf")
	if instance.Spec.CustomServiceConfig != "" {
		envVars["OS_NEUTRON_CONFIG_FILES"] = env.SetValue("01-neutron.conf;02-neutron-custom.conf")
	}

	containers := []corev1.Container{
		{
			Name:                     ServiceName + "-periodic-workers",
			Command:                  []string{"/bin/bash"},
			Args:                     []string{"-c", NeutronPeriodicWorkersCommand},
			Image:                    instance.Spec.ContainerImage,
			SecurityContext:          pod.RestrictiveSecurityContext(users.NeutronUID, users.NeutronGID),
			Env:                      env.MergeEnvs([]corev1.EnvVar{}, envVars),
			VolumeMounts:             volumeMounts,
			Resources:                instance.Spec.Resources,
			TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
		},
	}

	if instance.IsOVNEnabled() {
		containers = append(containers, corev1.Container{
			Name:                     ServiceName + "-ovn-maintenance-worker",
			Command:                  []string{"/bin/bash"},
			Args:                     []string{"-c", NeutronOVNMaintenanceCommand},
			Image:                    instance.Spec.ContainerImage,
			SecurityContext:          pod.RestrictiveSecurityContext(users.NeutronUID, users.NeutronGID),
			Env:                      env.MergeEnvs([]corev1.EnvVar{}, envVars),
			VolumeMounts:             volumeMounts,
			Resources:                instance.Spec.Resources,
			TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
		})
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", ServiceName, WorkerDeploymentSuffix),
			Namespace: instance.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: ptr.To(int32(1)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
					Labels:      labels,
				},
				Spec: corev1.PodSpec{
					SecurityContext:              pod.RestrictivePodSecurityContext(users.NeutronUID, users.NeutronGID),
					ServiceAccountName:           instance.RbacResourceName(),
					AutomountServiceAccountToken: ptr.To(false),
					Containers:                   containers,
					Volumes:                      volumes,
				},
			},
		},
	}

	applyPlacement(instance, deployment, topology)
	return deployment
}
