package neutronapi

import (
	"fmt"

	"github.com/openstack-k8s-operators/lib-common/modules/common/annotations"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	neutronv1beta1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DbSyncJob func
func DbSyncJob(
	cr *neutronv1beta1.NeutronAPI,
	labels map[string]string,
) (*batchv1.Job, error) {

	runAsUser := int64(0)
	initVolumeMounts := GetInitVolumeMounts()
	volumeMounts := GetAPIVolumeMounts()
	volumes := GetAPIVolumes(cr.Name)

	envVars := map[string]env.Setter{}
	envVars["KOLLA_CONFIG_FILE"] = env.SetValue(KollaConfigDbSync)
	envVars["KOLLA_CONFIG_STRATEGY"] = env.SetValue("COPY_ALWAYS")

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-db-sync",
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyOnFailure,
					ServiceAccountName: ServiceAccountName,
					Containers: []corev1.Container{
						{
							Name:  cr.Name + "-db-sync",
							Image: cr.Spec.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env:          env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts: volumeMounts,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

	// networks to attach to
	nwAnnotation, err := annotations.GetNADAnnotation(cr.Namespace, cr.Spec.NetworkAttachmentDefinitions)
	if err != nil {
		return nil, fmt.Errorf("failed create network annotation from %s: %w",
			cr.Spec.NetworkAttachmentDefinitions, err)
	}
	job.Spec.Template.Annotations = util.MergeStringMaps(job.Spec.Template.Annotations, nwAnnotation)

	initContainerDetails := InitContainer{
		ContainerImage:       cr.Spec.ContainerImage,
		DatabaseHost:         cr.Status.DatabaseHostname,
		DatabaseUser:         cr.Spec.DatabaseUser,
		Database:             Database,
		NeutronSecret:        cr.Spec.Secret,
		DBPasswordSelector:   cr.Spec.PasswordSelectors.Database,
		UserPasswordSelector: cr.Spec.PasswordSelectors.Service,
		VolumeMounts:         initVolumeMounts,
	}
	job.Spec.Template.Spec.InitContainers = GetInitContainer(initContainerDetails)
	return job, nil
}
