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

// Package controller contains the NeutronAPI controller for the neutron-operator.
package controller

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sort"
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

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
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
	"github.com/openstack-k8s-operators/neutron-operator/internal/neutronapi"
	ovnclient "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// errTransportURLSecretNameNilOrEmpty
var errTransportURLSecretNameNilOrEmpty = errors.New("transport_url secret name is nil or empty")

// GetLogger returns a logger object with a prefix of "controller.name" and additional controller context fields
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
// +kubebuilder:rbac:groups=neutron.openstack.org,resources=neutronapis/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;patch;update;delete;
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;patch;update;delete;
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;patch;update;delete;
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbaccounts/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list;watch;
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=ovn.openstack.org,resources=ovndbclusters,verbs=get;list;watch;
// +kubebuilder:rbac:groups=rabbitmq.openstack.org,resources=transporturls,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=memcached.openstack.org,resources=memcacheds,verbs=get;list;watch;
// +kubebuilder:rbac:groups=k8s.cni.cncf.io,resources=network-attachment-definitions,verbs=get;list;watch

// service account, role, rolebinding
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update;patch
// service account permissions that are needed to grant permission to the above
// +kubebuilder:rbac:groups="security.openshift.io",resourceNames=anyuid,resources=securitycontextconstraints,verbs=use
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;delete;get;list;patch;update;watch;patch
// +kubebuilder:rbac:groups=topology.openstack.org,resources=topologies,verbs=get;list;watch;update

// Reconcile - neutron api
func (r *NeutronAPIReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	Log := r.GetLogger(ctx)

	// Fetch the NeutronAPI instance
	instance := &neutronv1beta1.NeutronAPI{}
	err := r.Get(ctx, req.NamespacedName, instance)
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

	// initialize status if Conditions is nil, but do not reset if it already
	// exists
	isNewInstance := instance.Status.Conditions == nil
	if isNewInstance {
		instance.Status.Conditions = condition.Conditions{}
	}

	// Save a copy of the condtions so that we can restore the LastTransitionTime
	// when a condition's state doesn't change.
	savedConditions := instance.Status.Conditions.DeepCopy()

	// Always patch the instance status when exiting this function so we can
	// persist any changes.
	defer func() {
		// Don't update the status, if Reconciler Panics
		if r := recover(); r != nil {
			Log.Info(fmt.Sprintf("Panic during reconcile %v\n", r))
			panic(r)
		}
		condition.RestoreLastTransitionTimes(
			&instance.Status.Conditions, savedConditions)
		if instance.Status.Conditions.IsUnknown(condition.ReadyCondition) {
			instance.Status.Conditions.Set(
				instance.Status.Conditions.Mirror(condition.ReadyCondition))
		}
		err := helper.PatchInstance(ctx, instance)
		if err != nil {
			_err = err
			return
		}
	}()

	//
	// initialize status
	//
	cl := condition.CreateList(
		condition.UnknownCondition(condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage),
		condition.UnknownCondition(condition.DBReadyCondition, condition.InitReason, condition.DBReadyInitMessage),
		condition.UnknownCondition(condition.DBSyncReadyCondition, condition.InitReason, condition.DBSyncReadyInitMessage),
		condition.UnknownCondition(condition.CreateServiceReadyCondition, condition.InitReason, condition.CreateServiceReadyInitMessage),
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
		condition.UnknownCondition(condition.NotificationBusInstanceReadyCondition, condition.InitReason, condition.NotificationBusInstanceReadyInitMessage),
	)

	instance.Status.Conditions.Init(&cl)
	instance.Status.ObservedGeneration = instance.Generation

	// If we're not deleting this and the service object doesn't have our finalizer, add it.
	if instance.DeletionTimestamp.IsZero() && controllerutil.AddFinalizer(instance, helper.GetFinalizer()) || isNewInstance {
		return ctrl.Result{}, nil
	}
	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}
	if instance.Status.NetworkAttachments == nil {
		instance.Status.NetworkAttachments = map[string][]string{}
	}
	// Init Topology condition if there's a reference
	if instance.Spec.TopologyRef != nil {
		c := condition.UnknownCondition(condition.TopologyReadyCondition, condition.InitReason, condition.TopologyReadyInitMessage)
		cl.Set(c)
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
	caBundleSecretNameField = ".spec.tls.caBundleSecretName" // #nosec
	tlsAPIInternalField     = ".spec.tls.api.internal.secretName"
	tlsAPIPublicField       = ".spec.tls.api.public.secretName"
	tlsOVNField             = ".spec.tls.ovn.secretName"
	topologyField           = ".spec.topologyRef.Name"
)

var allWatchFields = []string{
	passwordSecretField,
	caBundleSecretNameField,
	tlsAPIInternalField,
	tlsAPIPublicField,
	tlsOVNField,
	topologyField,
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

	// index tlsOVNField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &neutronv1beta1.NeutronAPI{}, tlsOVNField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*neutronv1beta1.NeutronAPI)
		if cr.Spec.TLS.Ovn.SecretName == nil {
			return nil
		}
		return []string{*cr.Spec.TLS.Ovn.SecretName}
	}); err != nil {
		return err
	}

	// index topologyField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &neutronv1beta1.NeutronAPI{}, topologyField, func(rawObj client.Object) []string {
		// Extract the topology name from the spec, if one is provided
		cr := rawObj.(*neutronv1beta1.NeutronAPI)
		if cr.Spec.TopologyRef == nil {
			return nil
		}
		return []string{cr.Spec.TopologyRef.Name}
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
		Watches(&ovnclient.OVNDBCluster{}, handler.EnqueueRequestsFromMapFunc(ovnclient.OVNCRNamespaceMapFunc(crs, mgr.GetClient()))).
		Watches(&memcachedv1.Memcached{}, handler.EnqueueRequestsFromMapFunc(r.memcachedNamespaceMapFunc(ctx))).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&topologyv1.Topology{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&keystonev1.KeystoneAPI{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectForSrc),
			builder.WithPredicates(keystonev1.KeystoneAPIStatusChangedPredicate)).
		Complete(r)
}

