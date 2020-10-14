package neutronapi

import (
	neutronv1beta1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
	common "github.com/openstack-k8s-operators/neutron-operator/pkg/common"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// DbSyncJob func
func DbSyncJob(cr *neutronv1beta1.NeutronAPI, scheme *runtime.Scheme) *batchv1.Job {

	runAsUser := int64(0)
	initVolumeMounts := GetInitVolumeMounts()
	volumeMounts := GetAPIVolumeMounts()
	volumes := GetAPIVolumes(cr.Name)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-db-sync",
			Namespace: cr.Namespace,
			Labels:    common.GetLabels(cr.Name, AppLabel),
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
							Env: []corev1.EnvVar{
								{
									Name:  "KOLLA_CONFIG_FILE",
									Value: "/var/lib/config-data/merged/db-sync-config.json",
								},
								{
									Name:  "KOLLA_CONFIG_STRATEGY",
									Value: "COPY_ALWAYS",
								},
								{
									Name:  "DatabaseHost",
									Value: cr.Spec.DatabaseHostname,
								},
								{
									Name:  "Database",
									Value: Database,
								},
								{
									Name: "DatabasePassword",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: cr.Spec.NeutronSecret,
											},
											Key: "DatabasePassword",
										},
									},
								},
							},
							VolumeMounts: volumeMounts,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}
	initContainerDetails := common.InitContainer{
		ContainerImage: cr.Spec.ContainerImage,
		DatabaseHost:   cr.Spec.DatabaseHostname,
		Database:       Database,
		NeutronSecret:  cr.Spec.NeutronSecret,
		NovaSecret:     cr.Spec.NovaSecret,
		VolumeMounts:   initVolumeMounts,
	}
	job.Spec.Template.Spec.InitContainers = common.GetInitContainer(initContainerDetails)
	controllerutil.SetControllerReference(cr, job, scheme)
	return job
}
