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

// NeutronSriovAgentSpec defines the desired state of NeutronSriovAgent
type NeutronSriovAgentSpec struct {
	// Label is the value of the 'daemon=' label to set on a node that should run the daemon
	Label string `json:"label"`
	// Image is the Docker image to run for the daemon
	NeutronSriovImage string `json:"neutronSriovImage"`
	// RabbitMQ transport URL String
	RabbitTransportURL string `json:"rabbitTransportURL"`
	// Debug
	Debug string `json:"debug,omitempty"`
}

// NeutronSriovAgentStatus defines the observed state of NeutronSriovAgent
type NeutronSriovAgentStatus struct {
	// Count is the number of nodes the daemon is deployed to
	Count int32 `json:"count"`
	// Daemonset hash used to detect changes
	DaemonsetHash string `json:"daemonsetHash"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NeutronSriovAgent is the Schema for the neutronsriovagents API
// +operator-sdk:csv:customresourcedefinitions:displayName="Neutron Sriov Agent",resources={{NeutronSriovAgent,v1,neutronsriovagents.neutron.openstack.org}}
type NeutronSriovAgent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NeutronSriovAgentSpec   `json:"spec,omitempty"`
	Status NeutronSriovAgentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NeutronSriovAgentList contains a list of NeutronSriovAgent
type NeutronSriovAgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NeutronSriovAgent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NeutronSriovAgent{}, &NeutronSriovAgentList{})
}
