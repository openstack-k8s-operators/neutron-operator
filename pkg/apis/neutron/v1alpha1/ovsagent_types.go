package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// OvsAgentSpec defines the desired state of OvsAgent
// +k8s:openapi-gen=true
type OvsAgentSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book.kubebuilder.io/beyond_basics/generating_crd.html

        // Label is the value of the 'daemon=' label to set on a node that should run the daemon
        Label string `json:"label"`

        // Image is the Docker image to run for the daemon
        OpenvswitchImage string `json:"openvswitchImage"`
}

// OvsAgentStatus defines the observed state of OvsAgent
// +k8s:openapi-gen=true
type OvsAgentStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book.kubebuilder.io/beyond_basics/generating_crd.html

        // Count is the number of nodes the daemon is deployed to
        Count int32 `json:"count"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// OvsAgent is the Schema for the ovsagents API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type OvsAgent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OvsAgentSpec   `json:"spec,omitempty"`
	Status OvsAgentStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// OvsAgentList contains a list of OvsAgent
type OvsAgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OvsAgent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OvsAgent{}, &OvsAgentList{})
}
