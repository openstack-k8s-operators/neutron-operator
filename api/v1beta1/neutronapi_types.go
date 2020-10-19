/*


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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Hash - struct to add hashes to status
type Hash struct {
	// Name of hash referencing the parameter
	Name string `json:"name,omitempty"`
	// Hash
	Hash string `json:"hash,omitempty"`
}

// NeutronAPISpec defines the desired state of NeutronAPI
type NeutronAPISpec struct {
	// Neutron Database Hostname String
	DatabaseHostname string `json:"databaseHostname,omitempty"`
	// Neutron API Container Image URL
	ContainerImage string `json:"containerImage,omitempty"`
	// Cinder API Replicas
	Replicas int32 `json:"replicas"`
	// Secret containing: NeutronPassword
	NeutronSecret string `json:"neutronSecret,omitempty"`
	// Secret containing: NovaPassword
	NovaSecret string `json:"novaSecret,omitempty"`
	// ovn-connection configmap which holds NBConnection and SBConnection string
	OVNConnectionConfigMap string `json:"ovnConnectionConfigMap,omitempty"`
}

// NeutronAPIStatus defines the observed state of NeutronAPI
type NeutronAPIStatus struct {
	// hashes of Secrets, CMs
	Hashes []Hash `json:"hashes,omitempty"`
	// DbSyncHash db sync hash
	DbSyncHash string `json:"dbSyncHash"`
	// API endpoint
	APIEndpoint string `json:"apiEndpoint"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

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
