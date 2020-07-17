package ovncontroller

import (
	"context"
	"fmt"
	"reflect"
	"time"

	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	"github.com/openstack-k8s-operators/neutron-operator/pkg/common"
	"github.com/openstack-k8s-operators/neutron-operator/pkg/ovncontroller"
	appsv1 "k8s.io/api/apps/v1"

	neutronv1 "github.com/openstack-k8s-operators/neutron-operator/pkg/apis/neutron/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_ovncontroller")

// Add creates a new OVNController Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileOVNController{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("ovncontroller-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource OVNController
	err = c.Watch(&source.Kind{Type: &neutronv1.OVNController{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch ConfigMaps owned by OVNController
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &neutronv1.OVNController{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Pods and requeue the owner OVNController
	err = c.Watch(&source.Kind{Type: &appsv1.DaemonSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &neutronv1.OVNController{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileOVNController implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileOVNController{}

// ReconcileOVNController reconciles a OVNController object
type ReconcileOVNController struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a OVNController object and makes changes based on the state read
// and what is in the OVNController.Spec
func (r *ReconcileOVNController) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling OVNController")

	// Fetch the OVNController instance
	instance := &neutronv1.OVNController{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// ScriptsConfigMap
	scriptsConfigMap := ovncontroller.ScriptsConfigMap(instance, instance.Name+"-scripts")
	if err := controllerutil.SetControllerReference(instance, scriptsConfigMap, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	// Check if this ScriptsConfigMap already exists
	foundScriptsConfigMap := &corev1.ConfigMap{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: scriptsConfigMap.Name, Namespace: scriptsConfigMap.Namespace}, foundScriptsConfigMap)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating a new ScriptsConfigMap", "ScriptsConfigMap.Namespace", scriptsConfigMap.Namespace, "Job.Name", scriptsConfigMap.Name)
		err = r.client.Create(context.TODO(), scriptsConfigMap)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else if !reflect.DeepEqual(scriptsConfigMap.Data, foundScriptsConfigMap.Data) {
		reqLogger.Info("Updating ScriptsConfigMap")
		scriptsConfigMap.Data = foundScriptsConfigMap.Data
	}

	scriptsConfigMapHash, err := util.ObjectHash(scriptsConfigMap.Data)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("error calculating configuration hash: %v", err)
	}
	reqLogger.Info("ScriptsConfigMapHash: ", "Data Hash:", scriptsConfigMapHash)

	// TemplatesConfigMap
	templatesConfigMap := ovncontroller.TemplatesConfigMap(instance, instance.Name+"-templates")
	if err := controllerutil.SetControllerReference(instance, templatesConfigMap, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	// Check if this TemplatesConfigMap already exists
	foundTemplatesConfigMap := &corev1.ConfigMap{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: templatesConfigMap.Name, Namespace: templatesConfigMap.Namespace}, foundTemplatesConfigMap)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating a new TemplatesConfigMap", "TemplatesConfigMap.Namespace", templatesConfigMap.Namespace, "Job.Name", templatesConfigMap.Name)
		err = r.client.Create(context.TODO(), templatesConfigMap)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else if !reflect.DeepEqual(templatesConfigMap.Data, foundTemplatesConfigMap.Data) {
		reqLogger.Info("Updating TemplatesConfigMap")
		templatesConfigMap.Data = foundTemplatesConfigMap.Data
	}

	templatesConfigMapHash, err := util.ObjectHash(templatesConfigMap.Data)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("error calculating configuration hash: %v", err)
	}
	reqLogger.Info("TemplatesConfigMapHash: ", "Data Hash:", templatesConfigMapHash)

	// Define a new Daemonset object
	ds := newDaemonset(instance, instance.Name, templatesConfigMapHash, scriptsConfigMapHash)
	dsHash, err := util.ObjectHash(ds)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("error calculating configuration hash: %v", err)
	}
	reqLogger.Info("DaemonsetHash: ", "Daemonset Hash:", dsHash)

	// Set OVNController instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, ds, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Check if this Daemonset already exists
	found := &appsv1.DaemonSet{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: ds.Name, Namespace: ds.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating a new Daemonset", "Ds.Namespace", ds.Namespace, "Ds.Name", ds.Name)
		err = r.client.Create(context.TODO(), ds)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Daemonset created successfully - don't requeue
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, err
	} else {
		if instance.Status.DaemonsetHash != dsHash {
			reqLogger.Info("Daemonset Updated")
			found.Spec = ds.Spec
			err = r.client.Update(context.TODO(), found)
			if err != nil {
				return reconcile.Result{}, err
			}
			r.setDaemonsetHash(instance, dsHash)
			return reconcile.Result{RequeueAfter: time.Second}, err
		}
	}

	// Daemonset already exists - don't requeue
	reqLogger.Info("Skip reconcile: Daemonset already exists", "Ds.Namespace", found.Namespace, "Ds.Name", found.Name)
	return reconcile.Result{}, nil
}

func (r *ReconcileOVNController) setDaemonsetHash(instance *neutronv1.OVNController, hashStr string) error {
	if hashStr != instance.Status.DaemonsetHash {
		instance.Status.DaemonsetHash = hashStr
		if err := r.client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil
}

func newDaemonset(cr *neutronv1.OVNController, cmName string, templatesConfigHash string, scriptsConfigHash string) *appsv1.DaemonSet {
	var trueVar = true

	daemonSet := appsv1.DaemonSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DaemonSet",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cr.Namespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"daemonset": cr.Name + "-daemonset"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"daemonset": cr.Name + "-daemonset"},
				},
				Spec: corev1.PodSpec{
					NodeSelector:       common.GetComputeWorkerNodeSelector(cr.Spec.RoleName),
					HostNetwork:        true,
					HostPID:            true,
					DNSPolicy:          "ClusterFirstWithHostNet",
					Containers:         []corev1.Container{},
					Tolerations:        []corev1.Toleration{},
					ServiceAccountName: cr.Spec.ServiceAccount,
					PriorityClassName:  "system-node-critical",
				},
			},
		},
	}

	// add compute worker nodes tolerations
	for _, toleration := range common.GetComputeWorkerTolerations(cr.Spec.RoleName) {
		daemonSet.Spec.Template.Spec.Tolerations = append(daemonSet.Spec.Template.Spec.Tolerations, toleration)
	}

	containerSpec := corev1.Container{
		Name:  "ovn-controller",
		Image: cr.Spec.OvnControllerImage,
		Command: []string{
			"bash", "-c", "/usr/local/sbin/ovn.sh",
		},
		SecurityContext: &corev1.SecurityContext{
			Privileged: &trueVar,
		},
		Env: []corev1.EnvVar{
			{
				Name:  "TEMPLATES_CONFIG_HASH",
				Value: templatesConfigHash,
			},
			{
				Name:  "OVN_LOG_LEVEL",
				Value: cr.Spec.OvnLogLevel,
			},
			{
				Name:  "SCRIPTS_CONFIG_HASH",
				Value: scriptsConfigHash,
			},
			{
				Name:  "K8S_NODE",
				Value: cmName,
			},
		},
		VolumeMounts: []corev1.VolumeMount{},
	}
	// add common VolumeMounts
	for _, volMount := range common.GetVolumeMounts() {
		containerSpec.VolumeMounts = append(containerSpec.VolumeMounts, volMount)
	}
	// add ovncontroller specific VolumeMounts
	for _, volMount := range ovncontroller.GetVolumeMounts(cmName) {
		containerSpec.VolumeMounts = append(containerSpec.VolumeMounts, volMount)
	}

	daemonSet.Spec.Template.Spec.Containers = append(daemonSet.Spec.Template.Spec.Containers, containerSpec)

	// Volume config
	// add common Volumes
	for _, volConfig := range common.GetVolumes(cmName) {
		daemonSet.Spec.Template.Spec.Volumes = append(daemonSet.Spec.Template.Spec.Volumes, volConfig)
	}
	// add ovncontroller Volumes
	for _, volConfig := range ovncontroller.GetVolumes(cmName) {
		daemonSet.Spec.Template.Spec.Volumes = append(daemonSet.Spec.Template.Spec.Volumes, volConfig)
	}

	return &daemonSet
}
