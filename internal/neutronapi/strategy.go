package neutronapi

import (
	neutronv1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

// DeploymentStrategy defines the interface for different neutron deployment approaches
type DeploymentStrategy interface {
	// GetContainers returns the list of containers for this deployment strategy
	GetContainers(instance *neutronv1.NeutronAPI, configHash string, volumes []corev1.Volume) ([]corev1.Container, error)

	// GetConfigTemplates returns additional template files needed for this strategy
	GetConfigTemplates() map[string]string

	// GetServicePorts returns the ports that should be exposed by the service
	GetServicePorts() []corev1.ServicePort

	// GetProbes returns liveness and readiness probes for the main service
	GetProbes(instance *neutronv1.NeutronAPI) (*corev1.Probe, *corev1.Probe)

	// GetDeploymentType returns a string identifier for this deployment type
	GetDeploymentType() string

	// GetVolumeMounts returns additional volume mounts specific to this strategy
	GetVolumeMounts(instance *neutronv1.NeutronAPI, volumes []corev1.Volume) map[string][]corev1.VolumeMount
}