func (r *NeutronAPIReconciler) findObjectsForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	Log := r.GetLogger(ctx)

	for _, field := range allWatchFields {
		crList := &neutronv1beta1.NeutronAPIList{}
		listOps := &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(field, src.GetName()),
			Namespace:     src.GetNamespace(),
		}
		err := r.List(ctx, crList, listOps)
		if err != nil {
			Log.Error(err, fmt.Sprintf("listing %s for field: %s - %s", crList.GroupVersionKind().Kind, field, src.GetNamespace()))
			return requests
		}

		for _, item := range crList.Items {
			Log.Info(fmt.Sprintf("input source %s changed, reconcile: %s - %s", src.GetName(), item.GetName(), item.GetNamespace()))

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

func (r *NeutronAPIReconciler) findObjectForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	Log := r.GetLogger(ctx)

	crList := &neutronv1beta1.NeutronAPIList{}
	listOps := &client.ListOptions{
		Namespace: src.GetNamespace(),
	}
	err := r.List(ctx, crList, listOps)
	if err != nil {
		Log.Error(err, fmt.Sprintf("listing %s for namespace: %s", crList.GroupVersionKind().Kind, src.GetNamespace()))
		return requests
	}

	for _, item := range crList.Items {
		Log.Info(fmt.Sprintf("input source %s changed, reconcile: %s - %s", src.GetName(), item.GetName(), item.GetNamespace()))

		requests = append(requests,
			reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      item.GetName(),
					Namespace: item.GetNamespace(),
				},
			},
		)
	}

	return requests
}

func (r *NeutronAPIReconciler) reconcileDelete(ctx context.Context, instance *neutronv1beta1.NeutronAPI, helper *helper.Helper) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling Service delete")

	// remove db finalizer first
	db, err := mariadbv1.GetDatabaseByNameAndAccount(ctx, helper, neutronapi.DatabaseCRName, instance.Spec.DatabaseAccount, instance.Namespace)
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

	// Remove finalizer on the Topology CR
	if ctrlResult, err := topologyv1.EnsureDeletedTopologyRef(
		ctx,
		helper,
		instance.Status.LastAppliedTopology,
		instance.Name,
	); err != nil {
		return ctrlResult, err
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
	secretVars map[string]env.Setter,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling Service init")

	db, result, err := r.ensureDB(ctx, helper, instance)
	if err != nil {
		return ctrl.Result{}, err
	} else if (result != ctrl.Result{}) {
		return result, nil
	}

	// Create Secrets required as input for the Service and calculate an overall hash of hashes
	//

	//
	// create Secret required for neutronapi and dbsync input. It contains minimal neutron config required
	// to get the service up, user can add additional files to be added to the service.
	err = r.generateServiceSecrets(ctx, helper, instance, &secretVars, db)
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
		caHash, err := tls.ValidateCACertSecret(
			ctx,
			helper.GetClient(),
			types.NamespacedName{
				Name:      instance.Spec.TLS.CaBundleSecretName,
				Namespace: instance.Namespace,
			},
		)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				// Since the CA cert secret should have been manually created by the user and provided in the spec,
				// we treat this as a warning because it means that the service will not be able to start.
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.TLSInputReadyCondition,
					condition.ErrorReason,
					condition.SeverityWarning,
					condition.TLSInputReadyWaitingMessage, instance.Spec.TLS.CaBundleSecretName))
				return ctrl.Result{}, nil
			}
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.TLSInputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.TLSInputErrorMessage,
				err.Error()))
			return ctrl.Result{}, err
		}

		if caHash != "" {
			secretVars[tls.CABundleKey] = env.SetValue(caHash)
		}
	}

	// Validate API service certs secrets
	apiCertsHash, err := instance.Spec.TLS.API.ValidateCertSecrets(ctx, helper, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.TLSInputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.TLSInputReadyWaitingMessage, err.Error()))
			return ctrl.Result{}, nil
		}
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.TLSInputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.TLSInputErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	if apiCertsHash != "" {
		secretVars["apiCertsHash"] = env.SetValue(apiCertsHash)
	}

	// Validate OVN service cert secret
	if instance.Spec.TLS.Ovn.Enabled() {
		ovnCertsHash, err := instance.Spec.TLS.Ovn.ValidateCertSecret(ctx, helper, instance.Namespace)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				// This cert secret should be automatically created by the encompassing OpenStackControlPlane,
				// so we treat this as as info with the expectation that it will be created soon.
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.TLSInputReadyCondition,
					condition.RequestedReason,
					condition.SeverityInfo,
					condition.TLSInputReadyWaitingMessage, err.Error()))
				return ctrl.Result{}, nil
			}
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.TLSInputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.TLSInputErrorMessage,
				err.Error()))
			return ctrl.Result{}, err
		}

		if ovnCertsHash != "" {
			secretVars["ovnCertsHash"] = env.SetValue(ovnCertsHash)
		}
	}

	// all cert input checks out so report InputReady
	instance.Status.Conditions.MarkTrue(condition.TLSInputReadyCondition, condition.InputReadyMessage)

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
	ctrlResult, err := dbSyncjob.DoJob(
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

	// Sort endpoint types for deterministic iteration
	var endpointTypes []service.Endpoint
	for endpointType := range neutronEndpoints {
		endpointTypes = append(endpointTypes, endpointType)
	}
	sort.SliceStable(endpointTypes, func(i, j int) bool {
		return string(endpointTypes[i]) < string(endpointTypes[j])
	})

	for _, endpointType := range endpointTypes {
		data := neutronEndpoints[endpointType]
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
				condition.CreateServiceReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.CreateServiceReadyErrorMessage,
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
				condition.CreateServiceReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.CreateServiceReadyErrorMessage,
				err.Error()))

			return ctrlResult, err
		} else if (ctrlResult != ctrl.Result{}) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.CreateServiceReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.CreateServiceReadyRunningMessage))
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

	instance.Status.Conditions.MarkTrue(condition.CreateServiceReadyCondition, condition.CreateServiceReadyMessage)
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

