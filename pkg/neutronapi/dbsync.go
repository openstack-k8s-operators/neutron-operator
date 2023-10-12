package neutronapi

import (
	neutronv1beta1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DbSyncJob func
func DbSyncJob(
	cr *neutronv1beta1.NeutronAPI,
	labels map[string]string,
	annotations map[string]string,
) *batchv1.Job {
	dbSyncExtraMounts := []neutronv1beta1.NeutronExtraVolMounts{}
	volumeMounts := GetVolumeMounts("db-sync", dbSyncExtraMounts, DbsyncPropagation)
	volumes := GetVolumes(cr.Name, dbSyncExtraMounts, DbsyncPropagation)

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
					RestartPolicy:      corev1.RestartPolicyOnFailure,
					ServiceAccountName: cr.RbacResourceName(),
					Containers: []corev1.Container{
						{
							Command:         []string{"neutron-db-manage"},
							Args:            []string{"upgrade", "heads"},
							Name:            cr.Name + "-db-sync",
							Image:           cr.Spec.ContainerImage,
							SecurityContext: getNeutronSecurityContext(),
							VolumeMounts:    volumeMounts,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}
	return job
}
