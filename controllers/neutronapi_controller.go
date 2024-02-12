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
	"strings"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/deployment"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/job"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	nad "github.com/openstack-k8s-operators/lib-common/modules/common/networkattachment"
	common_rbac "github.com/openstack-k8s-operators/lib-common/modules/common/rbac"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	neutronv1beta1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/neutron-operator/pkg/neutronapi"
	ovnclient "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// getlogger returns a logger object with a prefix of "conroller.name" and aditional controller context fields
func (r *NeutronAPIReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("NeutronAPI")
}

// NeutronAPIReconciler reconciles a NeutronAPI object
type NeutronAPIReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Scheme  *runtime.Scheme
}

// +kubebuilder:rbac:groups=neutron.openstack.org,resources=neutronapis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=neutron.openstack.org,resources=neutronapis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=neutron.openstack.org,resources=neutronapis/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;patch;update;delete;
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;patch;update;delete;
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;patch;update;delete;
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbaccounts/finalizers,verbs=update
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list;watch;
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=ovn.openstack.org,resources=ovndbclusters,verbs=get;list;watch;
// +kubebuilder:rbac:groups=rabbitmq.openstack.org,resources=transporturls,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=memcached.openstack.org,resources=memcacheds,verbs=get;list;watch;
// +kubebuilder:rbac:groups=k8s.cni.cncf.io,resources=network-attachment-definitions,verbs=get;list;watch

// service account, role, rolebinding
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update
// service account permissions that are needed to grant permission to the above
// +kubebuilder:rbac:groups="security.openshift.io",resourceNames=anyuid;hostmount-anyuid,resources=securitycontextconstraints,verbs=use
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;delete;get;list;patch;update;watch

// Reconcile - neutron api
func (r *NeutronAPIReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	_ = context.Background()
	Log := r.GetLogger(ctx)

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
		Log,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Always patch the instance status when exiting this function so we can persist any changes.
	defer func() {
		// update the Ready condition based on the sub conditions
		if instance.Status.Conditions.AllSubConditionIsTrue() {
			instance.Status.Conditions.MarkTrue(
				condition.ReadyCondition, condition.ReadyMessage)
		} else {
			// something is not ready so reset the Ready condition
			instance.Status.Conditions.MarkUnknown(
				condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage)
			// and recalculate it based on the state of the rest of the conditions
			instance.Status.Conditions.Set(
				instance.Status.Conditions.Mirror(condition.ReadyCondition))
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
			condition.UnknownCondition(condition.RabbitMqTransportURLReadyCondition, condition.InitReason, condition.RabbitMqTransportURLReadyInitMessage),
			condition.UnknownCondition(condition.MemcachedReadyCondition, condition.InitReason, condition.MemcachedReadyInitMessage),
			condition.UnknownCondition(condition.InputReadyCondition, condition.InitReason, condition.InputReadyInitMessage),
			condition.UnknownCondition(condition.ServiceConfigReadyCondition, condition.InitReason, condition.ServiceConfigReadyInitMessage),
			condition.UnknownCondition(condition.DeploymentReadyCondition, condition.InitReason, condition.DeploymentReadyInitMessage),
			// right now we have no dedicated KeystoneServiceReadyInitMessage and KeystoneEndpointReadyInitMessage
			condition.UnknownCondition(condition.KeystoneServiceReadyCondition, condition.InitReason, ""),
			condition.UnknownCondition(condition.KeystoneEndpointReadyCondition, condition.InitReason, ""),
			condition.UnknownCondition(condition.NetworkAttachmentsReadyCondition, condition.InitReason, condition.NetworkAttachmentsReadyInitMessage),
			condition.UnknownCondition(condition.ServiceAccountReadyCondition, condition.InitReason, condition.ServiceAccountReadyInitMessage),
			condition.UnknownCondition(condition.RoleReadyCondition, condition.InitReason, condition.RoleReadyInitMessage),
			condition.UnknownCondition(condition.RoleBindingReadyCondition, condition.InitReason, condition.RoleBindingReadyInitMessage),
		)

		instance.Status.Conditions.Init(&cl)

		// Register overall status immediately to have an early feedback e.g. in the cli
		return ctrl.Result{}, nil
	}
	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}
	if instance.Status.NetworkAttachments == nil {
		instance.Status.NetworkAttachments = map[string][]string{}
	}

	// Handle service delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helper)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, instance, helper)
}

// fields to index to reconcile when change
const (
	passwordSecretField     = ".spec.secret"
	caBundleSecretNameField = ".spec.tls.caBundleSecretName"
	tlsAPIInternalField     = ".spec.tls.api.internal.secretName"
	tlsAPIPublicField       = ".spec.tls.api.public.secretName"
)

var allWatchFields = []string{
	passwordSecretField,
	caBundleSecretNameField,
	tlsAPIInternalField,
	tlsAPIPublicField,
}

