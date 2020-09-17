package common

import corev1 "k8s.io/api/core/v1"

// GetVolumes - Volumes used by pod
func GetVolumes(cmName string) []corev1.Volume {

	return []corev1.Volume{
		{
			Name: "etc-localtime",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/localtime",
				},
			},
		},
	}
}

// GetVolumeMounts -  VolumeMounts
func GetVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "etc-localtime",
			MountPath: "/etc/localtime",
			ReadOnly:  true,
		},
	}
}
