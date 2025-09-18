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
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	neutronv1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

const (
	// ServiceCommand is the command used to start the Neutron service
	ServiceCommand = "/usr/local/bin/kolla_start"
)

// Deployment func
func Deployment(
	instance *neutronv1.NeutronAPI,
	configHash string,
	labels map[string]string,
	annotations map[string]string,
	topology *topologyv1.Topology,
	memcached *memcachedv1.Memcached,
) (*appsv1.Deployment, error) {
	// TODO(lucasagomes): Look into how to implement separated probes
	// for the httpd and neutron-api containers. Right now the code uses
	// the same liveness and readiness probes for both containers which
	// only checks the port 9696 (NeutronPublicPort) which is the port
	// that httpd is listening to. Ideally, we should also include a
	// probe on port 9697 which is the port that neutron-api binds to
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
	args := []string{"-c", ServiceCommand}

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
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	envVars["CONFIG_HASH"] = env.SetValue(configHash)

	// create Volume and VolumeMounts
	volumes := GetVolumes(instance.Name, instance.Spec.ExtraMounts, NeutronAPIPropagation)
	apiVolumeMounts := GetVolumeMounts("neutron-api", instance.Spec.ExtraMounts, NeutronAPIPropagation)
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
		apiVolumeMounts = append(apiVolumeMounts, memcached.CreateMTLSVolumeMounts(nil, nil)...)
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
			CaMount:    ptr.To("/var/lib/config-data/tls/certs/ovndbca.crt"),
		}
		volumes = append(volumes, svc.CreateVolume("ovndb"))
		apiVolumeMounts = append(apiVolumeMounts, svc.CreateVolumeMounts("ovndb")...)
	}

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
					SecurityContext: &corev1.PodSecurityContext{
						FSGroup: ptr.To(NeutronUID),
					},
					ServiceAccountName: instance.RbacResourceName(),
					Containers: []corev1.Container{
						{
							Name:                     ServiceName + "-api",
							Command:                  []string{"/bin/bash"},
							Args:                     args,
							Image:                    instance.Spec.ContainerImage,
							SecurityContext:          getNeutronSecurityContext(),
							Env:                      env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts:             apiVolumeMounts,
							Resources:                instance.Spec.Resources,
							LivenessProbe:            livenessProbe,
							TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
						},
						{
							Name:                     ServiceName + "-httpd",
							Command:                  []string{"/bin/bash"},
							Args:                     args,
							Image:                    instance.Spec.ContainerImage,
							SecurityContext:          getNeutronSecurityContext(),
							Env:                      env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts:             httpdVolumeMounts,
							Resources:                instance.Spec.Resources,
							ReadinessProbe:           readinessProbe,
							LivenessProbe:            livenessProbe,
							TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

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
				ServiceName,
			},
			corev1.LabelHostname,
		)
	}

	return deployment, nil
}
