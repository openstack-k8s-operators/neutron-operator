package neutronapi

import (
	common "github.com/openstack-k8s-operators/lib-common/pkg/common"
	neutronv1beta1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DbSyncJob func
func DbSyncJob(
	cr *neutronv1beta1.NeutronAPI,
	labels map[string]string,
) *batchv1.Job {

	runAsUser := int64(0)
	initVolumeMounts := GetInitVolumeMounts()
	volumeMounts := GetAPIVolumeMounts()
	volumes := GetAPIVolumes(cr.Name)
	cmLabels := common.GetLabels(cr, common.GetGroupLabel(ServiceName), map[string]string{})

	envVars := map[string]common.EnvSetter{}
	envVars["KOLLA_CONFIG_FILE"] = common.EnvValue(KollaConfigDbSync)
	envVars["KOLLA_CONFIG_STRATEGY"] = common.EnvValue("COPY_ALWAYS")

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-db-sync",
			Namespace: cr.Namespace,
			Labels:    cmLabels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      "OnFailure",
					ServiceAccountName: ServiceAccountName,
					Containers: []corev1.Container{
						{
							Name:  cr.Name + "-db-sync",
							Image: cr.Spec.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env:          common.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts: volumeMounts,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}
	initContainerDetails := InitContainer{
		ContainerImage:       cr.Spec.ContainerImage,
		DatabaseHost:         cr.Status.DatabaseHostname,
		Database:             Database,
		NeutronSecret:        cr.Spec.Secret,
		DBPasswordSelector:   cr.Spec.PasswordSelectors.Database,
		UserPasswordSelector: cr.Spec.PasswordSelectors.Service,
		NovaPasswordSelector: cr.Spec.PasswordSelectors.NovaService,
		VolumeMounts:         initVolumeMounts,
	}
	job.Spec.Template.Spec.InitContainers = GetInitContainer(initContainerDetails)
	return job
}
