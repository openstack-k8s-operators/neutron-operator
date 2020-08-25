package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OVSNodeOspSpec defines the desired state of OVSNodeOsp
// +k8s:openapi-gen=true
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
}

// OVSNodeOspStatus defines the observed state of OVSNodeOsp
// +k8s:openapi-gen=true
type OVSNodeOspStatus struct {
	// Count is the number of nodes the daemon is deployed to
	Count int32 `json:"count"`
	// Daemonset hash used to detect changes
	DaemonsetHash string `json:"daemonsetHash"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// OVSNodeOsp is the Schema for the ovsnodeosps API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type OVSNodeOsp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OVSNodeOspSpec   `json:"spec,omitempty"`
	Status OVSNodeOspStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// OVSNodeOspList contains a list of OVSNodeOsp
type OVSNodeOspList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OVSNodeOsp `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OVSNodeOsp{}, &OVSNodeOspList{})
}