// SetupWithManager -
func (r *NeutronAPIReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	// index passwordSecretField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &neutronv1beta1.NeutronAPI{}, passwordSecretField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*neutronv1beta1.NeutronAPI)
		if cr.Spec.Secret == "" {
			return nil
		}
		return []string{cr.Spec.Secret}
	}); err != nil {
		return err
	}

	// index caBundleSecretNameField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &neutronv1beta1.NeutronAPI{}, caBundleSecretNameField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*neutronv1beta1.NeutronAPI)
		if cr.Spec.TLS.CaBundleSecretName == "" {
			return nil
		}
		return []string{cr.Spec.TLS.CaBundleSecretName}
	}); err != nil {
		return err
	}

	// index tlsAPIInternalField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &neutronv1beta1.NeutronAPI{}, tlsAPIInternalField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*neutronv1beta1.NeutronAPI)
		if cr.Spec.TLS.API.Internal.SecretName == nil {
			return nil
		}
		return []string{*cr.Spec.TLS.API.Internal.SecretName}
	}); err != nil {
		return err
	}

	// index tlsAPIPublicField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &neutronv1beta1.NeutronAPI{}, tlsAPIPublicField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*neutronv1beta1.NeutronAPI)
		if cr.Spec.TLS.API.Public.SecretName == nil {
			return nil
		}
		return []string{*cr.Spec.TLS.API.Public.SecretName}
	}); err != nil {
		return err
	}

	crs := &neutronv1beta1.NeutronAPIList{}
	return ctrl.NewControllerManagedBy(mgr).
		For(&neutronv1beta1.NeutronAPI{}).
		Owns(&mariadbv1.MariaDBDatabase{}).
		Owns(&mariadbv1.MariaDBAccount{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&appsv1.Deployment{}).
		Owns(&keystonev1.KeystoneService{}).
		Owns(&keystonev1.KeystoneEndpoint{}).
		Owns(&rabbitmqv1.TransportURL{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Watches(&source.Kind{Type: &ovnclient.OVNDBCluster{}}, handler.EnqueueRequestsFromMapFunc(ovnclient.OVNDBClusterNamespaceMapFunc(crs, mgr.GetClient(), r.GetLogger(ctx)))).
		Watches(&source.Kind{Type: &memcachedv1.Memcached{}}, handler.EnqueueRequestsFromMapFunc(r.memcachedNamespaceMapFunc(ctx, crs))).
		Watches(
			&source.Kind{Type: &corev1.Secret{}},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

func (r *NeutronAPIReconciler) findObjectsForSrc(src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	l := log.FromContext(context.Background()).WithName("Controllers").WithName("NeutronAPI")

	for _, field := range allWatchFields {
		crList := &neutronv1beta1.NeutronAPIList{}
		listOps := &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(field, src.GetName()),
			Namespace:     src.GetNamespace(),
		}
		err := r.List(context.TODO(), crList, listOps)
		if err != nil {
			return []reconcile.Request{}
		}

		for _, item := range crList.Items {
			l.Info(fmt.Sprintf("input source %s changed, reconcile: %s - %s", src.GetName(), item.GetName(), item.GetNamespace()))

			requests = append(requests,
				reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      item.GetName(),
						Namespace: item.GetNamespace(),
					},
				},
			)
		}
	}

	return requests
}

func (r *NeutronAPIReconciler) reconcileDelete(ctx context.Context, instance *neutronv1beta1.NeutronAPI, helper *helper.Helper) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling Service delete")

	// remove db finalizer first
	db, err := mariadbv1.GetDatabaseByName(ctx, helper, instance.Name)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	if !k8s_errors.IsNotFound(err) {
		if err := db.DeleteFinalizer(ctx, helper); err != nil {
			return ctrl.Result{}, err
		}
		Log.Info("Removed finalizer from our MariaDBDatabase")
	}

	// Remove the finalizer from our KeystoneEndpoint CR
	keystoneEndpoint, err := keystonev1.GetKeystoneEndpointWithName(ctx, helper, neutronapi.ServiceName, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	if err == nil {
		if controllerutil.RemoveFinalizer(keystoneEndpoint, helper.GetFinalizer()) {
			err = r.Update(ctx, keystoneEndpoint)
			if err != nil && !k8s_errors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			Log.Info("Removed finalizer from our KeystoneEndpoint")
		}
	}

	// Remove the finalizer from our KeystoneService CR
	keystoneService, err := keystonev1.GetKeystoneServiceWithName(ctx, helper, neutronapi.ServiceName, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	if err == nil {
		if controllerutil.RemoveFinalizer(keystoneService, helper.GetFinalizer()) {
			err = r.Update(ctx, keystoneService)
			if err != nil && !k8s_errors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			Log.Info("Removed finalizer from our KeystoneService")
		}
	}

	// Service is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	Log.Info("Reconciled Service delete successfully")

	return ctrl.Result{}, nil
}

func (r *NeutronAPIReconciler) reconcileInit(
	ctx context.Context,
	instance *neutronv1beta1.NeutronAPI,
	helper *helper.Helper,
	serviceLabels map[string]string,
	serviceAnnotations map[string]string,
	ospSecret *corev1.Secret,
	secretVars map[string]env.Setter,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling Service init")

	// create neutron DB instance
	//
	db := mariadbv1.NewDatabase(
		instance.Name,
		instance.Spec.DatabaseUser,
		instance.Spec.Secret,
		map[string]string{
			"dbName": instance.Spec.DatabaseInstance,
		},
	)
	// create or patch the DB
	ctrlResult, err := db.CreateOrPatchDBByName(
		ctx,
		helper,
		instance.Spec.DatabaseInstance,
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
	ctrlResult, err = db.WaitForDBCreatedWithTimeout(ctx, helper, time.Second*5)
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

	// Create Secrets required as input for the Service and calculate an overall hash of hashes
	//

	//
	// create Secret required for neutronapi and dbsync input. It contains minimal neutron config required
	// to get the service up, user can add additional files to be added to the service.
	err = r.generateServiceSecrets(ctx, helper, instance, ospSecret, &secretVars)
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
	// TLS input validation
	//
	// Validate the CA cert secret if provided
	if instance.Spec.TLS.CaBundleSecretName != "" {
		hash, ctrlResult, err := tls.ValidateCACertSecret(
			ctx,
			helper.GetClient(),
			types.NamespacedName{
				Name:      instance.Spec.TLS.CaBundleSecretName,
				Namespace: instance.Namespace,
			},
		)
		if err != nil {
			return ctrlResult, err
		} else if (ctrlResult != ctrl.Result{}) {
			return ctrlResult, nil
		}

		if hash != "" {
			secretVars[tls.CABundleKey] = env.SetValue(hash)
		}
	}

	// Validate API service certs secrets
	certsHash, ctrlResult, err := instance.Spec.TLS.API.ValidateCertSecrets(ctx, helper, instance.Namespace)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}
	secretVars[tls.TLSHashName] = env.SetValue(certsHash)

	//
	// create hash over all the different input resources to identify if any those changed
	// and a restart/recreate is required.
	//
	hashChanged, err := r.createHashOfInputHashes(ctx, instance, secretVars)
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
		return ctrl.Result{Requeue: true}, nil
	}
	// Create Secrets - end

	instance.Status.Conditions.MarkTrue(condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage)

	//
	// run Neutron db sync
	//

	dbSyncHash := instance.Status.Hash[neutronv1beta1.DbSyncHash]
	jobDef := neutronapi.DbSyncJob(instance, serviceLabels, serviceAnnotations)
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
		Log.Info(fmt.Sprintf("Job %s hash added - %s", jobDef.Name, instance.Status.Hash[neutronv1beta1.DbSyncHash]))
	}
	instance.Status.Conditions.MarkTrue(condition.DBSyncReadyCondition, condition.DBSyncReadyMessage)

	// run Neutron db sync - end

	//
	// expose the service (create service and return the created endpoint URLs)
	//
	neutronEndpoints := map[service.Endpoint]endpoint.Data{
		service.EndpointPublic: {
			Port: neutronapi.NeutronPublicPort,
		},
		service.EndpointInternal: {
			Port: neutronapi.NeutronInternalPort,
		},
	}
	apiEndpoints := make(map[string]string)

	for endpointType, data := range neutronEndpoints {
		endpointTypeStr := string(endpointType)
		endpointName := neutronapi.ServiceName + "-" + endpointTypeStr

		svcOverride := instance.Spec.Override.Service[endpointType]
		if svcOverride.EmbeddedLabelsAnnotations == nil {
			svcOverride.EmbeddedLabelsAnnotations = &service.EmbeddedLabelsAnnotations{}
		}

		exportLabels := util.MergeStringMaps(
			serviceLabels,
			map[string]string{
				service.AnnotationEndpointKey: endpointTypeStr,
			},
		)

		// Create the service
		svc, err := service.NewService(
			service.GenericService(&service.GenericServiceDetails{
				Name:      endpointName,
				Namespace: instance.Namespace,
				Labels:    exportLabels,
				Selector:  serviceLabels,
				Port: service.GenericServicePort{
					Name:     endpointName,
					Port:     data.Port,
					Protocol: corev1.ProtocolTCP,
				},
			}),
			5,
			&svcOverride.OverrideSpec,
		)
		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.ExposeServiceReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.ExposeServiceReadyErrorMessage,
				err.Error()))

			return ctrl.Result{}, err
		}

		svc.AddAnnotation(map[string]string{
			service.AnnotationEndpointKey: endpointTypeStr,
		})

		// add Annotation to whether creating an ingress is required or not
		if endpointType == service.EndpointPublic && svc.GetServiceType() == corev1.ServiceTypeClusterIP {
			svc.AddAnnotation(map[string]string{
				service.AnnotationIngressCreateKey: "true",
			})
		} else {
			svc.AddAnnotation(map[string]string{
				service.AnnotationIngressCreateKey: "false",
			})
			if svc.GetServiceType() == corev1.ServiceTypeLoadBalancer {
				svc.AddAnnotation(map[string]string{
					service.AnnotationHostnameKey: svc.GetServiceHostname(), // add annotation to register service name in dnsmasq
				})
			}
		}

		ctrlResult, err := svc.CreateOrPatch(ctx, helper)
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
		// create service - end

		// if TLS is enabled
		if instance.Spec.TLS.API.Enabled(endpointType) {
			// set endpoint protocol to https
			data.Protocol = ptr.To(service.ProtocolHTTPS)
		}

		apiEndpoints[string(endpointType)], err = svc.GetAPIEndpoint(
			svcOverride.EndpointURL, data.Protocol, data.Path)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	instance.Status.Conditions.MarkTrue(condition.ExposeServiceReadyCondition, condition.ExposeServiceReadyMessage)
	// expose service - end

	// update status with endpoint information
	Log.Info("Reconciling neutron KeystoneService")

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

	//
	// register endpoints
	//
	ksEndptSpec := keystonev1.KeystoneEndpointSpec{
		ServiceName: neutronapi.ServiceName,
		Endpoints:   apiEndpoints,
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

	Log.Info("Reconciled Service init successfully")
	return ctrl.Result{}, nil
}