func (r *NeutronAPIReconciler) reconcileUpdate(ctx context.Context) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling Service update")

	// TODO: should have minor update tasks if required
	// - delete dbsync hash from status to rerun it?

	Log.Info("Reconciled Service update successfully")
	return ctrl.Result{}, nil
}

func (r *NeutronAPIReconciler) reconcileUpgrade(ctx context.Context) (ctrl.Result, error) {
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
			ResourceNames: []string{"anyuid"},
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

	transportURL, _, err := r.transportURLCreateOrUpdate(
		ctx, instance, instance.Name, instance.Spec.RabbitMqClusterName)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.RabbitMqTransportURLReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.RabbitMqTransportURLReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
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
	// notifications transporturl
	//
	notificationBusName := ""
	if instance.Spec.NotificationsBusInstance != nil {
		notificationBusName = *instance.Spec.NotificationsBusInstance
	}

	if notificationBusName != "" {
		notificationTransportURL, _, err := r.transportURLCreateOrUpdate(
			ctx, instance, "notifications", notificationBusName)

		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.NotificationBusInstanceReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.NotificationBusInstanceReadyErrorMessage,
				err.Error()))
			return ctrl.Result{}, err
		}

		if !notificationTransportURL.IsReady() {
			Log.Info(fmt.Sprintf("Waiting for Notifications TransportURL %s secret to be created", notificationTransportURL.Name))

			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.NotificationBusInstanceReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.NotificationBusInstanceReadyRunningMessage))

			return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, nil
		}

		notificationsTransportURLSecretHash, result, err := secret.VerifySecret(
			ctx,
			types.NamespacedName{Namespace: instance.Namespace, Name: notificationTransportURL.Status.SecretName},
			[]string{"transport_url"},
			helper.GetClient(),
			time.Duration(10)*time.Second,
		)
		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.InputReadyErrorMessage,
				err.Error()))
			return result, err
		} else if (result != ctrl.Result{}) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.InputReadyWaitingMessage))
			return result, err
		}

		instance.Status.NotificationsTransportURLSecret = &notificationTransportURL.Status.SecretName
		Log.Info(fmt.Sprintf("Notifications TransportURL %s secret created Successfully ", notificationTransportURL.Name))

		secretVars[instance.Status.TransportURLSecret] = env.SetValue(notificationsTransportURLSecretHash)
		instance.Status.Conditions.MarkTrue(condition.NotificationBusInstanceReadyCondition, condition.NotificationBusInstanceReadyMessage)
	} else {
		instance.Status.NotificationsTransportURLSecret = nil
		instance.Status.Conditions.Remove(condition.NotificationBusInstanceReadyCondition)

		// Ensure to delete the previous notifications transport url
		notificationTransportURLName := "notifications-neutron-transport"
		err = r.transportURLDeleted(ctx, instance, notificationTransportURLName)
		if err != nil {
			Log.Error(err, fmt.Sprintf("Could not delete notification TransportURL %s", notificationTransportURLName))
			return ctrl.Result{}, err
		}

	}

	//
	// check for required TransportURL secret holding transport URL string
	//

	transportURLSecretHash, result, err := secret.VerifySecret(
		ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: instance.Status.TransportURLSecret},
		[]string{"transport_url"},
		helper.GetClient(),
		time.Duration(10)*time.Second,
	)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyErrorMessage,
			err.Error()))
		return result, err
	} else if (result != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.InputReadyWaitingMessage))
		return result, err
	}

	secretVars[instance.Status.TransportURLSecret] = env.SetValue(transportURLSecretHash)

	// run check TransportURL secret - end

	instance.Status.Conditions.MarkTrue(condition.RabbitMqTransportURLReadyCondition, condition.RabbitMqTransportURLReadyMessage)

	// end transportURL

	//
	// check for required OpenStack secret holding passwords for service/admin user and add hash to the vars map,
	//
	ospSecretHash, result, err := secret.VerifySecret(
		ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.Secret},
		[]string{instance.Spec.PasswordSelectors.Service},
		helper.GetClient(),
		time.Duration(10)*time.Second,
	)

	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyErrorMessage,
			err.Error()))
		return result, err
	} else if (result != ctrl.Result{}) {
		// Since the OpenStack secret should have been manually created by the user and referenced in the spec,
		// we treat this as a warning because it means that the service will not be able to start.
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyWaitingMessage))
		return result, err
	}

	secretVars[instance.Spec.Secret] = env.SetValue(ospSecretHash)

	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)
	// run check OpenStack secret - end

	memcached, err := memcachedv1.GetMemcachedByName(ctx, helper, instance.Spec.MemcachedInstance, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Memcached should be automatically created by the encompassing OpenStackControlPlane,
			// but we don't propagate its name into the "memcachedInstance" field of other sub-resources,
			// so if it is missing at this point, it *could* be because there's a mismatch between the
			// name of the Memcached CR and the name of the Memcached instance referenced by this CR.
			// Since that situation would block further reconciliation, we treat it as a warning.
			Log.Info(fmt.Sprintf("memcached %s not found", instance.Spec.MemcachedInstance))
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.MemcachedReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.MemcachedReadyWaitingMessage))
			return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, nil
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
		Log.Info(fmt.Sprintf("memcached %s is not ready", memcached.Name))
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.MemcachedReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.MemcachedReadyWaitingMessage))
		return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, nil
	}
	instance.Status.Conditions.MarkTrue(condition.MemcachedReadyCondition, condition.MemcachedReadyMessage)

	// run check memcached - end
	err = r.reconcileExternalSecrets(ctx, helper, instance, &secretVars)
	if err != nil {
		Log.Error(err, "Failed to reconcile external Secrets")
		return ctrl.Result{}, err
	}

	err = r.reconcileExternalOVNSecrets(ctx, helper, instance, &secretVars)
	if err != nil {
		Log.Error(err, "Failed to reconcile external OVN Secrets")
		return ctrl.Result{}, err
	}

	//
	// TODO check when/if Init, Update, or Upgrade should/could be skipped
	//

	serviceLabels := map[string]string{
		common.AppSelector: neutronapi.ServiceName,
	}

	// networks to attach to
	nadList := []networkv1.NetworkAttachmentDefinition{}
	for _, netAtt := range instance.Spec.NetworkAttachments {
		nad, err := nad.GetNADWithName(ctx, helper, netAtt, instance.Namespace)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				// Since the net-attach-def CR should have been manually created by the user and referenced in the spec,
				// we treat this as a warning because it means that the service will not be able to start.
				Log.Info(fmt.Sprintf("network-attachment-definition %s not found", netAtt))
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.NetworkAttachmentsReadyCondition,
					condition.ErrorReason,
					condition.SeverityWarning,
					condition.NetworkAttachmentsReadyWaitingMessage,
					netAtt))
				return ctrl.Result{RequeueAfter: time.Second * 10}, nil
			}
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.NetworkAttachmentsReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.NetworkAttachmentsReadyErrorMessage,
				err.Error()))
			return ctrl.Result{}, err
		}

		if nad != nil {
			nadList = append(nadList, *nad)
		}
	}

	serviceAnnotations, err := nad.EnsureNetworksAnnotation(nadList)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed create network annotation from %s: %w",
			instance.Spec.NetworkAttachments, err)
	}

	// Handle service init
	ctrlResult, err := r.reconcileInit(ctx, instance, helper, serviceLabels, serviceAnnotations, secretVars)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service update
	ctrlResult, err = r.reconcileUpdate(ctx)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service upgrade
	ctrlResult, err = r.reconcileUpgrade(ctx)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Define a new Deployment object
	inputHash, ok := instance.Status.Hash[common.InputHashName]
	if !ok {
		return ctrlResult, fmt.Errorf("%w input hash for Neutron deployment", util.ErrInvalidStatus)
	}

	//
	// Handle Topology
	//
	topology, err := topologyv1.EnsureServiceTopology(
		ctx,
		helper,
		instance.Spec.TopologyRef,
		instance.Status.LastAppliedTopology,
		instance.Name,
		labels.GetLabelSelector(serviceLabels),
	)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.TopologyReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.TopologyReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, fmt.Errorf("waiting for Topology requirements: %w", err)
	}

	// If TopologyRef is present and ensureNeutronAPITopology returned a valid
	// topology object, set .Status.LastAppliedTopology to the referenced one
	// and mark the condition as true
	if instance.Spec.TopologyRef != nil {
		// update the Status with the last retrieved Topology name
		instance.Status.LastAppliedTopology = instance.Spec.TopologyRef
		// update the TopologyRef associated condition
		instance.Status.Conditions.MarkTrue(condition.TopologyReadyCondition, condition.TopologyReadyMessage)
	} else {
		// remove LastAppliedTopology from the .Status
		instance.Status.LastAppliedTopology = nil
	}

	deplDef, err := neutronapi.Deployment(instance, inputHash, serviceLabels, serviceAnnotations, topology, memcached)
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

	deploy := depl.GetDeployment()
	if deploy.Generation == deploy.Status.ObservedGeneration {
		instance.Status.ReadyCount = deploy.Status.ReadyReplicas

		// verify if network attachment matches expectations
		networkReady, networkAttachmentStatus, err := nad.VerifyNetworkStatusFromAnnotation(ctx, helper, instance.Spec.NetworkAttachments, serviceLabels, instance.Status.ReadyCount)
		if err != nil {
			return ctrl.Result{}, err
		}

		instance.Status.NetworkAttachments = networkAttachmentStatus
		if !networkReady {
			err := fmt.Errorf("%w with ips as configured in NetworkAttachments: %s", util.ErrPodsInterfaces, instance.Spec.NetworkAttachments)
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.NetworkAttachmentsReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.NetworkAttachmentsReadyErrorMessage,
				err.Error()))

			return ctrl.Result{}, err
		}
		instance.Status.Conditions.MarkTrue(condition.NetworkAttachmentsReadyCondition, condition.NetworkAttachmentsReadyMessage)

		// Mark the Deployment as Ready only if the number of Replicas is equals
		// to the Deployed instances (ReadyCount), and the the Status.Replicas
		// match Status.ReadyReplicas. If a deployment update is in progress,
		// Replicas > ReadyReplicas.
		// In addition, make sure the controller sees the last Generation
		// by comparing it with the ObservedGeneration.
		if deployment.IsReady(deploy) {
			instance.Status.Conditions.MarkTrue(
				condition.DeploymentReadyCondition,
				condition.DeploymentReadyMessage,
			)
		} else {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.DeploymentReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.DeploymentReadyRunningMessage))
		}
	}
	// create Deployment - end
	if instance.Status.ReadyCount > 0 {
		// remove finalizers from unused MariaDBAccount records
		err = mariadbv1.DeleteUnusedMariaDBAccountFinalizers(
			ctx, helper, neutronapi.DatabaseCRName,
			instance.Spec.DatabaseAccount, instance.Namespace)
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	// We reached the end of the Reconcile, update the Ready condition based on
	// the sub conditions
	if instance.Status.Conditions.AllSubConditionIsTrue() {
		instance.Status.Conditions.MarkTrue(
			condition.ReadyCondition, condition.ReadyMessage)
	}
	Log.Info("Reconciled Service successfully")
	return ctrl.Result{}, nil
}

