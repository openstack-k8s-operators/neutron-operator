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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// OVSNodeOspSpec defines the desired state of OVSNodeOsp
type OVSNodeOspSpec struct {
	// container image to run for the daemon
	OvsNodeOspImage string `json:"ovsNodeOspImage"`
	// service account used to create pods
	ServiceAccount string `json:"serviceAccount"`
	// Name of the worker role created for OSP computes
	RoleName string `json:"roleName"`
	// log level
	OvsLogLevel string `json:"ovsLogLevel"`
	// NIC for ovn encap ip
	Nic string `json:"nic"`
	// Make the nodes a Network Gateways Node
	Gateway bool `json:"gateway,omitempty"`
	// Bridge Mappings
	BridgeMappings string `json:"bridgeMappings,omitempty"`
}

// OVSNodeOspStatus defines the observed state of OVSNodeOsp
type OVSNodeOspStatus struct {
	// Count is the number of nodes the daemon is deployed to
	Count int32 `json:"count"`
	// Daemonset hash used to detect changes
	DaemonsetHash string `json:"daemonsetHash"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// OVSNodeOsp is the Schema for the ovsnodeosps API
type OVSNodeOsp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OVSNodeOspSpec   `json:"spec,omitempty"`
	Status OVSNodeOspStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OVSNodeOspList contains a list of OVSNodeOsp
type OVSNodeOspList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OVSNodeOsp `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OVSNodeOsp{}, &OVSNodeOspList{})
}
