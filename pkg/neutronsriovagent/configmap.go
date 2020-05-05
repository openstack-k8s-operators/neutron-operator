package neutronsriovagent

import (
	neutronv1 "github.com/openstack-k8s-operators/neutron-operator/pkg/apis/neutron/v1"
	util "github.com/openstack-k8s-operators/neutron-operator/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type neutronSriovAgentConfigOptions struct {
	RabbitTransportURL string
	Debug              string
}

// ConfigMap - custom config map
func ConfigMap(cr *neutronv1.NeutronSriovAgent, cmName string) *corev1.ConfigMap {
	opts := neutronSriovAgentConfigOptions{cr.Spec.RabbitTransportURL,
		cr.Spec.Debug}

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cr.Namespace,
		},
		Data: map[string]string{
			"neutron.conf":    util.ExecuteTemplateFile("neutron.conf", &opts),
			"sriov_agent.ini": util.ExecuteTemplateFile("sriov_agent.ini", nil),
		},
	}

	return cm
}
