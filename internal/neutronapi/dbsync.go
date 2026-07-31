package neutronapi

import (
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/pod"
	"github.com/openstack-k8s-operators/lib-common/modules/users"
	neutronv1beta1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// DbSyncCommand - direct neutron-db-manage command without kolla wrapper
const DbSyncCommand = "neutron-db-manage --config-file /usr/share/neutron/neutron-dist.conf " +
	"--config-file /etc/neutron/neutron.conf --config-dir /etc/neutron/neutron.conf.d upgrade heads"

// DbSyncJob func
func DbSyncJob(
	cr *neutronv1beta1.NeutronAPI,
	labels map[string]string,
	annotations map[string]string,
) *batchv1.Job {
	dbSyncExtraMounts := cr.Spec.ExtraMounts

	volumes := GetVolumes(cr.Name, dbSyncExtraMounts, DbsyncPropagation)
	volumeMounts := GetVolumeMounts(dbSyncExtraMounts, DbsyncPropagation, false)

	// add CA cert if defined
	if cr.Spec.TLS.CaBundleSecretName != "" {
		volumes = append(volumes, cr.Spec.TLS.CreateVolume())
		volumeMounts = append(volumeMounts, cr.Spec.TLS.CreateVolumeMounts(nil)...)
	}

	envVars := map[string]env.Setter{}
	args := []string{"-c", DbSyncCommand}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        cr.Name + "-db-sync",
			Namespace:   cr.Namespace,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:                corev1.RestartPolicyOnFailure,
					ServiceAccountName:           cr.RbacResourceName(),
					AutomountServiceAccountToken: ptr.To(false),
					SecurityContext:              pod.RestrictivePodSecurityContext(users.NeutronUID, users.NeutronGID),
					Containers: []corev1.Container{
						{
							Name:            cr.Name + "-db-sync",
							Command:         []string{"/bin/bash"},
							Args:            args,
							Image:           cr.Spec.ContainerImage,
							SecurityContext: pod.RestrictiveSecurityContext(users.NeutronUID, users.NeutronGID),
							Env:             env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts:    volumeMounts,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

	if cr.Spec.NodeSelector != nil {
		job.Spec.Template.Spec.NodeSelector = *cr.Spec.NodeSelector
	}

	return job
}
