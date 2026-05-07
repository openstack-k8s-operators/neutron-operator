package neutronapi

import (
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	neutronv1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// UwsgiStrategy implements the new multi-container deployment with uwsgi + separate workers
type UwsgiStrategy struct{}

// GetContainers returns the uwsgi and worker containers for the new deployment
func (u *UwsgiStrategy) GetContainers(instance *neutronv1.NeutronAPI, configHash string, _ []corev1.Volume) ([]corev1.Container, error) {
	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")
	envVars["CONFIG_HASH"] = env.SetValue(configHash)
	envVars["OS_NEUTRON_CONFIG_DIR"] = env.SetValue("/etc/neutron/neutron.conf.d")
	envVars["OS_NEUTRON_CONFIG_FILES"] = env.SetValue("01-neutron.conf")
	if instance.Spec.CustomServiceConfig != "" {
		envVars["OS_NEUTRON_CONFIG_FILES"] = env.SetValue("01-neutron.conf;02-neutron-custom.conf")
	}

	// Get container-specific volume mounts - each container needs its own config.json
	uwsgiVolumeMounts := GetVolumeMounts("neutron-uwsgi", instance.Spec.ExtraMounts, NeutronAPIPropagation)
	periodicVolumeMounts := GetVolumeMounts("neutron-periodic-workers", instance.Spec.ExtraMounts, NeutronAPIPropagation)
	ovnVolumeMounts := GetVolumeMounts("neutron-ovn-maintenance-worker", instance.Spec.ExtraMounts, NeutronAPIPropagation)
	rpcVolumeMounts := GetVolumeMounts("neutron-rpc-server", instance.Spec.ExtraMounts, NeutronAPIPropagation)

	// Note: TLS volume mounts are handled by applyTLSMountsToContainers in deployment.go

	livenessProbe, readinessProbe := u.GetProbes(instance)

	containers := []corev1.Container{
		{
			Name:                     ServiceName + "-uwsgi",
			Command:                  []string{"/bin/bash"},
			Args:                     []string{"-c", ServiceCommand},
			Image:                    instance.Spec.ContainerImage,
			SecurityContext:          getNeutronSecurityContext(),
			Env:                      env.MergeEnvs([]corev1.EnvVar{}, envVars),
			VolumeMounts:             uwsgiVolumeMounts,
			Resources:                instance.Spec.Resources,
			LivenessProbe:            livenessProbe,
			ReadinessProbe:           readinessProbe,
			TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
			Ports: []corev1.ContainerPort{
				{
					Name:          "neutron-api",
					ContainerPort: NeutronPublicPort,
					Protocol:      corev1.ProtocolTCP,
				},
			},
		},
		{
			Name:                     ServiceName + "-periodic-workers",
			Command:                  []string{"/bin/bash"},
			Args:                     []string{"-c", ServiceCommand},
			Image:                    instance.Spec.ContainerImage,
			SecurityContext:          getNeutronSecurityContext(),
			Env:                      env.MergeEnvs([]corev1.EnvVar{}, envVars),
			VolumeMounts:             periodicVolumeMounts,
			Resources:                instance.Spec.Resources,
			TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
		},
		{
			Name:                     ServiceName + "-ovn-maintenance-worker",
			Command:                  []string{"/bin/bash"},
			Args:                     []string{"-c", ServiceCommand},
			Image:                    instance.Spec.ContainerImage,
			SecurityContext:          getNeutronSecurityContext(),
			Env:                      env.MergeEnvs([]corev1.EnvVar{}, envVars),
			VolumeMounts:             ovnVolumeMounts,
			Resources:                instance.Spec.Resources,
			TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
		},
		{
			Name:                     ServiceName + "-rpc-server",
			Command:                  []string{"/bin/bash"},
			Args:                     []string{"-c", ServiceCommand},
			Image:                    instance.Spec.ContainerImage,
			SecurityContext:          getNeutronSecurityContext(),
			Env:                      env.MergeEnvs([]corev1.EnvVar{}, envVars),
			VolumeMounts:             rpcVolumeMounts,
			Resources:                instance.Spec.Resources,
			TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
		},
	}

	return containers, nil
}

// GetConfigTemplates returns additional templates needed for uwsgi deployment
func (u *UwsgiStrategy) GetConfigTemplates() map[string]string {
	return map[string]string{
		"neutron-uwsgi-config.json":                  "/neutronapi/config/neutron-uwsgi-config.json",
		"neutron-uwsgi.ini":                          "/neutronapi/uwsgi/neutron-uwsgi.ini",
		"neutron-periodic-workers-config.json":       "/neutronapi/config/neutron-periodic-workers-config.json",
		"neutron-ovn-maintenance-worker-config.json": "/neutronapi/config/neutron-ovn-maintenance-worker-config.json",
		"neutron-rpc-server-config.json":             "/neutronapi/config/neutron-rpc-server-config.json",
	}
}

// GetServicePorts returns the neutron service ports for uwsgi
func (u *UwsgiStrategy) GetServicePorts() []corev1.ServicePort {
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

// GetProbes returns HTTP probes for the uwsgi container
func (u *UwsgiStrategy) GetProbes(_ *neutronv1.NeutronAPI) (*corev1.Probe, *corev1.Probe) {
	livenessProbe := &corev1.Probe{
		TimeoutSeconds:      30,
		PeriodSeconds:       30,
		InitialDelaySeconds: 10, // Slightly longer delay for uwsgi startup
	}
	livenessProbe.HTTPGet = &corev1.HTTPGetAction{
		Path: "/",
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(NeutronInternalAPIPort)},
	}

	readinessProbe := &corev1.Probe{
		TimeoutSeconds:      30,
		PeriodSeconds:       30,
		InitialDelaySeconds: 5,
	}
	readinessProbe.HTTPGet = &corev1.HTTPGetAction{
		Path: "/",
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: int32(NeutronInternalAPIPort)},
	}

	livenessProbe.HTTPGet.Scheme = corev1.URISchemeHTTP
	readinessProbe.HTTPGet.Scheme = corev1.URISchemeHTTP

	return livenessProbe, readinessProbe
}

// GetDeploymentType returns identifier for this strategy
func (u *UwsgiStrategy) GetDeploymentType() string {
	return "uwsgi"
}

// GetVolumeMounts returns the volume mounts for uwsgi deployment
func (u *UwsgiStrategy) GetVolumeMounts(instance *neutronv1.NeutronAPI, _ []corev1.Volume) map[string][]corev1.VolumeMount {
	return map[string][]corev1.VolumeMount{
		"neutron-uwsgi":                  GetVolumeMounts("neutron-uwsgi", instance.Spec.ExtraMounts, NeutronAPIPropagation),
		"neutron-periodic-workers":       GetVolumeMounts("neutron-periodic-workers", instance.Spec.ExtraMounts, NeutronAPIPropagation),
		"neutron-ovn-maintenance-worker": GetVolumeMounts("neutron-ovn-maintenance-worker", instance.Spec.ExtraMounts, NeutronAPIPropagation),
		"neutron-rpc-server":             GetVolumeMounts("neutron-rpc-server", instance.Spec.ExtraMounts, NeutronAPIPropagation),
	}
}