func (r *NeutronAPIReconciler) transportURLCreateOrUpdate(
	ctx context.Context,
	instance *neutronv1beta1.NeutronAPI,
	transporturlName string,
	rabbitmqName string,
) (*rabbitmqv1.TransportURL, controllerutil.OperationResult, error) {
	Log := r.GetLogger(ctx)
	transportURL := &rabbitmqv1.TransportURL{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-neutron-transport", transporturlName),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.Client, transportURL, func() error {
		transportURL.Spec.RabbitmqClusterName = rabbitmqName

		err := controllerutil.SetControllerReference(instance, transportURL, r.Scheme)
		return err
	})

	if op != controllerutil.OperationResultNone {
		Log.Info(fmt.Sprintf("TransportURL %s successfully reconciled - operation: %s", transportURL.Name, string(op)))
	}

	return transportURL, op, err
}

func (r *NeutronAPIReconciler) transportURLDeleted(
	ctx context.Context,
	instance *neutronv1beta1.NeutronAPI,
	transportURLName string,
) error {
	Log := r.GetLogger(ctx)
	transportURL := &rabbitmqv1.TransportURL{
		ObjectMeta: metav1.ObjectMeta{
			Name:      transportURLName,
			Namespace: instance.Namespace,
		},
	}

	err := r.Delete(ctx, transportURL)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			return nil
		}
		Log.Info(fmt.Sprintf("Could not delete TransportURL %s err: %s", transportURLName, err))
		return err
	}

	Log.Info("Deleted transportURL", ":", transportURLName)

	return nil
}