func (r *NeutronAPIReconciler) reconcileUpdate(ctx context.Context, instance *neutronv1beta1.NeutronAPI, helper *helper.Helper) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling Service update")

	// TODO: should have minor update tasks if required
	// - delete dbsync hash from status to rerun it?

	Log.Info("Reconciled Service update successfully")
	return ctrl.Result{}, nil
}

func (r *NeutronAPIReconciler) reconcileUpgrade(ctx context.Context, instance *neutronv1beta1.NeutronAPI, helper *helper.Helper) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling Service upgrade")

	// TODO: should have major version upgrade tasks
	// -delete dbsync hash from status to rerun it?

	Log.Info("Reconciled Service upgrade successfully")
	return ctrl.Result{}, nil
}

func (r *NeutronAPIReconciler) reconcileNormal(ctx context.Context, instance *neutronv1beta1.NeutronAPI, helper *helper.Helper) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling Service")

	// Service account, role, binding
	rbacRules := []rbacv1.PolicyRule{
		{
			APIGroups:     []string{"security.openshift.io"},
			ResourceNames: []string{"anyuid", "hostmount-anyuid"},
			Resources:     []string{"securitycontextconstraints"},
			Verbs:         []string{"use"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"create", "get", "list", "watch", "update", "patch", "delete"},
		},
	}
	rbacResult, err := common_rbac.ReconcileRbac(ctx, helper, instance, rbacRules)
	if err != nil {
		return rbacResult, err
	} else if (rbacResult != ctrl.Result{}) {
		return rbacResult, nil
	}

	secretVars := make(map[string]env.Setter)

	//
	// create RabbitMQ transportURL CR and get the actual URL from the associated secret that is created
	//

	transportURL, op, err := r.transportURLCreateOrUpdate(instance)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.RabbitMqTransportURLReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.RabbitMqTransportURLReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	if op != controllerutil.OperationResultNone {
		Log.Info(fmt.Sprintf("TransportURL %s successfully reconciled - operation: %s", transportURL.Name, string(op)))
	}

	instance.Status.TransportURLSecret = transportURL.Status.SecretName

	if instance.Status.TransportURLSecret == "" {
		Log.Info(fmt.Sprintf("Waiting for TransportURL %s secret to be created", transportURL.Name))

		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.RabbitMqTransportURLReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.RabbitMqTransportURLReadyRunningMessage))

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
	secretVars[transportURLSecret.Name] = env.SetValue(hash)

	// run check TransportURL secret - end

	instance.Status.Conditions.MarkTrue(condition.RabbitMqTransportURLReadyCondition, condition.RabbitMqTransportURLReadyMessage)

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
	secretVars[ospSecret.Name] = env.SetValue(hash)

	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)
	// run check OpenStack secret - end

	memcached, err := r.getNeutronMemcached(ctx, helper, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.MemcachedReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.MemcachedReadyWaitingMessage))
			return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, fmt.Errorf("memcached %s not found", instance.Spec.MemcachedInstance)
		}
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.MemcachedReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.MemcachedReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	if !memcached.IsReady() {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.MemcachedReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.MemcachedReadyWaitingMessage))
		return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, fmt.Errorf("memcached %s is not ready", memcached.Name)
	}
	instance.Status.Conditions.MarkTrue(condition.MemcachedReadyCondition, condition.MemcachedReadyMessage)
	// run check memcached - end

	err = r.reconcileExternalSecrets(ctx, helper, instance, &secretVars)
	if err != nil {
		Log.Error(err, "Failed to reconcile external Secrets")
		return ctrl.Result{}, err
	}

	//
	// TODO check when/if Init, Update, or Upgrade should/could be skipped
	//

	serviceLabels := map[string]string{
		common.AppSelector: neutronapi.ServiceName,
	}

	// networks to attach to
	for _, netAtt := range instance.Spec.NetworkAttachments {
		_, err := nad.GetNADWithName(ctx, helper, netAtt, instance.Namespace)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.NetworkAttachmentsReadyCondition,
					condition.RequestedReason,
					condition.SeverityInfo,
					condition.NetworkAttachmentsReadyWaitingMessage,
					netAtt))
				return ctrl.Result{RequeueAfter: time.Second * 10}, fmt.Errorf("network-attachment-definition %s not found", netAtt)
			}
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.NetworkAttachmentsReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.NetworkAttachmentsReadyErrorMessage,
				err.Error()))
			return ctrl.Result{}, err
		}
	}

	serviceAnnotations, err := nad.CreateNetworksAnnotation(instance.Namespace, instance.Spec.NetworkAttachments)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed create network annotation from %s: %w",
			instance.Spec.NetworkAttachments, err)
	}

	// Handle service init
	ctrlResult, err := r.reconcileInit(ctx, instance, helper, serviceLabels, serviceAnnotations, ospSecret, secretVars)
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
	inputHash, ok := instance.Status.Hash[common.InputHashName]
	if !ok {
		return ctrlResult, fmt.Errorf("Failed to fetch input hash for Neutron deployment")
	}

	deplDef, err := neutronapi.Deployment(instance, inputHash, serviceLabels, serviceAnnotations)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DeploymentReadyErrorMessage,
			err.Error()))
		return ctrlResult, err
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

	// verify if network attachment matches expectations
	networkReady, networkAttachmentStatus, err := nad.VerifyNetworkStatusFromAnnotation(ctx, helper, instance.Spec.NetworkAttachments, serviceLabels, instance.Status.ReadyCount)
	if err != nil {
		return ctrl.Result{}, err
	}

	instance.Status.NetworkAttachments = networkAttachmentStatus
	if networkReady {
		instance.Status.Conditions.MarkTrue(condition.NetworkAttachmentsReadyCondition, condition.NetworkAttachmentsReadyMessage)
	} else {
		err := fmt.Errorf("not all pods have interfaces with ips as configured in NetworkAttachments: %s", instance.Spec.NetworkAttachments)
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.NetworkAttachmentsReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.NetworkAttachmentsReadyErrorMessage,
			err.Error()))

		return ctrl.Result{}, err
	}

	if instance.Status.ReadyCount > 0 {
		instance.Status.Conditions.MarkTrue(condition.DeploymentReadyCondition, condition.DeploymentReadyMessage)
	}
	// create Deployment - end

	Log.Info("Reconciled Service successfully")
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

