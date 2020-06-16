package ovncontroller

import (
	corev1 "k8s.io/api/core/v1"
)

// GetVolumes - Volumes used by pod
func GetVolumes(cmName string) []corev1.Volume {
	var scriptsVolumeDefaultMode int32 = 0755
	return []corev1.Volume{
		{
			Name: "host-run-netns",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/run/netns",
				},
			},
		},
		{
			Name: "run-openvswitch",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/run/openvswitch",
				},
			},
		},
		{
			Name: "etc-openvswitch",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/openvswitch",
				},
			},
		},
		{
			Name: "var-lib-openvswitch",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lib/openvswitch",
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
			Name: "host-run-ovn-kubernetes",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/run/ovn-kubernetes",
				},
			},
		},
		{
			Name: cmName + "-scripts",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: &scriptsVolumeDefaultMode,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cmName + "-scripts",
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
			Name:      "run-openvswitch",
			MountPath: "/run/openvswitch",
		},
		{
			Name:      "etc-openvswitch",
			MountPath: "/etc/openvswitch",
		},
		{
			Name:      "var-lib-openvswitch",
			MountPath: "/var/lib/openvswitch",
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
			Name:      cmName + "-scripts",
			ReadOnly:  true,
			MountPath: "/usr/local/sbin/",
		},
	}

}