func getExternalSecretName(instance *neutronv1beta1.NeutronAPI, serviceName string) string {
	return fmt.Sprintf("%s-%s-neutron-config", instance.Name, serviceName)
}

func getMetadataAgentSecretName(instance *neutronv1beta1.NeutronAPI) string {
	return getExternalSecretName(instance, "ovn-metadata-agent")
}

func getOVNAgentSecretName(instance *neutronv1beta1.NeutronAPI) string {
	return getExternalSecretName(instance, "ovn-agent")
}

func getSriovAgentSecretName(instance *neutronv1beta1.NeutronAPI) string {
	return getExternalSecretName(instance, "sriov-agent")
}

func getDhcpAgentSecretName(instance *neutronv1beta1.NeutronAPI) string {
	return getExternalSecretName(instance, "dhcp-agent")
}

func (r *NeutronAPIReconciler) reconcileExternalOVNMetadataAgentSecret(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	envVars *map[string]env.Setter,
) error {
	if !instance.IsOVNEnabled() {
		err := r.deleteExternalSecret(ctx, h, instance, getMetadataAgentSecretName(instance))
		if err != nil {
			return fmt.Errorf("failed to delete Neutron Metadata Agent external Secret: %w", err)
		}
		return nil
	}
	sbCluster, err := ovnclient.GetDBClusterByType(ctx, h, instance.Namespace, map[string]string{}, ovnclient.SBDBType)
	if err != nil {
		err = r.deleteExternalSecret(ctx, h, instance, getMetadataAgentSecretName(instance))
		if err != nil {
			return fmt.Errorf("failed to delete Neutron Metadata Agent external Secret: %w", err)
		}
		return nil
	}

	sbEndpoint, err := sbCluster.GetExternalEndpoint()
	if err != nil {
		err = r.deleteExternalSecret(ctx, h, instance, getMetadataAgentSecretName(instance))
		if err != nil {
			return fmt.Errorf("failed to delete Neutron Metadata Agent external Secret: %w", err)
		}
		return nil
	}

	err = r.ensureExternalOVNMetadataAgentSecret(ctx, h, instance, sbEndpoint, envVars)
	if err != nil {
		return fmt.Errorf("failed to ensure Neutron Metadata Agent external Secret: %w", err)
	}
	return nil
}