func getExternalSecretName(instance *neutronv1beta1.NeutronAPI, serviceName string) string {
	return fmt.Sprintf("%s-%s-neutron-config", instance.Name, serviceName)
}

func getMetadataAgentSecretName(instance *neutronv1beta1.NeutronAPI) string {
	return getExternalSecretName(instance, "ovn-metadata-agent")
}

func getOvnAgentSecretName(instance *neutronv1beta1.NeutronAPI) string {
	return getExternalSecretName(instance, "ovn-agent")
}

func getSriovAgentSecretName(instance *neutronv1beta1.NeutronAPI) string {
	return getExternalSecretName(instance, "sriov-agent")
}

func getDhcpAgentSecretName(instance *neutronv1beta1.NeutronAPI) string {
	return getExternalSecretName(instance, "dhcp-agent")
}

func (r *NeutronAPIReconciler) reconcileExternalMetadataAgentSecret(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	envVars *map[string]env.Setter,
) error {
	sbCluster, err := ovnclient.GetDBClusterByType(ctx, h, instance.Namespace, map[string]string{}, ovnclient.SBDBType)
	if err != nil {
		err = r.deleteExternalSecret(ctx, h, instance, getMetadataAgentSecretName(instance))
		if err != nil {
			return fmt.Errorf("Failed to delete Neutron Metadata Agent external Secret: %w", err)
		}
		return nil
	}

	sbEndpoint, err := sbCluster.GetExternalEndpoint()
	if err != nil {
		err = r.deleteExternalSecret(ctx, h, instance, getMetadataAgentSecretName(instance))
		if err != nil {
			return fmt.Errorf("Failed to delete Neutron Metadata Agent external Secret: %w", err)
		}
		return nil
	}

	err = r.ensureExternalMetadataAgentSecret(ctx, h, instance, sbEndpoint, envVars)
	if err != nil {
		return fmt.Errorf("Failed to ensure Neutron Metadata Agent external Secret: %w", err)
	}
	return nil
}

