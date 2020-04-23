package neutronovsagent

import (
	neutronv1 "github.com/openstack-k8s-operators/neutron-operator/pkg/apis/neutron/v1"
	util "github.com/openstack-k8s-operators/neutron-operator/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type neutronOvsAgentConfigOptions struct {
	RabbitTransportUrl string
	Debug              string
}

// neutron-ovsagent-init config map
func InitConfigMap(cr *neutronv1.NeutronOvsAgent, cmName string) *corev1.ConfigMap {

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
			"common.sh":                 util.ExecuteTemplateFile("common.sh", nil),
			"openvswitch_agent_init.sh": util.ExecuteTemplateFile("openvswitch_agent_init.sh", nil),
		},
	}

	return cm
}

// neutron-ovsagent config map
func ConfigMap(cr *neutronv1.NeutronOvsAgent, cmName string) *corev1.ConfigMap {
	opts := neutronOvsAgentConfigOptions{cr.Spec.RabbitTransportUrl,
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
			"neutron.conf":          util.ExecuteTemplateFile("neutron.conf", &opts),
			"openvswitch_agent.ini": util.ExecuteTemplateFile("openvswitch_agent.ini", nil),
		},
	}

	return cm
}
