package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// OVNMetadataAgentSpec defines the desired state of OVNMetadataAgent
type OVNMetadataAgentSpec struct {
	// name of configmap which holds general information on the OSP env
	CommonConfigMap string `json:"commonConfigMap"`
	// name of secret which holds sensitive information on the OSP env
	OspSecrets string `json:"ospSecrets"`
	// container image to run for the daemon
	OVNMetadataAgentImage string `json:"ovnMetadataAgentImage"`
	// service account used to create pods
	ServiceAccount string `json:"serviceAccount"`
	// Name of the worker role created for OSP computes
	RoleName string `json:"roleName"`
}

// OVNMetadataAgentStatus defines the observed state of OVNMetadataAgent
type OVNMetadataAgentStatus struct {
	// Count is the number of nodes the daemon is deployed to
	Count int32 `json:"count"`
	// Daemonset hash used to detect changes
	DaemonsetHash string `json:"daemonsetHash"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// OVNMetadataAgent is the Schema for the ovnmetadataagents API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=ovnmetadataagents,scope=Namespaced
type OVNMetadataAgent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OVNMetadataAgentSpec   `json:"spec,omitempty"`
	Status OVNMetadataAgentStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// OVNMetadataAgentList contains a list of OVNMetadataAgent
type OVNMetadataAgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OVNMetadataAgent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OVNMetadataAgent{}, &OVNMetadataAgentList{})
}