func (r *NeutronAPIReconciler) reconcileExternalOvnAgentSecret(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	envVars *map[string]env.Setter,
) error {
	nbCluster, err := ovnclient.GetDBClusterByType(ctx, h, instance.Namespace, map[string]string{}, ovnclient.NBDBType)
	if err != nil {
		err = r.deleteExternalSecret(ctx, h, instance, getOvnAgentSecretName(instance))
		if err != nil {
			return fmt.Errorf("Failed to delete Neutron Ovn Agent external Secret: %w", err)
		}
		return nil
	}

	nbEndpoint, err := nbCluster.GetExternalEndpoint()
	if err != nil {
		err = r.deleteExternalSecret(ctx, h, instance, getOvnAgentSecretName(instance))
		if err != nil {
			return fmt.Errorf("Failed to delete Neutron Ovn Agent external Secret: %w", err)
		}
		return nil
	}

	sbCluster, err := ovnclient.GetDBClusterByType(ctx, h, instance.Namespace, map[string]string{}, ovnclient.SBDBType)
	if err != nil {
		err = r.deleteExternalSecret(ctx, h, instance, getOvnAgentSecretName(instance))
		if err != nil {
			return fmt.Errorf("Failed to delete Neutron Ovn Agent external Secret: %w", err)
		}
		return nil
	}

	sbEndpoint, err := sbCluster.GetExternalEndpoint()
	if err != nil {
		err = r.deleteExternalSecret(ctx, h, instance, getOvnAgentSecretName(instance))
		if err != nil {
			return fmt.Errorf("Failed to delete Neutron Ovn Agent external Secret: %w", err)
		}
		return nil
	}

	err = r.ensureExternalOvnAgentSecret(ctx, h, instance, nbEndpoint, sbEndpoint, envVars)
	if err != nil {
		return fmt.Errorf("Failed to ensure Neutron Ovn Agent external Secret: %w", err)
	}
	return nil
}

