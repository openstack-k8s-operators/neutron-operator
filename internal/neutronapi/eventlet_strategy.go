package neutronapi

import (
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	neutronv1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// EventletStrategy implements the traditional 2-container deployment with neutron-server + httpd
type EventletStrategy struct{}

// GetContainers returns the neutron-api and neutron-httpd containers for eventlet deployment
func (e *EventletStrategy) GetContainers(instance *neutronv1.NeutronAPI, configHash string, _ []corev1.Volume) ([]corev1.Container, error) {
	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	envVars["CONFIG_HASH"] = env.SetValue(configHash)

	args := []string{"-c", ServiceCommand}

	// Get volume mounts for each container
	apiVolumeMounts := GetVolumeMounts("neutron-api", instance.Spec.ExtraMounts, NeutronAPIPropagation)
	httpdVolumeMounts := GetHttpdVolumeMount()

	// Note: TLS volume mounts are handled by applyTLSMountsToContainers in deployment.go

	livenessProbe, readinessProbe := e.GetProbes(instance)

	return []corev1.Container{
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
	}, nil
}

// GetConfigTemplates returns empty map as eventlet uses existing templates
func (e *EventletStrategy) GetConfigTemplates() map[string]string {
	return map[string]string{}
}

// GetServicePorts returns the standard neutron service ports
func (e *EventletStrategy) GetServicePorts() []corev1.ServicePort {
	return []corev1.ServicePort{
		{
			Name:     "public",
			Port:     NeutronPublicPort,
			Protocol: corev1.ProtocolTCP,
		},
		{
			Name:     "internal",
			Port:     NeutronInternalPort,
			Protocol: corev1.ProtocolTCP,
		},
	}
}

// GetProbes returns HTTP probes for the httpd container
func (e *EventletStrategy) GetProbes(instance *neutronv1.NeutronAPI) (*corev1.Probe, *corev1.Probe) {
	livenessProbe := &corev1.Probe{
		TimeoutSeconds:      30,
		PeriodSeconds:       30,
		InitialDelaySeconds: 5,
	}
	livenessProbe.HTTPGet = &corev1.HTTPGetAction{
		Path: "/",
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(NeutronPublicPort)},
	}

	readinessProbe := &corev1.Probe{
		TimeoutSeconds:      30,
		PeriodSeconds:       30,
		InitialDelaySeconds: 5,
	}
	readinessProbe.HTTPGet = &corev1.HTTPGetAction{
		Path: "/",
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(NeutronPublicPort)},
	}

	// Enable HTTPS probes if TLS is configured
	if instance.Spec.TLS.API.Enabled(service.EndpointPublic) {
		livenessProbe.HTTPGet.Scheme = corev1.URISchemeHTTPS
		readinessProbe.HTTPGet.Scheme = corev1.URISchemeHTTPS
	}

	return livenessProbe, readinessProbe
}

// GetDeploymentType returns identifier for this strategy
func (e *EventletStrategy) GetDeploymentType() string {
	return "eventlet"
}

// GetVolumeMounts returns the volume mounts for eventlet deployment
func (e *EventletStrategy) GetVolumeMounts(instance *neutronv1.NeutronAPI, _ []corev1.Volume) map[string][]corev1.VolumeMount {
	return map[string][]corev1.VolumeMount{
		"neutron-api":   GetVolumeMounts("neutron-api", instance.Spec.ExtraMounts, NeutronAPIPropagation),
		"neutron-httpd": GetHttpdVolumeMount(),
	}
}
