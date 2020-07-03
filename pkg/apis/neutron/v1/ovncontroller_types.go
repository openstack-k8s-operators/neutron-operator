package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

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

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// OVNController is the Schema for the ovncontrollers API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=ovncontrollers,scope=Namespaced
type OVNController struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OVNControllerSpec   `json:"spec,omitempty"`
	Status OVNControllerStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// OVNControllerList contains a list of OVNController
type OVNControllerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OVNController `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OVNController{}, &OVNControllerList{})
}