func (r *NeutronAPIReconciler) getTransportURL(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
) (string, error) {
	transportURLSecret, _, err := secret.GetSecret(ctx, h, instance.Status.TransportURLSecret, instance.Namespace)
	if err != nil {
		return "", err
	}
	transportURL, ok := transportURLSecret.Data["transport_url"]
	if !ok {
		return "", fmt.Errorf("No transport_url key found in Transport Secret")
	}
	return string(transportURL), nil
}

func (r *NeutronAPIReconciler) reconcileExternalSriovAgentSecret(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	envVars *map[string]env.Setter,
) error {
	transportURL, err := r.getTransportURL(ctx, h, instance)
	if err != nil {
		err = r.deleteExternalSecret(ctx, h, instance, getSriovAgentSecretName(instance))
		if err != nil {
			return fmt.Errorf("Failed to delete Neutron SR-IOV Agent external Secret: %w", err)
		}
		return nil
	}
	err = r.ensureExternalSriovAgentSecret(ctx, h, instance, transportURL, envVars)
	if err != nil {
		return fmt.Errorf("Failed to ensure Neutron SR-IOV Agent external Secret: %w", err)
	}
	return nil
}

func (r *NeutronAPIReconciler) reconcileExternalDhcpAgentSecret(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	envVars *map[string]env.Setter,
) error {
	transportURLSecret, _, err := secret.GetSecret(ctx, h, instance.Status.TransportURLSecret, instance.Namespace)
	if err != nil {
		err = r.deleteExternalSecret(ctx, h, instance, getDhcpAgentSecretName(instance))
		if err != nil {
			return fmt.Errorf("Failed to delete Neutron DHCP Agent external Secret: %w", err)
		}
		return nil
	}
	transportURL, ok := transportURLSecret.Data["transport_url"]
	if !ok {
		err = r.deleteExternalSecret(ctx, h, instance, getDhcpAgentSecretName(instance))
		if err != nil {
			return fmt.Errorf("Failed to delete Neutron DHCP Agent external Secret: %w", err)
		}
		return nil
	}
	err = r.ensureExternalDhcpAgentSecret(ctx, h, instance, string(transportURL), envVars)
	if err != nil {
		return fmt.Errorf("Failed to ensure Neutron DHCP Agent external Secret: %w", err)
	}
	return nil
}

// TODO(ihar) - is there any hashing mechanism for EDP config? do we trigger deploy somehow?
func (r *NeutronAPIReconciler) reconcileExternalSecrets(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	envVars *map[string]env.Setter,
) error {
	Log := r.GetLogger(ctx)
	// Generate one Secret per external service
	err := r.reconcileExternalMetadataAgentSecret(ctx, h, instance, envVars)
	if err != nil {
		return fmt.Errorf("Failed to reconcile Neutron Metadata Agent external Secret: %w", err)
	}
	err = r.reconcileExternalOvnAgentSecret(ctx, h, instance, envVars)
	if err != nil {
		return fmt.Errorf("Failed to reconcile Neutron Ovn Agent external Secret: %w", err)
	}
	err = r.reconcileExternalSriovAgentSecret(ctx, h, instance, envVars)
	if err != nil {
		return fmt.Errorf("Failed to reconcile Neutron SR-IOV Agent external Secret: %w", err)
	}
	err = r.reconcileExternalDhcpAgentSecret(ctx, h, instance, envVars)
	if err != nil {
		return fmt.Errorf("Failed to reconcile Neutron DHCP Agent external Secret: %w", err)
	}
	// NOTE(ihar): Add config reconciliation code for any other services here
	Log.Info(fmt.Sprintf("Reconciled external secrets for %s", instance.Name))
	return nil
}

// TODO(ihar) this function could live in lib-common
func (r *NeutronAPIReconciler) deleteExternalSecret(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	secretName string,
) error {
	cm := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: instance.Namespace,
		},
	}

	err := h.GetClient().Delete(ctx, cm)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return fmt.Errorf("Failed to delete external Secret %s: %w", secretName, err)
	}

	// Remove hash
	delete(instance.Status.Hash, secretName)

	return nil
}

func (r *NeutronAPIReconciler) ensureExternalSecret(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	secretName string,
	templates map[string]string,
	templateParameters map[string]interface{},
	envVars *map[string]env.Setter,
) error {
	secretLabels := labels.GetLabels(instance, labels.GetGroupLabel(neutronapi.ServiceName), map[string]string{})

	secrets := []util.Template{
		{
			Name:               secretName,
			Namespace:          instance.Namespace,
			Type:               util.TemplateTypeNone,
			InstanceType:       instance.Kind,
			Labels:             secretLabels,
			ConfigOptions:      templateParameters,
			AdditionalTemplate: templates,
		},
	}
	err := secret.EnsureSecrets(ctx, h, instance, secrets, envVars)
	if err != nil {
		return err
	}

	// Save hash
	a := &corev1.EnvVar{}
	(*envVars)[secretName](a)
	instance.Status.Hash[secretName] = a.Value
	return nil
}

