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
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/affinity"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/pod"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	"github.com/openstack-k8s-operators/lib-common/modules/common/volume"
	"github.com/openstack-k8s-operators/lib-common/modules/users"
	neutronv1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

// NeutronAPICommand is the command used to run the native neutron-server
// process (the neutron-api container). Only used under the legacy Eventlet
// strategy (wsgi=false) -- under WSGI, httpd/mod_wsgi loads the API
// in-process and this container is not created at all.
const NeutronAPICommand = "neutron-server --config-file /usr/share/neutron/neutron-dist.conf " +
	"--config-file /etc/neutron/neutron.conf --config-dir /etc/neutron/neutron.conf.d"

// NeutronHttpdCommand is the command used to run httpd. Under both
// strategies httpd is the container that serves port 9696: as a reverse
// proxy to the eventlet neutron-api process (wsgi=false), or as the
// mod_wsgi host of the API application itself (wsgi=true).
const NeutronHttpdCommand = "httpd -DFOREGROUND"

// Deployment func
func Deployment(
	instance *neutronv1.NeutronAPI,
	configHash string,
	labels map[string]string,
	annotations map[string]string,
	topology *topologyv1.Topology,
	memcached *memcachedv1.Memcached,
	wsgi bool,
) (*appsv1.Deployment, error) {
	// TODO(lucasagomes): Look into how to implement separated probes
	// for the httpd and neutron-api containers under the Eventlet (wsgi=false)
	// strategy. Right now the code uses the same liveness and readiness
	// probes for both containers which only checks the port 9696
	// (NeutronPublicPort) which is the port that httpd is listening to.
	// Ideally, we should also include a probe on port 9697 which is the
	// port that neutron-api binds to. Under the WSGI (wsgi=true) strategy
	// this isn't a gap: httpd/mod_wsgi *is* the API process, so probing
	// port 9696 accurately reflects the API's health.
	livenessProbe := &corev1.Probe{
		TimeoutSeconds:      30,
		PeriodSeconds:       30,
		InitialDelaySeconds: 5,
	}
	readinessProbe := &corev1.Probe{
		TimeoutSeconds:      30,
		PeriodSeconds:       30,
		InitialDelaySeconds: 5,
	}
	apiArgs := []string{"-c", NeutronAPICommand}
	httpdArgs := []string{"-c", NeutronHttpdCommand}

	//
	// https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/
	//
	livenessProbe.HTTPGet = &corev1.HTTPGetAction{
		Path: "/",
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(NeutronPublicPort)},
	}
	readinessProbe.HTTPGet = &corev1.HTTPGetAction{
		Path: "/",
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(NeutronPublicPort)},
	}

	if instance.Spec.TLS.API.Enabled(service.EndpointPublic) {
		livenessProbe.HTTPGet.Scheme = corev1.URISchemeHTTPS
		readinessProbe.HTTPGet.Scheme = corev1.URISchemeHTTPS
	}

	envVars := map[string]env.Setter{}
	envVars["CONFIG_HASH"] = env.SetValue(configHash)

	// create Volume and VolumeMounts
	volumes := append(GetVolumes(instance.Name, instance.Spec.ExtraMounts, NeutronAPIPropagation), volume.WritableDirVolume(volume.RunHttpdVolumeName))
	policyOverwrite := len(instance.Spec.DefaultConfigOverwrite["policy.yaml"]) > 0
	apiVolumeMounts := GetVolumeMounts(instance.Spec.ExtraMounts, NeutronAPIPropagation, policyOverwrite)
	httpdVolumeMounts := GetHttpdVolumeMount()

	// add CA cert if defined
	if instance.Spec.TLS.CaBundleSecretName != "" {
		volumes = append(volumes, instance.Spec.TLS.CreateVolume())
		apiVolumeMounts = append(apiVolumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
		httpdVolumeMounts = append(httpdVolumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
	}

	// add MTLS cert if defined
	if memcached.Status.MTLSCert != "" {
		volumes = append(volumes, memcached.CreateMTLSVolume())
		certMountPath := memcachedv1.CertPathDst
		keyMountPath := memcachedv1.KeyPathDst
		apiVolumeMounts = append(apiVolumeMounts, memcached.CreateMTLSVolumeMounts(&certMountPath, &keyMountPath)...)
	}

	for _, endpt := range []service.Endpoint{service.EndpointInternal, service.EndpointPublic} {
		if instance.Spec.TLS.API.Enabled(endpt) {
			var tlsEndptCfg tls.GenericService
			switch endpt {
			case service.EndpointPublic:
				tlsEndptCfg = instance.Spec.TLS.API.Public
			case service.EndpointInternal:
				tlsEndptCfg = instance.Spec.TLS.API.Internal
			}

			svc, err := tlsEndptCfg.ToService()
			if err != nil {
				return nil, err
			}
			// httpd container is not using kolla, mount the certs to its dst
			svc.CertMount = ptr.To(fmt.Sprintf("/etc/pki/tls/certs/%s.crt", endpt.String()))
			svc.KeyMount = ptr.To(fmt.Sprintf("/etc/pki/tls/private/%s.key", endpt.String()))

			volumes = append(volumes, svc.CreateVolume(endpt.String()))
			httpdVolumeMounts = append(httpdVolumeMounts, svc.CreateVolumeMounts(endpt.String())...)
		}
	}

	if instance.IsOVNEnabled() && instance.Spec.TLS.Ovn.Enabled() {
		svc := tls.Service{
			SecretName: *instance.Spec.TLS.Ovn.SecretName,
			// ovn_{nb,sb}_certificate/private_key/ca_cert in 01-neutron.conf
			// point directly here -- mount at the final paths, not the
			// kolla-era staging paths CertMount/KeyMount default to.
			CertMount: ptr.To("/etc/pki/tls/certs/ovndb.crt"),
			KeyMount:  ptr.To("/etc/pki/tls/private/ovndb.key"),
			CaMount:   ptr.To("/etc/pki/tls/certs/ovndbca.crt"),
		}
		volumes = append(volumes, svc.CreateVolume("ovndb"))
		apiVolumeMounts = append(apiVolumeMounts, svc.CreateVolumeMounts("ovndb")...)
	}

	// Under the WSGI strategy httpd/mod_wsgi loads the neutron API
	// application in-process, so the httpd container needs everything the
	// eventlet neutron-api container would otherwise mount (neutron config,
	// OVN client certs, etc) in addition to its own httpd config/TLS mounts.
	// There is no separate neutron-api container at all in this mode.
	// Some mounts (e.g. the CA bundle) are deliberately added to both
	// apiVolumeMounts and httpdVolumeMounts above so that each ends up on
	// whichever container needs it under the Eventlet strategy, where they
	// land in two different containers -- merged into a single container
	// here, they must be de-duplicated by MountPath or the Deployment is
	// rejected ("must be unique").
	httpdContainerVolumeMounts := httpdVolumeMounts
	if wsgi {
		httpdContainerVolumeMounts = dedupeVolumeMountsByPath(
			append(append([]corev1.VolumeMount{}, httpdVolumeMounts...), apiVolumeMounts...))
		envVars["OS_NEUTRON_CONFIG_DIR"] = env.SetValue("/etc/neutron/neutron.conf.d")
		envVars["OS_NEUTRON_CONFIG_FILES"] = env.SetValue("01-neutron.conf")
		if instance.Spec.CustomServiceConfig != "" {
			envVars["OS_NEUTRON_CONFIG_FILES"] = env.SetValue("01-neutron.conf;02-neutron-custom.conf")
		}
	}

	containers := []corev1.Container{}
	if !wsgi {
		containers = append(containers, corev1.Container{
			Name:                     ServiceName + "-api",
			Command:                  []string{"/bin/bash"},
			Args:                     apiArgs,
			Image:                    instance.Spec.ContainerImage,
			SecurityContext:          pod.RestrictiveSecurityContext(users.NeutronUID, users.NeutronGID),
			Env:                      env.MergeEnvs([]corev1.EnvVar{}, envVars),
			VolumeMounts:             apiVolumeMounts,
			Resources:                instance.Spec.Resources,
			LivenessProbe:            livenessProbe,
			TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
		})
	}
	containers = append(containers, corev1.Container{
		Name:                     ServiceName + "-httpd",
		Command:                  []string{"/bin/bash"},
		Args:                     httpdArgs,
		Image:                    instance.Spec.ContainerImage,
		SecurityContext:          pod.RestrictiveSecurityContext(users.NeutronUID, users.NeutronGID),
		Env:                      env.MergeEnvs([]corev1.EnvVar{}, envVars),
		VolumeMounts:             httpdContainerVolumeMounts,
		Resources:                instance.Spec.Resources,
		ReadinessProbe:           readinessProbe,
		LivenessProbe:            livenessProbe,
		TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
	})

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceName,
			Namespace: instance.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: instance.Spec.Replicas,
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

	return deployment, nil
}

// dedupeVolumeMountsByPath drops later entries that reuse a MountPath
// already claimed by an earlier one. The Kubernetes API rejects a container
// with two VolumeMounts sharing a MountPath, which can happen once mounts
// built for two separate containers (API and httpd, under Eventlet) are
// merged onto a single one (under WSGI).
func dedupeVolumeMountsByPath(mounts []corev1.VolumeMount) []corev1.VolumeMount {
	seen := make(map[string]bool, len(mounts))
	deduped := make([]corev1.VolumeMount, 0, len(mounts))
	for _, m := range mounts {
		if seen[m.MountPath] {
			continue
		}
		seen[m.MountPath] = true
		deduped = append(deduped, m)
	}
	return deduped
}

// applyPlacement applies NodeSelector, Topology and anti-affinity rules
// shared by all NeutronAPI-owned Deployments (API, RPC, worker).
func applyPlacement(instance *neutronv1.NeutronAPI, deployment *appsv1.Deployment, topology *topologyv1.Topology) {
	if instance.Spec.NodeSelector != nil {
		deployment.Spec.Template.Spec.NodeSelector = *instance.Spec.NodeSelector
	}

	if topology != nil {
		topology.ApplyTo(&deployment.Spec.Template)
	} else {
		// If possible two pods of the same service should not
		// run on the same worker node. If this is not possible
		// the get still created on the same worker node.
		deployment.Spec.Template.Spec.Affinity = affinity.DistributePods(
			common.AppSelector,
			[]string{
				deployment.Name,
			},
			corev1.LabelHostname,
		)
	}
}