func (r *NeutronAPIReconciler) reconcileExternalOVNAgentSecret(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	envVars *map[string]env.Setter,
) error {
	if !instance.IsOVNEnabled() {
		err := r.deleteExternalSecret(ctx, h, instance, getOVNAgentSecretName(instance))
		if err != nil {
			return fmt.Errorf("failed to delete Neutron OVN Agent external Secret: %w", err)
		}
		return nil
	}
	nbCluster, err := ovnclient.GetDBClusterByType(ctx, h, instance.Namespace, map[string]string{}, ovnclient.NBDBType)
	if err != nil {
		err = r.deleteExternalSecret(ctx, h, instance, getOVNAgentSecretName(instance))
		if err != nil {
			return fmt.Errorf("failed to delete Neutron OVN Agent external Secret: %w", err)
		}
		return nil
	}

	nbEndpoint, err := nbCluster.GetExternalEndpoint()
	if err != nil {
		err = r.deleteExternalSecret(ctx, h, instance, getOVNAgentSecretName(instance))
		if err != nil {
			return fmt.Errorf("failed to delete Neutron OVN Agent external Secret: %w", err)
		}
		return nil
	}

	sbCluster, err := ovnclient.GetDBClusterByType(ctx, h, instance.Namespace, map[string]string{}, ovnclient.SBDBType)
	if err != nil {
		err = r.deleteExternalSecret(ctx, h, instance, getOVNAgentSecretName(instance))
		if err != nil {
			return fmt.Errorf("failed to delete Neutron OVN Agent external Secret: %w", err)
		}
		return nil
	}

	sbEndpoint, err := sbCluster.GetExternalEndpoint()
	if err != nil {
		err = r.deleteExternalSecret(ctx, h, instance, getOVNAgentSecretName(instance))
		if err != nil {
			return fmt.Errorf("failed to delete Neutron OVN Agent external Secret: %w", err)
		}
		return nil
	}

	err = r.ensureExternalOVNAgentSecret(ctx, h, instance, nbEndpoint, sbEndpoint, envVars)
	if err != nil {
		return fmt.Errorf("failed to ensure Neutron OVN Agent external Secret: %w", err)
	}
	return nil
}

// getTransportURL returns both the transport URL and quorum queues setting from the transport URL secret
func (r *NeutronAPIReconciler) getTransportURL(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	secretName *string,
) (string, bool, error) {
	if secretName == nil || *secretName == "" {
		return "", false, errTransportURLSecretNameNilOrEmpty
	}
	transportURLSecret, _, err := secret.GetSecret(ctx, h, *secretName, instance.Namespace)
	if err != nil {
		return "", false, err
	}

	transportURL, ok := transportURLSecret.Data["transport_url"]
	if !ok {
		return "", false, fmt.Errorf("transport_url %w Transport Secret", util.ErrNotFound)
	}

	quorumQueues := string(transportURLSecret.Data["quorumqueues"]) == "true"

	return string(transportURL), quorumQueues, nil
}

func (r *NeutronAPIReconciler) reconcileExternalSriovAgentSecret(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	envVars *map[string]env.Setter,
) error {
	transportURL, quorumQueues, err := r.getTransportURL(ctx, h, instance, &instance.Status.TransportURLSecret)
	if err != nil {
		err = r.deleteExternalSecret(ctx, h, instance, getSriovAgentSecretName(instance))
		if err != nil {
			return fmt.Errorf("failed to delete Neutron SR-IOV Agent external Secret: %w", err)
		}
		return nil
	}
	err = r.ensureExternalSriovAgentSecret(ctx, h, instance, transportURL, quorumQueues, envVars)
	if err != nil {
		return fmt.Errorf("failed to ensure Neutron SR-IOV Agent external Secret: %w", err)
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
			return fmt.Errorf("failed to delete Neutron DHCP Agent external Secret: %w", err)
		}
		return nil
	}
	transportURL, ok := transportURLSecret.Data["transport_url"]
	if !ok {
		err = r.deleteExternalSecret(ctx, h, instance, getDhcpAgentSecretName(instance))
		if err != nil {
			return fmt.Errorf("failed to delete Neutron DHCP Agent external Secret: %w", err)
		}
		return nil
	}
	quorumQueues := string(transportURLSecret.Data["quorumqueues"]) == "true"
	err = r.ensureExternalDhcpAgentSecret(ctx, h, instance, string(transportURL), quorumQueues, envVars)
	if err != nil {
		return fmt.Errorf("failed to ensure Neutron DHCP Agent external Secret: %w", err)
	}
	return nil
}

// TODO(ihar) - is there any hashing mechanism for EDP config? do we trigger deploy somehow?
// NOTE(ihar): Add config reconciliation code for any other services here or below
func (r *NeutronAPIReconciler) reconcileExternalSecrets(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	envVars *map[string]env.Setter,
) error {
	Log := r.GetLogger(ctx)
	// Generate one Secret per external service
	err := r.reconcileExternalSriovAgentSecret(ctx, h, instance, envVars)
	if err != nil {
		return fmt.Errorf("failed to reconcile Neutron SR-IOV Agent external Secret: %w", err)
	}
	err = r.reconcileExternalDhcpAgentSecret(ctx, h, instance, envVars)
	if err != nil {
		return fmt.Errorf("failed to reconcile Neutron DHCP Agent external Secret: %w", err)
	}

	Log.Info(fmt.Sprintf("Reconciled external secrets for %s", instance.Name))
	return nil
}

