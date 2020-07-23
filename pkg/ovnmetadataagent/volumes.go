package ovnmetadataagent

import (
corev1 "k8s.io/api/core/v1"
)

// GetVolumes - Volumes used by pod
func GetVolumes(cmName string) []corev1.Volume {
	var config0640AccessMode int32 = 0640
	return []corev1.Volume{
		{
			Name: "host-modules",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/lib/modules",
				},
			},
		},
		{
			Name: "host-run-netns",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/run/netns",
				},
			},
		},
		{
			Name: "var-lib-openvswitch",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lib/openvswitch/data",
				},
			},
		},
		{
			Name: "etc-openvswitch",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: " /var/lib/openvswitch/etc",
				},
			},
		},
		{
			Name: "run-openvswitch",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/run/openvswitch",
				},
			},
		},
		{
			Name: "run-ovn",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/run/ovn",
				},
			},
		},
		{
			Name: "lib-neutron",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lib/neutron",
				},
			},
		},
		{
			Name: "log-neutron",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/log/neutron",
				},
			},
		},
		{
			Name: cmName + "-templates",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: &config0640AccessMode,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cmName + "-templates",
					},
				},
			},
		},
	}

}

// GetVolumeMounts -  VolumeMounts
func GetVolumeMounts(cmName string) []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "host-modules",
			MountPath: "/lib/modules",
		},
		{
			Name:      "host-run-netns",
			MountPath: "/run/netns",
		},
		{
			Name:      "run-openvswitch",
			MountPath: "/run/openvswitch",
		},
		{
			Name:      "run-ovn",
			MountPath: "/run/ovn/",
		},
		{
			Name:      "etc-openvswitch",
			MountPath: "/etc/ovn/",
		},
		{
			Name:      "var-lib-openvswitch",
			MountPath: "/var/lib/openvswitch",
		},
		{
			Name:      "lib-neutron",
			MountPath: "/var/lib/neutron",
		},
		{
			Name:      "log-neutron",
			MountPath: "/var/log/neutron",
		},
		{
			Name:      "neutron-conf",
			MountPath: "/etc/neutron",
		},
	}

}
