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

// OVNControllerSpec defines the desired state of OVNController
type OVNControllerSpec struct {
	// container image to run for the daemon
	OvnControllerImage string `json:"ovnControllerImage"`
	// service account used to create pods
	ServiceAccount string `json:"serviceAccount"`
	// Name of the worker role created for OSP computes
	RoleName string `json:"roleName"`
	// log level
	OvnLogLevel string `json:"ovnLogLevel"`
}

// OVNControllerStatus defines the observed state of OVNController
type OVNControllerStatus struct {
	// Count is the number of nodes the daemon is deployed to
	Count int32 `json:"count"`
	// Daemonset hash used to detect changes
	DaemonsetHash string `json:"daemonsetHash"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// OVNController is the Schema for the ovncontrollers API
// +kubebuilder:resource:path=ovncontrollers,scope=Namespaced
type OVNController struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OVNControllerSpec   `json:"spec,omitempty"`
	Status OVNControllerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OVNControllerList contains a list of OVNController
type OVNControllerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OVNController `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OVNController{}, &OVNControllerList{})
}