func (r *NeutronAPIReconciler) reconcileExternalOVNSecrets(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	envVars *map[string]env.Setter,
) error {
	Log := r.GetLogger(ctx)
	// Generate one Secret per external service
	err := r.reconcileExternalOVNMetadataAgentSecret(ctx, h, instance, envVars)
	if err != nil {
		return fmt.Errorf("failed to reconcile Neutron Metadata Agent external Secret: %w", err)
	}
	err = r.reconcileExternalOVNAgentSecret(ctx, h, instance, envVars)
	if err != nil {
		return fmt.Errorf("failed to reconcile Neutron OVN Agent external Secret: %w", err)
	}
	Log.Info(fmt.Sprintf("Reconciled external OVN secrets for %s", instance.Name))
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
		return fmt.Errorf("failed to delete external Secret %s: %w", secretName, err)
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
	templateParameters map[string]any,
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

func (r *NeutronAPIReconciler) ensureExternalOVNMetadataAgentSecret(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	sbEndpoint string,
	envVars *map[string]env.Setter,
) error {
	templates := map[string]string{
		neutronapi.NeutronOVNMetadataAgentSecretKey: "/ovn-metadata-agent.conf",
	}
	templateParameters := make(map[string]any)
	templateParameters["SBConnection"] = sbEndpoint
	templateParameters["OVNDB_TLS"] = instance.Spec.TLS.Ovn.Enabled()

	secretName := getMetadataAgentSecretName(instance)
	return r.ensureExternalSecret(ctx, h, instance, secretName, templates, templateParameters, envVars)
}

func (r *NeutronAPIReconciler) ensureExternalOVNAgentSecret(
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
	templateParameters := make(map[string]any)
	templateParameters["NBConnection"] = nbEndpoint
	templateParameters["SBConnection"] = sbEndpoint
	templateParameters["OVNDB_TLS"] = instance.Spec.TLS.Ovn.Enabled()

	secretName := getOVNAgentSecretName(instance)
	return r.ensureExternalSecret(ctx, h, instance, secretName, templates, templateParameters, envVars)
}

func (r *NeutronAPIReconciler) ensureExternalSriovAgentSecret(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	transportURL string,
	quorumQueues bool,
	envVars *map[string]env.Setter,
) error {
	templates := map[string]string{
		neutronapi.NeutronSriovAgentSecretKey: "/sriov-agent.conf",
	}
	templateParameters := make(map[string]any)
	templateParameters["transportURL"] = transportURL
	templateParameters["QuorumQueues"] = quorumQueues

	secretName := getSriovAgentSecretName(instance)
	return r.ensureExternalSecret(ctx, h, instance, secretName, templates, templateParameters, envVars)
}

func (r *NeutronAPIReconciler) ensureExternalDhcpAgentSecret(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	transportURL string,
	quorumQueues bool,
	envVars *map[string]env.Setter,
) error {
	templates := map[string]string{
		neutronapi.NeutronDhcpAgentSecretKey: "/dhcp-agent.conf",
	}
	templateParameters := make(map[string]any)
	templateParameters["transportURL"] = transportURL
	templateParameters["QuorumQueues"] = quorumQueues

	secretName := getDhcpAgentSecretName(instance)
	return r.ensureExternalSecret(ctx, h, instance, secretName, templates, templateParameters, envVars)
}

// generateServiceSecrets - create secrets which service configuration
// TODO(ihar) we may want to split generation of config for db-sync and for neutronapi main pod, which
// would allow the operator to proceed with db-sync without waiting for other configuration values
func (r *NeutronAPIReconciler) generateServiceSecrets(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	envVars *map[string]env.Setter,
	db *mariadbv1.Database,
) error {
	// Create/update secrets from templates
	cmLabels := labels.GetLabels(instance, labels.GetGroupLabel(neutronapi.ServiceName), map[string]string{})

	var tlsCfg *tls.Service
	if instance.Spec.TLS.CaBundleSecretName != "" {
		tlsCfg = &tls.Service{}
	}
	// customData hold any customization for the service.
	// 02-neutron-custom.conf is going to /etc/<service>/<service>.conf.d
	// 01-neutron.conf is going to /etc/<service>/<service>.conf.d such that it gets loaded before custom one
	// all other files get placed into /etc/<service> to allow overwrite of e.g. policy.yaml
	customData := map[string]string{
		"02-neutron-custom.conf": instance.Spec.CustomServiceConfig,
		"my.cnf":                 db.GetDatabaseClientConfig(tlsCfg), //(mschuppert) for now just get the default my.cnf
	}
	maps.Copy(customData, instance.Spec.DefaultConfigOverwrite)

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

	transportURL, quorumQueues, err := r.getTransportURL(ctx, h, instance, &instance.Status.TransportURLSecret)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.InputReadyWaitingMessage,
		))
		return err
	}

	mc, err := memcachedv1.GetMemcachedByName(ctx, h, instance.Spec.MemcachedInstance, instance.Namespace)
	if err != nil {
		return err
	}

	ospSecret, _, err := secret.GetSecret(ctx, h, instance.Spec.Secret, instance.Namespace)
	if err != nil {
		return err
	}

	templateParameters := make(map[string]any)
	templateParameters["ServiceUser"] = instance.Spec.ServiceUser
	templateParameters["KeystoneInternalURL"] = keystoneInternalURL
	templateParameters["KeystonePublicURL"] = keystonePublicURL
	templateParameters["TransportURL"] = transportURL
	templateParameters["MemcachedServers"] = mc.GetMemcachedServerListString()
	templateParameters["MemcachedServersWithInet"] = mc.GetMemcachedServerListWithInetString()
	templateParameters["MemcachedTLS"] = mc.GetMemcachedTLSSupport()
	templateParameters["TimeOut"] = instance.Spec.APITimeout
	templateParameters["QuorumQueues"] = quorumQueues

	notificationsTransportURL, _, err := r.getTransportURL(ctx, h, instance, instance.Status.NotificationsTransportURLSecret)
	if err != nil && !errors.Is(err, errTransportURLSecretNameNilOrEmpty) {
		// in case not configured yet.
		return err
	}
	if notificationsTransportURL != "" {
		templateParameters["NotificationsTransportURL"] = notificationsTransportURL
	}

	// MTLS
	if mc.GetMemcachedMTLSSecret() != "" {
		templateParameters["MemcachedAuthCert"] = fmt.Sprint(memcachedv1.CertMountPath())
		templateParameters["MemcachedAuthKey"] = fmt.Sprint(memcachedv1.KeyMountPath())
		templateParameters["MemcachedAuthCa"] = fmt.Sprint(memcachedv1.CaMountPath())
	}

	// Other OpenStack services
	servicePassword := string(ospSecret.Data[instance.Spec.PasswordSelectors.Service])
	templateParameters["ServicePassword"] = servicePassword

	// Database
	databaseAccount := db.GetAccount()
	dbSecret := db.GetSecret()

	templateParameters["DbHost"] = instance.Status.DatabaseHostname
	templateParameters["DbUser"] = databaseAccount.Spec.UserName
	templateParameters["DbPassword"] = string(dbSecret.Data[mariadbv1.DatabasePasswordSelector])
	templateParameters["Db"] = neutronapi.Database

	templateParameters["CorePlugin"] = instance.Spec.CorePlugin
	// TODO: add join func to template library?
	templateParameters["Ml2MechanismDrivers"] = strings.Join(instance.Spec.Ml2MechanismDrivers, ",")

	// OVN
	templateParameters["IsOVN"] = instance.IsOVNEnabled()
	if instance.IsOVNEnabled() {
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
		templateParameters["NBConnection"] = nbEndpoint
		templateParameters["SBConnection"] = sbEndpoint
		templateParameters["OVNDB_TLS"] = instance.Spec.TLS.Ovn.Enabled()
	}

	// create httpd  vhost template parameters
	httpdVhostConfig := map[string]any{}
	for _, endpt := range []service.Endpoint{service.EndpointInternal, service.EndpointPublic} {
		endptConfig := map[string]any{}
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

func (r *NeutronAPIReconciler) memcachedNamespaceMapFunc(ctx context.Context) handler.MapFunc {
	Log := r.GetLogger(ctx)

	return func(_ context.Context, o client.Object) []reconcile.Request {
		result := []reconcile.Request{}

		// get all Neutron CRs
		neutrons := &neutronv1beta1.NeutronAPIList{}
		listOpts := []client.ListOption{
			client.InNamespace(o.GetNamespace()),
		}
		if err := r.List(context.Background(), neutrons, listOpts...); err != nil {
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

// ensureDB - create neutron DB instance
func (r *NeutronAPIReconciler) ensureDB(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
) (*mariadbv1.Database, ctrl.Result, error) {
	// ensure MariaDBAccount exists.  This account record may be created by
	// openstack-operator or the cloud operator up front without a specific
	// MariaDBDatabase configured yet.   Otherwise, a MariaDBAccount CR is
	// created here with a generated username as well as a secret with
	// generated password.   The MariaDBAccount is created without being
	// yet associated with any MariaDBDatabase.
	_, _, err := mariadbv1.EnsureMariaDBAccount(
		ctx, h, instance.Spec.DatabaseAccount,
		instance.Namespace, false, neutronapi.DatabaseUsernamePrefix,
	)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			mariadbv1.MariaDBAccountReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			mariadbv1.MariaDBAccountNotReadyMessage,
			err.Error()))

		return nil, ctrl.Result{}, err
	}
	instance.Status.Conditions.MarkTrue(
		mariadbv1.MariaDBAccountReadyCondition,
		mariadbv1.MariaDBAccountReadyMessage)

	// create neutron DB instance
	//
	db := mariadbv1.NewDatabaseForAccount(
		instance.Spec.DatabaseInstance, // mariadb/galera service to target
		neutronapi.Database,            // name used in CREATE DATABASE in mariadb
		neutronapi.DatabaseCRName,      // CR name for MariaDBDatabase
		instance.Spec.DatabaseAccount,  // CR name for MariaDBAccount
		instance.Namespace,             // namespace
	)

	// create or patch the DB
	ctrlResult, err := db.CreateOrPatchAll(ctx, h)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBReadyErrorMessage,
			err.Error()))
		return db, ctrl.Result{}, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBReadyRunningMessage))
		return db, ctrlResult, nil
	}
	// wait for the DB to be setup
	ctrlResult, err = db.WaitForDBCreatedWithTimeout(ctx, h, time.Second*5)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBReadyErrorMessage,
			err.Error()))
		return db, ctrlResult, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBReadyRunningMessage))
		return db, ctrlResult, nil
	}
	// update Status.DatabaseHostname, used to bootstrap/config the service
	instance.Status.DatabaseHostname = db.GetDatabaseHostname()
	instance.Status.Conditions.MarkTrue(condition.DBReadyCondition, condition.DBReadyMessage)

	return db, ctrlResult, nil
}
