package neutronapi

import (
	"github.com/openstack-k8s-operators/lib-common/modules/storage"
	neutronv1beta1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

// GetVolumes -
// TODO: merge to GetVolumes when other controllers also switched to current config
//
//	mechanism.
func GetVolumes(name string, extraVol []neutronv1beta1.NeutronExtraVolMounts, svc []storage.PropagationType) []corev1.Volume {
	var config0644AccessMode int32 = 0644

	res := []corev1.Volume{
		{
			Name: "config-data",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: &config0644AccessMode,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name + "-config-data",
					},
				},
			},
		},
	}
	for _, exv := range extraVol {
		for _, vol := range exv.Propagate(svc) {
			res = append(res, vol.Volumes...)
		}
	}
	return res

}

// GetVolumeMounts - Neutron API VolumeMounts
func GetVolumeMounts(serviceName string, extraVol []neutronv1beta1.NeutronExtraVolMounts, svc []storage.PropagationType) []corev1.VolumeMount {
	res := []corev1.VolumeMount{
		{
			Name:      "config-data",
			MountPath: "/etc/neutron.conf.d",
			ReadOnly:  true,
		},
	}
	for _, exv := range extraVol {
		for _, vol := range exv.Propagate(svc) {
			res = append(res, vol.Mounts...)
		}
	}
	return res

}
