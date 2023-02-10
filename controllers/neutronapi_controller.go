/*
Copyright 2022 Red Hat

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
	routev1 "github.com/openshift/api/route/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
	"github.com/openstack-k8s-operators/lib-common/modules/common/deployment"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/job"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	"github.com/openstack-k8s-operators/lib-common/modules/database"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	neutronv1beta1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/neutron-operator/pkg/neutronapi"
	ovnclient "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
// +kubebuilder:rbac:groups=neutron.openstack.org,resources=neutronapis/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;patch;update;delete;
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;patch;update;delete;
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;patch;update;delete;
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list;watch;
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=ovn.openstack.org,resources=ovndbclusters,verbs=get;list;watch;
// +kubebuilder:rbac:groups=rabbitmq.openstack.org,resources=transporturls,verbs=get;list;watch;create;update;patch;delete

// Reconcile - neutron api
func (r *NeutronAPIReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	_ = context.Background()
	_ = r.Log.WithValues("neutronapi", req.NamespacedName)

	// Fetch the NeutronAPI instance
	instance := &neutronv1beta1.NeutronAPI{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
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

	helper, err := helper.NewHelper(
		instance,
		r.Client,
		r.Kclient,
		r.Scheme,
		r.Log,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Always patch the instance status when exiting this function so we can persist any changes.
	defer func() {
		// update the overall status condition if service is ready
		if instance.IsReady() {
			instance.Status.Conditions.MarkTrue(condition.ReadyCondition, condition.ReadyMessage)
		}

		err := helper.PatchInstance(ctx, instance)
		if err != nil {
			_err = err
			return
		}
	}()

	// If we're not deleting this and the service object doesn't have our finalizer, add it.
	if instance.DeletionTimestamp.IsZero() && controllerutil.AddFinalizer(instance, helper.GetFinalizer()) {
		return ctrl.Result{}, nil
	}

	//
	// initialize status
	//
	if instance.Status.Conditions == nil {
		instance.Status.Conditions = condition.Conditions{}
		// initialize conditions used later as Status=Unknown
		cl := condition.CreateList(
			condition.UnknownCondition(condition.DBReadyCondition, condition.InitReason, condition.DBReadyInitMessage),
			condition.UnknownCondition(condition.DBSyncReadyCondition, condition.InitReason, condition.DBSyncReadyInitMessage),
			condition.UnknownCondition(condition.ExposeServiceReadyCondition, condition.InitReason, condition.ExposeServiceReadyInitMessage),
			condition.UnknownCondition(neutronv1beta1.NeutronRabbitMqTransportURLReadyCondition, condition.InitReason, neutronv1beta1.NeutronRabbitMqTransportURLReadyInitMessage),
			condition.UnknownCondition(condition.InputReadyCondition, condition.InitReason, condition.InputReadyInitMessage),
			condition.UnknownCondition(condition.ServiceConfigReadyCondition, condition.InitReason, condition.ServiceConfigReadyInitMessage),
			condition.UnknownCondition(condition.DeploymentReadyCondition, condition.InitReason, condition.DeploymentReadyInitMessage),
			// right now we have no dedicated KeystoneServiceReadyInitMessage
			condition.UnknownCondition(condition.KeystoneServiceReadyCondition, condition.InitReason, ""),
		)

		instance.Status.Conditions.Init(&cl)

		// Register overall status immediately to have an early feedback e.g. in the cli
		return ctrl.Result{}, nil
	}
	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}
	if instance.Status.APIEndpoints == nil {
		instance.Status.APIEndpoints = map[string]string{}
	}

	// Handle service delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helper)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, instance, helper)
}

// SetupWithManager -
func (r *NeutronAPIReconciler) SetupWithManager(mgr ctrl.Manager) error {
	crs := &neutronv1beta1.NeutronAPIList{}
	return ctrl.NewControllerManagedBy(mgr).
		For(&neutronv1beta1.NeutronAPI{}).
		Owns(&mariadbv1.MariaDBDatabase{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.Deployment{}).
		Owns(&routev1.Route{}).
		Owns(&keystonev1.KeystoneService{}).
		Owns(&keystonev1.KeystoneEndpoint{}).
		Owns(&rabbitmqv1.TransportURL{}).
		Watches(&source.Kind{Type: &ovnclient.OVNDBCluster{}}, handler.EnqueueRequestsFromMapFunc(ovnclient.OVNDBClusterNamespaceMapFunc(crs, mgr.GetClient(), r.Log))).
		Complete(r)
}

func (r *NeutronAPIReconciler) reconcileDelete(ctx context.Context, instance *neutronv1beta1.NeutronAPI, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service delete")

	// remove db finalizer first
	db, err := database.GetDatabaseByName(ctx, helper, instance.Name)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	if !k8s_errors.IsNotFound(err) {
		if err := db.DeleteFinalizer(ctx, helper); err != nil {
			return ctrl.Result{}, err
		}
		util.LogForObject(helper, "Removed finalizer from our MariaDBDatabase", instance)
	}

	// Remove the finalizer from our KeystoneEndpoint CR
	keystoneEndpoint, err := keystonev1.GetKeystoneEndpointWithName(ctx, helper, neutronapi.ServiceName, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	if err == nil {
		controllerutil.RemoveFinalizer(keystoneEndpoint, helper.GetFinalizer())
		if err = helper.GetClient().Update(ctx, keystoneEndpoint); err != nil && !k8s_errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		util.LogForObject(helper, "Removed finalizer from our KeystoneEndpoint", instance)
	}

	// Remove the finalizer from our KeystoneService CR
	keystoneService, err := keystonev1.GetKeystoneServiceWithName(ctx, helper, neutronapi.ServiceName, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	if err == nil {
		controllerutil.RemoveFinalizer(keystoneService, helper.GetFinalizer())
		if err = helper.GetClient().Update(ctx, keystoneService); err != nil && !k8s_errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		util.LogForObject(helper, "Removed finalizer from our KeystoneService", instance)
	}

	// Service is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	r.Log.Info("Reconciled Service delete successfully")

	return ctrl.Result{}, nil
}

func (r *NeutronAPIReconciler) reconcileInit(
	ctx context.Context,
	instance *neutronv1beta1.NeutronAPI,
	helper *helper.Helper,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service init")

	// create neutron DB instance
	//
	db := database.NewDatabase(
		instance.Name,
		instance.Spec.DatabaseUser,
		instance.Spec.Secret,
		map[string]string{
			"dbName": instance.Spec.DatabaseInstance,
		},
	)
	// create or patch the DB
	ctrlResult, err := db.CreateOrPatchDB(
		ctx,
		helper,
	)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBReadyRunningMessage))
		return ctrlResult, nil
	}
	// wait for the DB to be setup
	ctrlResult, err = db.WaitForDBCreated(ctx, helper)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBReadyErrorMessage,
			err.Error()))
		return ctrlResult, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBReadyRunningMessage))
		return ctrlResult, nil
	}
	// update Status.DatabaseHostname, used to bootstrap/config the service
	instance.Status.DatabaseHostname = db.GetDatabaseHostname()
	instance.Status.Conditions.MarkTrue(condition.DBReadyCondition, condition.DBReadyMessage)
	// create neutron DB - end

	//
	// run Neutron db sync
	//

	dbSyncHash := instance.Status.Hash[neutronv1beta1.DbSyncHash]
	jobDef, err := neutronapi.DbSyncJob(instance, serviceLabels)
	if err != nil {
		return ctrl.Result{}, err
	}
	dbSyncjob := job.NewJob(
		jobDef,
		neutronv1beta1.DbSyncHash,
		instance.Spec.PreserveJobs,
		time.Duration(5)*time.Second,
		dbSyncHash,
	)
	ctrlResult, err = dbSyncjob.DoJob(
		ctx,
		helper,
	)
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBSyncReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBSyncReadyRunningMessage))
		return ctrlResult, nil
	}
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBSyncReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBSyncReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	if dbSyncjob.HasChanged() {
		instance.Status.Hash[neutronv1beta1.DbSyncHash] = dbSyncjob.GetHash()
		r.Log.Info(fmt.Sprintf("Job %s hash added - %s", jobDef.Name, instance.Status.Hash[neutronv1beta1.DbSyncHash]))
	}
	instance.Status.Conditions.MarkTrue(condition.DBSyncReadyCondition, condition.DBSyncReadyMessage)

	// run Neutron db sync - end

	//
	// expose the service (create service, route and return the created endpoint URLs)
	//
	var keystonePorts = map[endpoint.Endpoint]endpoint.Data{
		endpoint.EndpointAdmin: {
			Port: neutronapi.NeutronAdminPort,
		},
		endpoint.EndpointPublic: {
			Port: neutronapi.NeutronPublicPort,
		},
		endpoint.EndpointInternal: {
			Port: neutronapi.NeutronInternalPort,
		},
	}

	apiEndpoints, ctrlResult, err := endpoint.ExposeEndpoints(
		ctx,
		helper,
		neutronapi.ServiceName,
		serviceLabels,
		keystonePorts,
		time.Duration(5)*time.Second,
	)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ExposeServiceReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ExposeServiceReadyErrorMessage,
			err.Error()))
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ExposeServiceReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.ExposeServiceReadyRunningMessage))
		return ctrlResult, nil
	}
	instance.Status.Conditions.MarkTrue(condition.ExposeServiceReadyCondition, condition.ExposeServiceReadyMessage)

	//
	// Update instance status with service endpoint url from route host information
	//
	// TODO: need to support https default here
	if instance.Status.APIEndpoints == nil {
		instance.Status.APIEndpoints = map[string]string{}
	}
	instance.Status.APIEndpoints = apiEndpoints

	// expose service - end

	// update status with endpoint information
	r.Log.Info("Reconciling neutron KeystoneService")

	ksSvcSpec := keystonev1.KeystoneServiceSpec{
		ServiceType:        neutronapi.ServiceType,
		ServiceName:        neutronapi.ServiceName,
		ServiceDescription: "Openstack Networking",
		Enabled:            true,
		ServiceUser:        instance.Spec.ServiceUser,
		Secret:             instance.Spec.Secret,
		PasswordSelector:   instance.Spec.PasswordSelectors.Service,
	}
	ksSvc := keystonev1.NewKeystoneService(ksSvcSpec, instance.Namespace, serviceLabels, time.Duration(10)*time.Second)
	ctrlResult, err = ksSvc.CreateOrPatch(ctx, helper)

	if err != nil {
		return ctrlResult, err
	}

	// mirror the Status, Reason, Severity and Message of the latest keystoneservice condition
	// into a local condition with the type condition.KeystoneServiceReadyCondition
	c := ksSvc.GetConditions().Mirror(condition.KeystoneServiceReadyCondition)
	if c != nil {
		instance.Status.Conditions.Set(c)
	}

	if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	instance.Status.ServiceID = ksSvc.GetServiceID()

	//
	// register endpoints
	//
	ksEndptSpec := keystonev1.KeystoneEndpointSpec{
		ServiceName: neutronapi.ServiceName,
		Endpoints:   instance.Status.APIEndpoints,
	}
	ksEndpt := keystonev1.NewKeystoneEndpoint(
		neutronapi.ServiceName,
		instance.Namespace,
		ksEndptSpec,
		serviceLabels,
		time.Duration(10)*time.Second)
	ctrlResult, err = ksEndpt.CreateOrPatch(ctx, helper)
	if err != nil {
		return ctrlResult, err
	}
	// mirror the Status, Reason, Severity and Message of the latest keystoneendpoint condition
	// into a local condition with the type condition.KeystoneEndpointReadyCondition
	c = ksEndpt.GetConditions().Mirror(condition.KeystoneEndpointReadyCondition)
	if c != nil {
		instance.Status.Conditions.Set(c)
	}

	if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	r.Log.Info("Reconciled Service init successfully")
	return ctrl.Result{}, nil
}

func (r *NeutronAPIReconciler) reconcileUpdate(ctx context.Context, instance *neutronv1beta1.NeutronAPI, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service update")

	// TODO: should have minor update tasks if required
	// - delete dbsync hash from status to rerun it?

	r.Log.Info("Reconciled Service update successfully")
	return ctrl.Result{}, nil
}

func (r *NeutronAPIReconciler) reconcileUpgrade(ctx context.Context, instance *neutronv1beta1.NeutronAPI, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service upgrade")

	// TODO: should have major version upgrade tasks
	// -delete dbsync hash from status to rerun it?

	r.Log.Info("Reconciled Service upgrade successfully")
	return ctrl.Result{}, nil
}

func (r *NeutronAPIReconciler) reconcileNormal(ctx context.Context, instance *neutronv1beta1.NeutronAPI, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service")

	// ConfigMap
	configMapVars := make(map[string]env.Setter)

	//
	// create RabbitMQ transportURL CR and get the actual URL from the associated secret that is created
	//

	transportURL, op, err := r.transportURLCreateOrUpdate(instance)

	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			neutronv1beta1.NeutronRabbitMqTransportURLReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			neutronv1beta1.NeutronRabbitMqTransportURLReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	if op != controllerutil.OperationResultNone {
		r.Log.Info(fmt.Sprintf("TransportURL %s successfully reconciled - operation: %s", transportURL.Name, string(op)))
	}

	instance.Status.TransportURLSecret = transportURL.Status.SecretName

	if instance.Status.TransportURLSecret == "" {
		r.Log.Info(fmt.Sprintf("Waiting for TransportURL %s secret to be created", transportURL.Name))

		instance.Status.Conditions.Set(condition.FalseCondition(
			neutronv1beta1.NeutronRabbitMqTransportURLReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			neutronv1beta1.NeutronRabbitMqTransportURLReadyRunningMessage))

		return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, nil
	}

	//
	// check for required TransportURL secret holding transport URL string
	//
	transportURLSecret, hash, err := secret.GetSecret(ctx, helper, instance.Status.TransportURLSecret, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.InputReadyWaitingMessage))
			return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, fmt.Errorf("TransportURL secret %s not found", instance.Status.TransportURLSecret)
		}
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	configMapVars[transportURLSecret.Name] = env.SetValue(hash)
	// run check TransportURL secret - end

	instance.Status.Conditions.MarkTrue(neutronv1beta1.NeutronRabbitMqTransportURLReadyCondition, neutronv1beta1.NeutronRabbitMqTransportURLReadyMessage)

	// end transportURL

	//
	// check for required OpenStack secret holding passwords for service/admin user and add hash to the vars map,
	//
	ospSecret, hash, err := secret.GetSecret(ctx, helper, instance.Spec.Secret, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.InputReadyWaitingMessage))
			return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, fmt.Errorf("OpenStack secret %s not found", instance.Spec.Secret)
		}
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	configMapVars[ospSecret.Name] = env.SetValue(hash)

	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)
	// run check OpenStack secret - end

	//
	// Create ConfigMaps and Secrets required as input for the Service and calculate an overall hash of hashes
	//

	//
	// create Configmap required for neutron input
	// - %-scripts configmap holding scripts to e.g. bootstrap the service
	// - %-config configmap holding minimal neutron config required to get the service up, user can add additional files to be added to the service
	// - parameters which has passwords gets added from the OpenStack secret via the init container
	//
	err = r.generateServiceConfigMaps(ctx, helper, instance, &configMapVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	//
	// create hash over all the different input resources to identify if any those changed
	// and a restart/recreate is required.
	//
	inputHash, hashChanged, err := r.createHashOfInputHashes(ctx, instance, configMapVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	} else if hashChanged {
		// Hash changed and instance status should be updated (which will be done by main defer func),
		// so we need to return and reconcile again
		return ctrl.Result{}, nil
	}
	// Create ConfigMaps and Secrets - end

	instance.Status.Conditions.MarkTrue(condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage)

	//
	// TODO check when/if Init, Update, or Upgrade should/could be skipped
	//

	serviceLabels := map[string]string{
		common.AppSelector: neutronapi.ServiceName,
	}

	// Handle service init
	ctrlResult, err := r.reconcileInit(ctx, instance, helper, serviceLabels)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service update
	ctrlResult, err = r.reconcileUpdate(ctx, instance, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service upgrade
	ctrlResult, err = r.reconcileUpgrade(ctx, instance, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Define a new Deployment object
	deplDef, err := neutronapi.Deployment(instance, inputHash, serviceLabels)
	if err != nil {
		return ctrl.Result{}, err
	}
	depl := deployment.NewDeployment(
		deplDef,
		time.Duration(5)*time.Second,
	)

	ctrlResult, err = depl.CreateOrPatch(ctx, helper)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DeploymentReadyErrorMessage,
			err.Error()))
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DeploymentReadyRunningMessage))
		return ctrlResult, nil
	}

	instance.Status.ReadyCount = depl.GetDeployment().Status.ReadyReplicas
	instance.Status.Networks = instance.Spec.NetworkAttachmentDefinitions

	if instance.Status.ReadyCount > 0 {
		instance.Status.Conditions.MarkTrue(condition.DeploymentReadyCondition, condition.DeploymentReadyMessage)
	}
	// create Deployment - end

	r.Log.Info("Reconciled Service successfully")
	return ctrl.Result{}, nil
}

func (r *NeutronAPIReconciler) transportURLCreateOrUpdate(instance *neutronv1beta1.NeutronAPI) (*rabbitmqv1.TransportURL, controllerutil.OperationResult, error) {
	transportURL := &rabbitmqv1.TransportURL{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-neutron-transport", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.Client, transportURL, func() error {
		transportURL.Spec.RabbitmqClusterName = instance.Spec.RabbitMqClusterName

		err := controllerutil.SetControllerReference(instance, transportURL, r.Scheme)
		return err
	})

	return transportURL, op, err
}

// generateServiceConfigMaps - create create configmaps which hold scripts and service configuration
// TODO add DefaultConfigOverwrite
func (r *NeutronAPIReconciler) generateServiceConfigMaps(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	envVars *map[string]env.Setter,
) error {
	// Create/update configmaps from templates
	cmLabels := labels.GetLabels(instance, labels.GetGroupLabel(neutronapi.ServiceName), map[string]string{})

	dbmap, err := ovnclient.GetDBEndpoints(ctx, h, instance.Namespace, map[string]string{})
	if err != nil {
		return err
	}

	// customData hold any customization for the service.
	// custom.conf is going to /etc/<service>/<service>.conf.d
	// all other files get placed into /etc/<service> to allow overwrite of e.g. logging.conf or policy.json
	// TODO: make sure custom.conf can not be overwritten
	customData := map[string]string{common.CustomServiceConfigFileName: instance.Spec.CustomServiceConfig}
	for key, data := range instance.Spec.DefaultConfigOverwrite {
		customData[key] = data
	}

	keystoneAPI, err := keystonev1.GetKeystoneAPI(ctx, h, instance.Namespace, map[string]string{})
	if err != nil {
		return err
	}
	keystoneInternalURL, err := keystoneAPI.GetEndpoint(endpoint.EndpointInternal)
	if err != nil {
		return err
	}
	keystonePublicURL, err := keystoneAPI.GetEndpoint(endpoint.EndpointPublic)
	if err != nil {
		return err
	}
	templateParameters := make(map[string]interface{})
	templateParameters["ServiceUser"] = instance.Spec.ServiceUser
	templateParameters["KeystoneInternalURL"] = keystoneInternalURL
	templateParameters["KeystonePublicURL"] = keystonePublicURL

	templateParameters["NBConnection"] = dbmap["NB"]
	templateParameters["SBConnection"] = dbmap["SB"]

	cms := []util.Template{
		// ScriptsConfigMap
		{
			Name:         fmt.Sprintf("%s-scripts", instance.Name),
			Namespace:    instance.Namespace,
			Type:         util.TemplateTypeScripts,
			InstanceType: instance.Kind,
			Labels:       cmLabels,
		},
		// ConfigMap
		{
			Name:          fmt.Sprintf("%s-config-data", instance.Name),
			Namespace:     instance.Namespace,
			Type:          util.TemplateTypeConfig,
			InstanceType:  instance.Kind,
			CustomData:    customData,
			Labels:        cmLabels,
			ConfigOptions: templateParameters,
		},
	}
	return configmap.EnsureConfigMaps(ctx, h, instance, cms, envVars)
}

// createHashOfInputHashes - creates a hash of hashes which gets added to the resources which requires a restart
// if any of the input resources change, like configs, passwords, ...
//
// returns the hash, whether the hash changed (as a bool) and any error
func (r *NeutronAPIReconciler) createHashOfInputHashes(
	ctx context.Context,
	instance *neutronv1beta1.NeutronAPI,
	envVars map[string]env.Setter,
) (string, bool, error) {
	var hashMap map[string]string
	changed := false
	mergedMapVars := env.MergeEnvs([]corev1.EnvVar{}, envVars)
	hash, err := util.ObjectHash(mergedMapVars)
	if err != nil {
		return hash, changed, err
	}
	if hashMap, changed = util.SetHash(instance.Status.Hash, common.InputHashName, hash); changed {
		instance.Status.Hash = hashMap
		r.Log.Info(fmt.Sprintf("Input maps hash %s - %s", common.InputHashName, hash))
	}
	return hash, changed, nil
}
