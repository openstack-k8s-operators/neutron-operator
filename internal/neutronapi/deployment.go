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
	"context"
	"fmt"

	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/affinity"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	neutronv1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ServiceCommand is the command used to start the Neutron service
	ServiceCommand = "/usr/local/bin/kolla_start"
)

// Deployment func
func Deployment(
	ctx context.Context,
	client client.Client,
	instance *neutronv1.NeutronAPI,
	configHash string,
	labels map[string]string,
	annotations map[string]string,
	topology *topologyv1.Topology,
	memcached *memcachedv1.Memcached,
) (*appsv1.Deployment, error) {
	// Detect deployment strategy based on container image
	detector := NewStrategyDetector(client)
	strategy, err := detector.DetectStrategy(ctx, instance)
	if err != nil {
		return nil, fmt.Errorf("failed to detect deployment strategy: %w", err)
	}

	// create Volume and VolumeMounts with strategy awareness
	volumes := GetVolumesForStrategy(instance.Name, instance.Spec.ExtraMounts, NeutronAPIPropagation, strategy.GetDeploymentType())

	// add CA cert if defined
	if instance.Spec.TLS.CaBundleSecretName != "" {
		volumes = append(volumes, instance.Spec.TLS.CreateVolume())
	}

	// add MTLS cert if defined
	if memcached.Status.MTLSCert != "" {
		volumes = append(volumes, memcached.CreateMTLSVolume())
	}

	// Handle TLS certificates for API endpoints
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
			// For eventlet strategy, httpd container needs specific cert mount paths
			if strategy.GetDeploymentType() == "eventlet" {
				svc.CertMount = ptr.To(fmt.Sprintf("/etc/pki/tls/certs/%s.crt", endpt.String()))
				svc.KeyMount = ptr.To(fmt.Sprintf("/etc/pki/tls/private/%s.key", endpt.String()))
			}
			// For uwsgi, gunicorn, and httpd strategies, container needs specific cert mount paths
			if strategy.GetDeploymentType() == "uwsgi" || strategy.GetDeploymentType() == "gunicorn" || strategy.GetDeploymentType() == "httpd" {
				svc.CertMount = ptr.To(fmt.Sprintf("/etc/pki/tls/certs/%s.crt", endpt.String()))
				svc.KeyMount = ptr.To(fmt.Sprintf("/etc/pki/tls/private/%s.key", endpt.String()))
			}

			volumes = append(volumes, svc.CreateVolume(endpt.String()))
		}
	}

	// Add OVN TLS if enabled
	if instance.IsOVNEnabled() && instance.Spec.TLS.Ovn.Enabled() {
		svc := tls.Service{
			SecretName: *instance.Spec.TLS.Ovn.SecretName,
			CaMount:    ptr.To("/var/lib/config-data/tls/certs/ovndbca.crt"),
		}
		volumes = append(volumes, svc.CreateVolume("ovndb"))
	}

	// Generate strategy-specific containers
	containers, err := strategy.GetContainers(instance, configHash, volumes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate containers: %w", err)
	}

	// Apply TLS volume mounts to containers based on strategy
	err = applyTLSMountsToContainers(instance, memcached, strategy, containers)
	if err != nil {
		return nil, fmt.Errorf("failed to apply TLS mounts: %w", err)
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
					Containers:         containers,
					Volumes:            volumes,
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

// applyTLSMountsToContainers adds TLS-related volume mounts to containers based on strategy
func applyTLSMountsToContainers(
	instance *neutronv1.NeutronAPI,
	memcached *memcachedv1.Memcached,
	strategy DeploymentStrategy,
	containers []corev1.Container,
) error {
	// Apply CA certificate mounts to all containers if defined
	if instance.Spec.TLS.CaBundleSecretName != "" {
		caMounts := instance.Spec.TLS.CreateVolumeMounts(nil)
		for i := range containers {
			containers[i].VolumeMounts = append(containers[i].VolumeMounts, caMounts...)
		}
	}

	// Apply MTLS certificate mounts to all containers if defined
	if memcached.Status.MTLSCert != "" {
		mtlsMounts := memcached.CreateMTLSVolumeMounts(nil, nil)
		for i := range containers {
			containers[i].VolumeMounts = append(containers[i].VolumeMounts, mtlsMounts...)
		}
	}

	// Apply TLS API endpoint mounts based on strategy
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
				return err
			}

			var tlsMounts []corev1.VolumeMount
			if strategy.GetDeploymentType() == "eventlet" {
				// For eventlet strategy, apply TLS certs to httpd container
				svc.CertMount = ptr.To(fmt.Sprintf("/etc/pki/tls/certs/%s.crt", endpt.String()))
				svc.KeyMount = ptr.To(fmt.Sprintf("/etc/pki/tls/private/%s.key", endpt.String()))
				tlsMounts = svc.CreateVolumeMounts(endpt.String())

				// Apply only to httpd container
				for i := range containers {
					if containers[i].Name == ServiceName+"-httpd" {
						containers[i].VolumeMounts = append(containers[i].VolumeMounts, tlsMounts...)
					}
				}
			} else if strategy.GetDeploymentType() == "uwsgi" {
				// For uwsgi strategy, apply TLS certs to uwsgi container
				svc.CertMount = ptr.To(fmt.Sprintf("/etc/pki/tls/certs/%s.crt", endpt.String()))
				svc.KeyMount = ptr.To(fmt.Sprintf("/etc/pki/tls/private/%s.key", endpt.String()))
				tlsMounts = svc.CreateVolumeMounts(endpt.String())

				// Apply only to uwsgi container
				for i := range containers {
					if containers[i].Name == ServiceName+"-uwsgi" {
						containers[i].VolumeMounts = append(containers[i].VolumeMounts, tlsMounts...)
					}
				}
			} else if strategy.GetDeploymentType() == "gunicorn" {
				// For gunicorn strategy, apply TLS certs to gunicorn container
				svc.CertMount = ptr.To(fmt.Sprintf("/etc/pki/tls/certs/%s.crt", endpt.String()))
				svc.KeyMount = ptr.To(fmt.Sprintf("/etc/pki/tls/private/%s.key", endpt.String()))
				tlsMounts = svc.CreateVolumeMounts(endpt.String())

				// Apply only to gunicorn container
				for i := range containers {
					if containers[i].Name == ServiceName+"-gunicorn" {
						containers[i].VolumeMounts = append(containers[i].VolumeMounts, tlsMounts...)
					}
				}
			} else if strategy.GetDeploymentType() == "httpd" {
				// For httpd strategy, apply TLS certs to httpd container
				svc.CertMount = ptr.To(fmt.Sprintf("/etc/pki/tls/certs/%s.crt", endpt.String()))
				svc.KeyMount = ptr.To(fmt.Sprintf("/etc/pki/tls/private/%s.key", endpt.String()))
				tlsMounts = svc.CreateVolumeMounts(endpt.String())

				// Apply only to httpd container
				for i := range containers {
					if containers[i].Name == ServiceName+"-httpd" {
						containers[i].VolumeMounts = append(containers[i].VolumeMounts, tlsMounts...)
					}
				}
			}
		}
	}

	// Apply OVN TLS mounts to relevant containers if enabled
	if instance.IsOVNEnabled() && instance.Spec.TLS.Ovn.Enabled() {
		svc := tls.Service{
			SecretName: *instance.Spec.TLS.Ovn.SecretName,
			CaMount:    ptr.To("/var/lib/config-data/tls/certs/ovndbca.crt"),
		}
		ovnMounts := svc.CreateVolumeMounts("ovndb")

		// Apply to all containers that need neutron config access
		for i := range containers {
			containers[i].VolumeMounts = append(containers[i].VolumeMounts, ovnMounts...)
		}
	}

	return nil
}
