/*
Copyright 2022.

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

package v1beta1

import (
	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	"github.com/openstack-k8s-operators/lib-common/modules/storage"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const (
	// DbSyncHash hash
	DbSyncHash = "dbsync"

	// DeploymentHash hash used to detect changes
	DeploymentHash = "deployment"

	// Container image fall-back defaults

	// NeutronAPIContainerImage is the fall-back container image for NeutronAPI
	NeutronAPIContainerImage = "quay.io/podified-antelope-centos9/openstack-neutron-server:current-podified"
)

// NeutronAPISpec defines the desired state of NeutronAPI
type NeutronAPISpec struct {
	NeutronAPISpecCore `json:",inline"`
	// +kubebuilder:validation:Required
	// NeutronAPI Container Image URL (will be set to environmental default if empty)
	ContainerImage string `json:"containerImage"`
}

// NeutronAPISpecCore -
type NeutronAPISpecCore struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=120
	// +kubebuilder:validation:Minimum=1
	// APITimeout for HAProxy, Apache
	APITimeout int `json:"apiTimeout"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=neutron
	// ServiceUser - optional username used for this service to register in neutron
	ServiceUser string `json:"serviceUser"`

	// +kubebuilder:validation:Required
	// MariaDB instance name
	// Right now required by the maridb-operator to get the credentials from the instance to create the DB
	// Might not be required in future
	DatabaseInstance string `json:"databaseInstance"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=neutron
	// DatabaseAccount - optional MariaDBAccount CR name used for neutron DB, defaults to neutron
	DatabaseAccount string `json:"databaseAccount"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default=rabbitmq
	// RabbitMQ instance name
	// Needed to request a transportURL that is created and used in Neutron
	RabbitMqClusterName string `json:"rabbitMqClusterName" deprecated:"messagingBus.cluster"`

	// +kubebuilder:validation:Optional
	// MessagingBus configuration (username, vhost, and cluster)
	MessagingBus rabbitmqv1.RabbitMqConfig `json:"messagingBus,omitempty"`

	// +kubebuilder:validation:Optional
	// NotificationsBus configuration (username, vhost, and cluster) for notifications
	NotificationsBus *rabbitmqv1.RabbitMqConfig `json:"notificationsBus,omitempty"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default=memcached
	// Memcached instance name.
	MemcachedInstance string `json:"memcachedInstance"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Maximum=32
	// +kubebuilder:validation:Minimum=0
	// Replicas of neutron API to run
	Replicas *int32 `json:"replicas"`

	// +kubebuilder:validation:Required
	// Secret containing OpenStack password information for NeutronPassword
	Secret string `json:"secret"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={service: NeutronPassword}
	// PasswordSelectors - Selectors to identify the ServiceUser password from the Secret
	PasswordSelectors PasswordSelector `json:"passwordSelectors"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes running this service
	NodeSelector *map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// PreserveJobs - do not delete jobs after they finished e.g. to check logs
	PreserveJobs bool `json:"preserveJobs"`

	// +kubebuilder:validation:Optional
	// CorePlugin - Neutron core plugin to use. Using "ml2" if not set.
	// +kubebuilder:default="ml2"
	CorePlugin string `json:"corePlugin"`

	// +kubebuilder:validation:Optional
	// Ml2MechanismDrivers - list of ml2 drivers to enable. Using {"ovn"} if not set.
	// +kubebuilder:default={"ovn"}
	Ml2MechanismDrivers []string `json:"ml2MechanismDrivers"`

	// +kubebuilder:validation:Optional
	// CustomServiceConfig - customize the service config using this parameter to change service defaults,
	// or overwrite rendered information using raw OpenStack config format. The content gets added to
	// to /etc/<service>/<service>.conf.d directory as custom.conf file.
	CustomServiceConfig string `json:"customServiceConfig,omitempty"`

	// +kubebuilder:validation:Optional
	// DefaultConfigOverwrite - interface to overwrite default config files like policy.yaml
	DefaultConfigOverwrite map[string]string `json:"defaultConfigOverwrite,omitempty"`

	// +kubebuilder:validation:Optional
	// Resources - Compute Resources required by this service (Limits/Requests).
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// +kubebuilder:validation:Optional
	// NetworkAttachments is a list of NetworkAttachment resource names to expose the services to the given network
	NetworkAttachments []string `json:"networkAttachments,omitempty"`

	// ExtraMounts containing conf files
	// +kubebuilder:validation:Optional
	ExtraMounts []NeutronExtraVolMounts `json:"extraMounts,omitempty"`

	// +kubebuilder:validation:Optional
	// Override, provides the ability to override the generated manifest of several child resources.
	Override APIOverrideSpec `json:"override,omitempty"`

	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// TLS - Parameters related to the TLS
	TLS NeutronApiTLS `json:"tls,omitempty"`

	// +kubebuilder:validation:Optional
	// TopologyRef to apply the Topology defined by the associated CR referenced
	// by name
	TopologyRef *topologyv1.TopoRef `json:"topologyRef,omitempty"`

	// +kubebuilder:validation:Optional
	// NotificationsBusInstance is the name of the RabbitMqCluster CR to select
	// the Message Bus Service instance used by the neutron to publish external notifications.
	// If undefined, the value will be inherited from OpenStackControlPlane.
	// An empty value "" leaves the notification drivers unconfigured and emitting no notifications at all.
	// Avoid colocating it with RabbitMqClusterName used for RPC.
	NotificationsBusInstance *string `json:"notificationsBusInstance,omitempty" deprecated:"notificationsBus.cluster"`
}

type NeutronApiTLS struct {
	// +kubebuilder:validation:optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// API tls type which encapsulates for API services
	API tls.APIService `json:"api,omitempty"`
	// +kubebuilder:validation:optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// Secret containing CA bundle
	tls.Ca `json:",inline"`
	// +kubebuilder:validation:optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	// Ovn GenericService - holds the secret for the OvnDb client cert
	Ovn tls.GenericService `json:"ovn,omitempty"`
}

// APIOverrideSpec to override the generated manifest of several child resources.
type APIOverrideSpec struct {
	// Override configuration for the Service created to serve traffic to the cluster.
	// The key must be the endpoint type (public, internal)
	Service map[service.Endpoint]service.RoutedOverrideSpec `json:"service,omitempty"`
}

// PasswordSelector to identify the DB and AdminUser password from the Secret
type PasswordSelector struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="NeutronPassword"
	// Database - Selector to get the neutron service password from the Secret
	Service string `json:"service"`
}

// NeutronAPIStatus defines the observed state of NeutronAPI
type NeutronAPIStatus struct {
	// ReadyCount of neutron API instances
	ReadyCount int32 `json:"readyCount,omitempty"`

	// Map of hashes to track e.g. job status
	Hash map[string]string `json:"hash,omitempty"`

	// Conditions
	Conditions condition.Conditions `json:"conditions,omitempty" optional:"true"`

	// Neutron Database Hostname
	DatabaseHostname string `json:"databaseHostname,omitempty"`

	// TransportURLSecret - Secret containing RabbitMQ transportURL
	TransportURLSecret string `json:"transportURLSecret,omitempty"`

	// NetworkAttachments status of the deployment pods
	NetworkAttachments map[string][]string `json:"networkAttachments,omitempty"`

	// ObservedGeneration - the most recent generation observed for this
	// service. If the observed generation is less than the spec generation,
	// then the controller has not processed the latest changes injected by
	// the opentack-operator in the top-level CR (e.g. the ContainerImage)
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastAppliedTopology - the last applied Topology
	LastAppliedTopology *topologyv1.TopoRef `json:"lastAppliedTopology,omitempty"`

	// NotificationsTransportURLSecret - Secret containing
	// external notifications transportURL
	NotificationsTransportURLSecret *string `json:"notificationsTransportURLSecret,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NetworkAttachments",type="string",JSONPath=".status.networkAttachments",description="NetworkAttachments"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[0].status",description="Status"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[0].message",description="Message"

// NeutronAPI is the Schema for the neutronapis API
type NeutronAPI struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NeutronAPISpec   `json:"spec,omitempty"`
	Status NeutronAPIStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NeutronAPIList contains a list of NeutronAPI
type NeutronAPIList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NeutronAPI `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NeutronAPI{}, &NeutronAPIList{})
}

// IsReady - returns true if service is ready to server requests
func (instance NeutronAPI) IsReady() bool {
	// Ready when:
	// NeutronAPI is reconciled succcessfully
	return instance.Status.Conditions.IsTrue(condition.ReadyCondition)
}

// NeutronExtraVolMounts exposes additional parameters processed by the neutron-operator
// and defines the common VolMounts structure provided by the main storage module
type NeutronExtraVolMounts struct {
	// +kubebuilder:validation:Optional
	Name string `json:"name,omitempty"`
	// +kubebuilder:validation:Optional
	Region string `json:"region,omitempty"`
	// +kubebuilder:validation:Required
	VolMounts []storage.VolMounts `json:"extraVol"`
}

// Propagate is a function used to filter VolMounts according to the specified
// PropagationType array
func (c *NeutronExtraVolMounts) Propagate(svc []storage.PropagationType) []storage.VolMounts {

	var vl []storage.VolMounts

	for _, gv := range c.VolMounts {
		vl = append(vl, gv.Propagate(svc)...)
	}

	return vl
}

// RbacConditionsSet - set the conditions for the rbac object
func (instance NeutronAPI) RbacConditionsSet(c *condition.Condition) {
	instance.Status.Conditions.Set(c)
}

// RbacNamespace - return the namespace
func (instance NeutronAPI) RbacNamespace() string {
	return instance.Namespace
}

// RbacResourceName - return the name to be used for rbac objects (serviceaccount, role, rolebinding)
func (instance NeutronAPI) RbacResourceName() string {
	return "neutron-" + instance.Name
}

func (instance NeutronAPI) IsOVNEnabled() bool {
	for _, driver := range instance.Spec.Ml2MechanismDrivers {
		// TODO: use const
		if driver == "ovn" {
			return true
		}
	}
	return false
}

// SetupDefaults - initializes any CRD field defaults based on environment variables (the defaulting mechanism itself is implemented via webhooks)
func SetupDefaults() {
	// Acquire environmental defaults and initialize Neutron defaults with them
	neutronDefaults := NeutronAPIDefaults{
		ContainerImageURL: util.GetEnvVar("RELATED_IMAGE_NEUTRON_API_IMAGE_URL_DEFAULT", NeutronAPIContainerImage),
		APITimeout:        120,
	}

	SetupNeutronAPIDefaults(neutronDefaults)
}

// ValidateTopology -
func (instance *NeutronAPISpecCore) ValidateTopology(
	basePath *field.Path,
	namespace string,
) field.ErrorList {
	var allErrs field.ErrorList
	allErrs = append(allErrs, topologyv1.ValidateTopologyRef(
		instance.TopologyRef,
		*basePath.Child("topologyRef"), namespace)...)
	return allErrs
}
