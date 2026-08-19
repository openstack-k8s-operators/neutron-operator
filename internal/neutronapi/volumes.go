package neutronapi

import (
	"github.com/openstack-k8s-operators/lib-common/modules/common/volume"
	"github.com/openstack-k8s-operators/lib-common/modules/storage"
	neutronv1beta1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

var configMode int32 = 0440

// GetVolumes -
// TODO: merge to GetVolumes when other controllers also switched to current config
//
//	mechanism.
func GetVolumes(name string, extraVol []neutronv1beta1.NeutronExtraVolMounts, svc []storage.PropagationType) []corev1.Volume {
	res := []corev1.Volume{
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &configMode,
					SecretName:  name + "-config",
				},
			},
		},
		{
			Name: "httpd-config",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &configMode,
					SecretName:  name + "-httpd-config",
				},
			},
		},
	}
	for _, exv := range extraVol {
		for _, vol := range exv.Propagate(svc) {
			for _, v := range vol.Volumes {
				volumeSource, _ := v.ToCoreVolumeSource()
				convertedVolume := corev1.Volume{
					Name:         v.Name,
					VolumeSource: *volumeSource,
				}
				res = append(res, convertedVolume)
			}
		}
	}
	return res

}

// GetVolumeMounts - Neutron API/db-sync VolumeMounts
func GetVolumeMounts(
	extraVol []neutronv1beta1.NeutronExtraVolMounts,
	svc []storage.PropagationType,
	policyOverwrite bool,
) []corev1.VolumeMount {
	res := []corev1.VolumeMount{
		{
			Name:      "config",
			MountPath: "/etc/neutron/neutron.conf.d/01-neutron.conf",
			SubPath:   "01-neutron.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config",
			MountPath: "/etc/neutron/neutron.conf.d/02-neutron-custom.conf",
			SubPath:   "02-neutron-custom.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config",
			MountPath: "/etc/my.cnf",
			SubPath:   "my.cnf",
			ReadOnly:  true,
		},
	}
	// policy.yaml is only present in the config Secret when the user
	// overwrites it via spec.defaultConfigOverwrite -- mounting it
	// unconditionally would break every NeutronAPI that doesn't set it.
	if policyOverwrite {
		res = append(res, corev1.VolumeMount{
			Name:      "config",
			MountPath: "/etc/neutron/policy.yaml",
			SubPath:   "policy.yaml",
			ReadOnly:  true,
		})
	}
	for _, exv := range extraVol {
		for _, vol := range exv.Propagate(svc) {
			res = append(res, vol.Mounts...)
		}
	}
	return res

}

// GetHttpdVolumeMount - Returns the VolumeMounts used by the httpd sidecar
func GetHttpdVolumeMount() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "httpd-config",
			MountPath: "/etc/httpd/conf/httpd.conf",
			SubPath:   "httpd.conf",
			ReadOnly:  true,
		},
		{
			Name:      "httpd-config",
			MountPath: "/etc/httpd/conf.d/10-neutron.conf",
			SubPath:   "10-neutron-httpd.conf",
			ReadOnly:  true,
		},
		{
			Name:      "httpd-config",
			MountPath: "/etc/httpd/conf.d/ssl.conf",
			SubPath:   "ssl.conf",
			ReadOnly:  true,
		},
		volume.WritableDirVolumeMount(volume.RunHttpdVolumeName, volume.RunHttpdMountPath),
	}
}
