/*
Copyright 2020 Red Hat

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	neutronv1beta1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
	common "github.com/openstack-k8s-operators/neutron-operator/pkg/common"
	neutronapi "github.com/openstack-k8s-operators/neutron-operator/pkg/neutronapi"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
)

// NeutronAPIReconciler reconciles a NeutronAPI object
type NeutronAPIReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

// GetClient -
func (r *NeutronAPIReconciler) GetClient() client.Client {
	return r.Client
}

// GetLogger -
func (r *NeutronAPIReconciler) GetLogger() logr.Logger {
	return r.Log
}

// GetScheme -
func (r *NeutronAPIReconciler) GetScheme() *runtime.Scheme {
	return r.Scheme
}

// +kubebuilder:rbac:groups=neutron.openstack.org,resources=neutronapis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=neutron.openstack.org,resources=neutronapis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;create;update;delete;
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;create;update;delete;
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;create;update;delete;
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;create;update;delete;

// Reconcile - neutron api
func (r *NeutronAPIReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("neutronapi", req.NamespacedName)

	// Fetch the NeutronAPI instance
	instance := &neutronv1beta1.NeutronAPI{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	envVars := make(map[string]util.EnvSetter)

	// check for required secrets
	hashes := []neutronv1beta1.Hash{}
	_, hash, err := common.GetSecret(r.Client, instance.Spec.NeutronSecret, instance.Namespace)
	if err != nil {
		return ctrl.Result{RequeueAfter: time.Second * 10}, err
	}
	envVars[instance.Spec.NeutronSecret] = util.EnvValue(hash)
	hashes = append(hashes, neutronv1beta1.Hash{Name: instance.Spec.NeutronSecret, Hash: hash})

	_, hash, err = common.GetSecret(r.Client, instance.Spec.NovaSecret, instance.Namespace)
	if err != nil {
		return ctrl.Result{RequeueAfter: time.Second * 10}, err
	}
	envVars[instance.Spec.NovaSecret] = util.EnvValue(hash)
	hashes = append(hashes, neutronv1beta1.Hash{Name: instance.Spec.NovaSecret, Hash: hash})

	// check for required ovn-connection configMap
	ovnConnection, hash, err := common.GetConfigMapAndHashWithName(r, instance.Spec.OVNConnectionConfigMap, instance.Namespace)
	if err != nil {
		return ctrl.Result{RequeueAfter: time.Second * 10}, err
	}
	envVars[instance.Spec.OVNConnectionConfigMap] = util.EnvValue(hash)
	hashes = append(hashes, neutronv1beta1.Hash{Name: instance.Spec.OVNConnectionConfigMap, Hash: hash})

	// Create/update configmaps from templates
	cmLabels := common.GetLabels(instance.Name, neutronapi.AppLabel)

	cms := []common.ConfigMap{
		// ScriptsConfigMap
		{
			Name:           fmt.Sprintf("%s-scripts", instance.Name),
			Namespace:      instance.Namespace,
			CMType:         common.CMTypeScripts,
			InstanceType:   instance.Kind,
			AdditionalData: map[string]string{},
			Labels:         cmLabels,
		},
		// ConfigMap
		{
			Name:           fmt.Sprintf("%s-config-data", instance.Name),
			Namespace:      instance.Namespace,
			CMType:         common.CMTypeConfig,
			InstanceType:   instance.Kind,
			AdditionalData: map[string]string{},
			Labels:         cmLabels,
			ConfigOptions:  ovnConnection.Data,
		},
		// CustomConfigMap
		{
			Name:      fmt.Sprintf("%s-config-data-custom", instance.Name),
			Namespace: instance.Namespace,
			CMType:    common.CMTypeCustom,
			Labels:    cmLabels,
		},
	}
	err = common.EnsureConfigMaps(r, instance, cms, &envVars)
	if err != nil {
		return ctrl.Result{}, nil
	}

	// Create neutron DB
	db := common.Database{
		DatabaseName:     neutronapi.Database,
		DatabaseHostname: instance.Spec.DatabaseHostname,
		Secret:           instance.Spec.NeutronSecret,
	}
	databaseObj, err := common.DatabaseObject(r, instance, db)
	if err != nil {
		return ctrl.Result{}, err
	}
	// set owner reference on databaseObj
	oref := metav1.NewControllerRef(instance, instance.GroupVersionKind())
	databaseObj.SetOwnerReferences([]metav1.OwnerReference{*oref})

	foundDatabase := &unstructured.Unstructured{}
	foundDatabase.SetGroupVersionKind(databaseObj.GroupVersionKind())
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: databaseObj.GetName(), Namespace: databaseObj.GetNamespace()}, foundDatabase)
	if err != nil && k8s_errors.IsNotFound(err) {
		err := r.Client.Create(context.TODO(), &databaseObj)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else if err != nil {
		return ctrl.Result{}, err
	} else {
		completed, _, err := unstructured.NestedBool(foundDatabase.UnstructuredContent(), "status", "completed")
		if !completed {
			r.Log.Info(fmt.Sprintf("Waiting on %s DB to be created...", neutronapi.Database))
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}
	}

	// run dbsync job
	job := neutronapi.DbSyncJob(instance, r.Scheme)
	dbSyncHash, err := util.ObjectHash(job)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error calculating DB sync hash: %v", err)
	}

	requeue := true
	if instance.Status.DbSyncHash != dbSyncHash {
		requeue, err = util.EnsureJob(job, r.Client, r.Log)
		r.Log.Info("Running DB sync")
		if err != nil {
			return ctrl.Result{}, err
		} else if requeue {
			r.Log.Info("Waiting on DB sync")
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}
	}
	// db sync completed... okay to store the hash to disable it
	if err := r.setDbSyncHash(instance, dbSyncHash); err != nil {
		return ctrl.Result{}, err
	}

	// delete the dbsync job
	requeue, err = util.DeleteJob(job, r.Kclient, r.Log)
	if err != nil {
		return ctrl.Result{}, err
	}

	// update Hashes in CR status
	err = common.UpdateStatusHash(r, instance, &instance.Status.Hashes, hashes)
	if err != nil {
		return ctrl.Result{RequeueAfter: time.Second * 10}, err
	}

	// neutron-api
	// Create or update the Deployment object
	op, err := r.deploymentCreateOrUpdate(instance, envVars)
	if err != nil {
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		r.Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", instance.Name, string(op)))
	}

	// neutron service
	selector := make(map[string]string)
	selector["app"] = neutronapi.AppLabel

	serviceInfo := common.ServiceDetails{
		Name:      instance.Name,
		Namespace: instance.Namespace,
		AppLabel:  neutronapi.AppLabel,
		Selector:  selector,
		Port:      9696,
	}

	service := &corev1.Service{}
	service.Name = serviceInfo.Name
	service.Namespace = serviceInfo.Namespace
	if err := controllerutil.SetControllerReference(instance, service, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	service, op, err = common.CreateOrUpdateService(r.Client, r.Log, service, &serviceInfo)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.Log.Info("Service successfully reconciled", "operation", op)

	// Create the route if none exists
	routeInfo := common.RouteDetails{
		Name:      instance.Name,
		Namespace: instance.Namespace,
		AppLabel:  neutronapi.AppLabel,
		Port:      "api",
	}
	route := common.Route(routeInfo)
	if err := controllerutil.SetControllerReference(instance, route, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	err = common.CreateOrUpdateRoute(r.Client, r.Log, route)
	if err != nil {
		return ctrl.Result{}, err
	}

	// update status with endpoint information
	r.Log.Info("Reconciling neutron KeystoneService")
	neutronKeystoneService := &keystonev1beta1.KeystoneService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(context.TODO(), r.Client, neutronKeystoneService, func() error {
		neutronKeystoneService.Spec.ServiceType = "network"
		neutronKeystoneService.Spec.ServiceName = "neutron"
		neutronKeystoneService.Spec.ServiceDescription = "Openstack Networking"
		neutronKeystoneService.Spec.Enabled = true
		neutronKeystoneService.Spec.Region = "regionOne"
		neutronKeystoneService.Spec.AdminURL = fmt.Sprintf("http://%s", route.Spec.Host)
		neutronKeystoneService.Spec.PublicURL = fmt.Sprintf("http://%s", route.Spec.Host)
		neutronKeystoneService.Spec.InternalURL = "http://neutronapi.openstack.svc:9696"

		return nil
	})

	if err != nil {
		return ctrl.Result{}, err
	}
	r.setAPIEndpoint(instance, neutronKeystoneService.Spec.PublicURL)

	return ctrl.Result{}, nil
}

// SetupWithManager -
func (r *NeutronAPIReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&neutronv1beta1.NeutronAPI{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}

func (r *NeutronAPIReconciler) setDbSyncHash(api *neutronv1beta1.NeutronAPI, hashStr string) error {

	if hashStr != api.Status.DbSyncHash {
		api.Status.DbSyncHash = hashStr
		if err := r.Client.Status().Update(context.TODO(), api); err != nil {
			return err
		}
	}
	return nil
}

func (r *NeutronAPIReconciler) setAPIEndpoint(instance *neutronv1beta1.NeutronAPI, endpoint string) error {

	if endpoint != instance.Status.APIEndpoint {
		instance.Status.APIEndpoint = endpoint
		if err := r.Client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}

func (r *NeutronAPIReconciler) deploymentCreateOrUpdate(instance *neutronv1beta1.NeutronAPI, envVars map[string]util.EnvSetter) (controllerutil.OperationResult, error) {
	// set KOLLA_CONFIG env vars
	envVars["KOLLA_CONFIG_FILE"] = util.EnvValue(neutronapi.KollaConfig)
	envVars["KOLLA_CONFIG_STRATEGY"] = util.EnvValue("COPY_ALWAYS")

	// TODO:
	// get readinessProbes
	//readinessProbe := util.Probe{ProbeType: "readiness"}
	//livenessProbe := util.Probe{ProbeType: "liveness"}

	// get volumes
	initVolumeMounts := neutronapi.GetInitVolumeMounts()
	volumeMounts := neutronapi.GetAPIVolumeMounts()
	volumes := neutronapi.GetAPIVolumes(instance.Name)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.Client, deployment, func() error {

		// Daemonset selector is immutable so we set this value only if
		// a new object is going to be created
		if deployment.ObjectMeta.CreationTimestamp.IsZero() {
			deployment.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: common.GetLabels(instance.Name, neutronapi.AppLabel),
			}
		}

		if len(deployment.Spec.Template.Spec.Containers) != 1 {
			deployment.Spec.Template.Spec.Containers = make([]corev1.Container, 1)
		}
		envs := util.MergeEnvs(deployment.Spec.Template.Spec.Containers[0].Env, envVars)

		// labels
		common.InitLabelMap(&deployment.Spec.Template.Labels)
		for k, v := range common.GetLabels(instance.Name, neutronapi.AppLabel) {
			deployment.Spec.Template.Labels[k] = v
		}

		deployment.Spec.Replicas = &instance.Spec.Replicas
		deployment.Spec.Template.Spec = corev1.PodSpec{
			ServiceAccountName: neutronapi.ServiceAccountName,
			Volumes:            volumes,
			Containers: []corev1.Container{
				{
					Name:  "neutron-api",
					Image: instance.Spec.ContainerImage,
					// TODO - tripleo healthcheck script expects vhost config at /etc/httpd/conf.d/10-nova_api_wsgi.conf
					//ReadinessProbe: readinessProbe.GetProbe(),
					//LivenessProbe:  livenessProbe.GetProbe(),
					Env:          envs,
					VolumeMounts: volumeMounts,
				},
			},
		}

		initContainerDetails := common.InitContainer{
			ContainerImage: instance.Spec.ContainerImage,
			DatabaseHost:   instance.Spec.DatabaseHostname,
			NeutronSecret:  instance.Spec.NeutronSecret,
			NovaSecret:     instance.Spec.NovaSecret,
			VolumeMounts:   initVolumeMounts,
		}
		deployment.Spec.Template.Spec.InitContainers = common.GetInitContainer(initContainerDetails)

		err := controllerutil.SetControllerReference(instance, deployment, r.Scheme)
		if err != nil {
			return err
		}

		return nil
	})

	return op, err
}
