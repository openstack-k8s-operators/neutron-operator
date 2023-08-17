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
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	"github.com/openstack-k8s-operators/lib-common/modules/storage"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	// DatabaseUser - optional username used for neutron DB, defaults to neutron
	// TODO: -> implement needs work in mariadb-operator, right now only neutron
	DatabaseUser string `json:"databaseUser"`

	// +kubebuilder:validation:Required
	// +kubebuilder:default=rabbitmq
	// RabbitMQ instance name
	// Needed to request a transportURL that is created and used in Neutron
	RabbitMqClusterName string `json:"rabbitMqClusterName"`

	// +kubebuilder:validation:Required
	// NeutronAPI Container Image URL (will be set to environmental default if empty)
	ContainerImage string `json:"containerImage"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Maximum=32
	// +kubebuilder:validation:Minimum=0
	// Replicas of neutron API to run
	Replicas *int32 `json:"replicas"`

	// +kubebuilder:validation:Required
	// Secret containing OpenStack password information for NeutronDatabasePassword, NeutronPassword
	Secret string `json:"secret"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default={database: NeutronDatabasePassword, service: NeutronPassword}
	// PasswordSelectors - Selectors to identify the DB and ServiceUser password from the Secret
	PasswordSelectors PasswordSelector `json:"passwordSelectors"`

	// +kubebuilder:validation:Optional
	// NodeSelector to target subset of worker nodes running this service
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// +kubebuilder:validation:Optional
	// Debug - enable debug for different deploy stages. If an init container is used, it runs and the
	// actual action pod gets started with sleep infinity
	Debug NeutronAPIDebug `json:"debug,omitempty"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// PreserveJobs - do not delete jobs after they finished e.g. to check logs
	PreserveJobs bool `json:"preserveJobs"`

	// +kubebuilder:validation:Optional
	// CustomServiceConfig - customize the service config using this parameter to change service defaults,
	// or overwrite rendered information using raw OpenStack config format. The content gets added to
	// to /etc/<service>/<service>.conf.d directory as custom.conf file.
	CustomServiceConfig string `json:"customServiceConfig,omitempty"`

	// +kubebuilder:validation:Optional
	// ConfigOverwrite - interface to overwrite default config files like e.g. logging.conf or policy.json.
	// But can also be used to add additional files. Those get added to the service config dir in /etc/<service> .
	// TODO: -> implement
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
}

// APIOverrideSpec to override the generated manifest of several child resources.
type APIOverrideSpec struct {
	// Override configuration for the Service created to serve traffic to the cluster.
	// The key must be the endpoint type (public, internal)
	Service map[string]service.OverrideSpec `json:"service,omitempty"`
}

// PasswordSelector to identify the DB and AdminUser password from the Secret
type PasswordSelector struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="NeutronDatabasePassword"
	// Database - Selector to get the neutron database user password from the Secret
	// TODO: not used, need change in mariadb-operator
	Database string `json:"database"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="NeutronPassword"
	// Database - Selector to get the neutron service password from the Secret
	Service string `json:"service"`
}

// NeutronAPIDebug defines the observed state of NeutronAPIDebug
type NeutronAPIDebug struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// DBSync enable debug
	DBSync bool `json:"dbSync"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// Bootstrap enable debug
	Bootstrap bool `json:"bootstrap"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	// Service enable debug
	Service bool `json:"service"`
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
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NetworkAttachments",type="string",JSONPath=".spec.networkAttachments",description="NetworkAttachments"
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

// SetupDefaults - initializes any CRD field defaults based on environment variables (the defaulting mechanism itself is implemented via webhooks)
func SetupDefaults() {
	// Acquire environmental defaults and initialize Neutron defaults with them
	neutronDefaults := NeutronAPIDefaults{
		ContainerImageURL: util.GetEnvVar("NEUTRON_API_IMAGE_URL_DEFAULT", NeutronAPIContainerImage),
	}

	SetupNeutronAPIDefaults(neutronDefaults)
}
