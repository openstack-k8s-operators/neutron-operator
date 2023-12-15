package neutronapi

import (
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
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

	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")

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
							Name:            cr.Name + "-db-sync",
							Image:           cr.Spec.ContainerImage,
							SecurityContext: getNeutronSecurityContext(),
							Env:             env.MergeEnvs([]corev1.EnvVar{}, envVars),
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
