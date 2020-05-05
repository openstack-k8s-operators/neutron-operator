package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NeutronSriovAgentSpec defines the desired state of NeutronSriovAgent
// +k8s:openapi-gen=true
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
// +k8s:openapi-gen=true
type NeutronSriovAgentStatus struct {
        // Count is the number of nodes the daemon is deployed to
        Count int32 `json:"count"`
        // Daemonset hash used to detect changes
        DaemonsetHash string `json:"daemonsetHash"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NeutronSriovAgent is the Schema for the neutronsriovagents API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type NeutronSriovAgent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NeutronSriovAgentSpec   `json:"spec,omitempty"`
	Status NeutronSriovAgentStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NeutronSriovAgentList contains a list of NeutronSriovAgent
type NeutronSriovAgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NeutronSriovAgent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NeutronSriovAgent{}, &NeutronSriovAgentList{})
}