func (r *NeutronAPIReconciler) ensureExternalMetadataAgentSecret(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	sbEndpoint string,
	envVars *map[string]env.Setter,
) error {
	templates := map[string]string{
		neutronapi.NeutronOVNMetadataAgentSecretKey: "/ovn-metadata-agent.conf",
	}
	templateParameters := make(map[string]interface{})
	templateParameters["SBConnection"] = sbEndpoint

	secretName := getMetadataAgentSecretName(instance)
	return r.ensureExternalSecret(ctx, h, instance, secretName, templates, templateParameters, envVars)
}

func (r *NeutronAPIReconciler) ensureExternalOvnAgentSecret(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	nbEndpoint string,
	sbEndpoint string,
	envVars *map[string]env.Setter,
) error {
	templates := map[string]string{
		neutronapi.NeutronOVNAgentSecretKey: "/ovn-agent.conf",
	}
	templateParameters := make(map[string]interface{})
	templateParameters["NBConnection"] = nbEndpoint
	templateParameters["SBConnection"] = sbEndpoint

	secretName := getOvnAgentSecretName(instance)
	return r.ensureExternalSecret(ctx, h, instance, secretName, templates, templateParameters, envVars)
}

func (r *NeutronAPIReconciler) ensureExternalSriovAgentSecret(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	transportURL string,
	envVars *map[string]env.Setter,
) error {
	templates := map[string]string{
		neutronapi.NeutronSriovAgentSecretKey: "/sriov-agent.conf",
	}
	templateParameters := make(map[string]interface{})
	templateParameters["transportURL"] = transportURL

	secretName := getSriovAgentSecretName(instance)
	return r.ensureExternalSecret(ctx, h, instance, secretName, templates, templateParameters, envVars)
}

func (r *NeutronAPIReconciler) ensureExternalDhcpAgentSecret(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	transportURL string,
	envVars *map[string]env.Setter,
) error {
	templates := map[string]string{
		neutronapi.NeutronDhcpAgentSecretKey: "/dhcp-agent.conf",
	}
	templateParameters := make(map[string]interface{})
	templateParameters["transportURL"] = transportURL

	secretName := getDhcpAgentSecretName(instance)
	return r.ensureExternalSecret(ctx, h, instance, secretName, templates, templateParameters, envVars)
}

