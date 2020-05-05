package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NeutronOvsAgentSpec defines the desired state of NeutronOvsAgent
// +k8s:openapi-gen=true
type NeutronOvsAgentSpec struct {
        // Label is the value of the 'daemon=' label to set on a node that should run the daemon
        Label string `json:"label"`
        // Image is the Docker image to run for the daemon
        OpenvswitchImage string `json:"openvswitchImage"`
        // RabbitMQ transport URL String
        RabbitTransportURL string `json:"rabbitTransportURL"`
        // Debug
        Debug string `json:"debug,omitempty"`
}

// NeutronOvsAgentStatus defines the observed state of NeutronOvsAgent
// +k8s:openapi-gen=true
type NeutronOvsAgentStatus struct {
        // Count is the number of nodes the daemon is deployed to
        Count int32 `json:"count"`
        // Daemonset hash used to detect changes
        DaemonsetHash string `json:"daemonsetHash"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NeutronOvsAgent is the Schema for the neutronovsagents API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type NeutronOvsAgent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NeutronOvsAgentSpec   `json:"spec,omitempty"`
	Status NeutronOvsAgentStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NeutronOvsAgentList contains a list of NeutronOvsAgent
type NeutronOvsAgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NeutronOvsAgent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NeutronOvsAgent{}, &NeutronOvsAgentList{})
}