// generateServiceSecrets - create secrets which service configuration
// TODO(ihar) we may want to split generation of config for db-sync and for neutronapi main pod, which
// would allow the operator to proceed with db-sync without waiting for other configuration values
// TODO add DefaultConfigOverwrite
func (r *NeutronAPIReconciler) generateServiceSecrets(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	ospSecret *corev1.Secret,
	envVars *map[string]env.Setter,
) error {
	// Create/update secrets from templates
	cmLabels := labels.GetLabels(instance, labels.GetGroupLabel(neutronapi.ServiceName), map[string]string{})

	nbCluster, err := ovnclient.GetDBClusterByType(ctx, h, instance.Namespace, map[string]string{}, ovnclient.NBDBType)
	if err != nil {
		return err
	}
	nbEndpoint, err := nbCluster.GetInternalEndpoint()
	if err != nil {
		return err
	}

	sbCluster, err := ovnclient.GetDBClusterByType(ctx, h, instance.Namespace, map[string]string{}, ovnclient.SBDBType)
	if err != nil {
		return err
	}
	sbEndpoint, err := sbCluster.GetInternalEndpoint()
	if err != nil {
		return err
	}

	// customData hold any customization for the service.
	// 02-neutron-custom.conf is going to /etc/<service>.conf.d
	// 01-neutron.conf is going to /etc/<service>.conf.d such that it gets loaded before custom one
	// all other files get placed into /etc/<service> to allow overwrite of e.g. logging.conf or policy.json
	// TODO: make sure custom.conf can not be overwritten
	customData := map[string]string{"02-neutron-custom.conf": instance.Spec.CustomServiceConfig}
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

	transportURL, err := r.getTransportURL(ctx, h, instance)
	if err != nil {
		return err
	}

	mc, err := r.getNeutronMemcached(ctx, h, instance)
	if err != nil {
		return err
	}

	templateParameters := make(map[string]interface{})
	templateParameters["ServiceUser"] = instance.Spec.ServiceUser
	templateParameters["KeystoneInternalURL"] = keystoneInternalURL
	templateParameters["KeystonePublicURL"] = keystonePublicURL
	templateParameters["TransportURL"] = transportURL
	templateParameters["MemcachedServers"] = strings.Join(mc.Status.ServerList, ",")
	templateParameters["MemcachedServersWithInet"] = strings.Join(mc.Status.ServerListWithInet, ",")

	// Other OpenStack services
	servicePassword := string(ospSecret.Data[instance.Spec.PasswordSelectors.Service])
	databasePassword := string(ospSecret.Data[instance.Spec.PasswordSelectors.Database])
	templateParameters["ServicePassword"] = servicePassword

	// Database
	templateParameters["DbHost"] = instance.Status.DatabaseHostname
	templateParameters["DbUser"] = instance.Spec.DatabaseUser
	templateParameters["DbPassword"] = databasePassword
	templateParameters["Db"] = neutronapi.Database

	// OVN
	templateParameters["NBConnection"] = nbEndpoint
	templateParameters["SBConnection"] = sbEndpoint

	// create httpd  vhost template parameters
	httpdVhostConfig := map[string]interface{}{}
	for _, endpt := range []service.Endpoint{service.EndpointInternal, service.EndpointPublic} {
		endptConfig := map[string]interface{}{}
		endptConfig["ServerName"] = fmt.Sprintf("neutron-%s.%s.svc", endpt.String(), instance.Namespace)
		endptConfig["TLS"] = false // default TLS to false, and set it bellow to true if enabled
		if instance.Spec.TLS.API.Enabled(endpt) {
			endptConfig["TLS"] = true
			endptConfig["SSLCertificateFile"] = fmt.Sprintf("/etc/pki/tls/certs/%s.crt", endpt.String())
			endptConfig["SSLCertificateKeyFile"] = fmt.Sprintf("/etc/pki/tls/private/%s.key", endpt.String())
		}
		httpdVhostConfig[endpt.String()] = endptConfig
	}

	templateParameters["VHosts"] = httpdVhostConfig

	secrets := []util.Template{
		{
			Name:          fmt.Sprintf("%s-config", instance.Name),
			Namespace:     instance.Namespace,
			Type:          util.TemplateTypeConfig,
			InstanceType:  instance.Kind,
			CustomData:    customData,
			Labels:        cmLabels,
			ConfigOptions: templateParameters,
		},
		{
			Name:         fmt.Sprintf("%s-httpd-config", instance.Name),
			Namespace:    instance.Namespace,
			Type:         util.TemplateTypeNone,
			InstanceType: instance.Kind,
			Labels:       cmLabels,
			AdditionalTemplate: map[string]string{
				"httpd.conf":            "/neutronapi/httpd/httpd.conf",
				"10-neutron-httpd.conf": "/neutronapi/httpd/10-neutron-httpd.conf",
				"ssl.conf":              "/neutronapi/httpd/ssl.conf",
			},
			ConfigOptions: templateParameters,
		},
	}
	return secret.EnsureSecrets(ctx, h, instance, secrets, envVars)
}

// createHashOfInputHashes - creates a hash of hashes which gets added to the resources which requires a restart
// if any of the input resources change, like configs, passwords, ...
//
// returns whether the hash changed (as a bool) and any error
func (r *NeutronAPIReconciler) createHashOfInputHashes(
	ctx context.Context,
	instance *neutronv1beta1.NeutronAPI,
	envVars map[string]env.Setter,
) (bool, error) {
	Log := r.GetLogger(ctx)

	var hashMap map[string]string
	changed := false
	mergedMapVars := env.MergeEnvs([]corev1.EnvVar{}, envVars)
	hash, err := util.ObjectHash(mergedMapVars)
	if err != nil {
		return changed, err
	}
	if hashMap, changed = util.SetHash(instance.Status.Hash, common.InputHashName, hash); changed {
		instance.Status.Hash = hashMap
		Log.Info(fmt.Sprintf("Input maps hash %s - %s", common.InputHashName, hash))
	}
	return changed, nil
}

func (r *NeutronAPIReconciler) memcachedNamespaceMapFunc(ctx context.Context, clt client.ObjectList) handler.MapFunc {
	Log := r.GetLogger(ctx)

	return func(o client.Object) []reconcile.Request {
		result := []reconcile.Request{}

		// get all Neutron CRs
		neutrons := &neutronv1beta1.NeutronAPIList{}
		listOpts := []client.ListOption{
			client.InNamespace(o.GetNamespace()),
		}
		if err := r.Client.List(context.Background(), neutrons, listOpts...); err != nil {
			Log.Error(err, "Unable to retrieve Neutron CRs %w")
			return nil
		}

		for _, cr := range neutrons.Items {
			if o.GetName() == cr.Spec.MemcachedInstance {
				name := client.ObjectKey{
					Namespace: o.GetNamespace(),
					Name:      cr.Name,
				}
				Log.Info(fmt.Sprintf("Memcached %s is used by Neutron CR %s", o.GetName(), cr.Name))
				result = append(result, reconcile.Request{NamespacedName: name})
			}
		}
		if len(result) > 0 {
			return result
		}
		return nil
	}
}

// getNeutronMemcached - gets the Memcached instance used for neutron cache backend
func (r *NeutronAPIReconciler) getNeutronMemcached(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
) (*memcachedv1.Memcached, error) {
	memcached := &memcachedv1.Memcached{}
	err := h.GetClient().Get(
		ctx,
		types.NamespacedName{
			Name:      instance.Spec.MemcachedInstance,
			Namespace: instance.Namespace,
		},
		memcached)
	if err != nil {
		return nil, err
	}
	return memcached, err
}
