package ovnmetadataagent

import (
	"strings"

	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type neutronComputeConfigOptions struct {
	RabbitTransportURL               string
	NovaMetadataInternal             string
	NeutronMetadataProxySharedSecret string
	Debug							 string
	OvnSbRemote                      string

}


// TemplatesConfigMap - mandatory settings config map
func TemplatesConfigMap(cr *novav1.NovaCompute, commonConfigMap *corev1.ConfigMap, ospSecrets *corev1.Secret, cmName string) *corev1.ConfigMap {
	opts := neutronComputeConfigOptions{
		string(ospSecrets.Data["RabbitTransportURL"]),
		string(ospSecrets.Data["NovaMetadataInternal"]),
		string(ospSecrets.Data["NeutronMetadataProxySharedSecret"]),
		commonConfigMap.Data["Debug"],
		commonConfigMap.Data["OvnSbRemote"],
	}

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
			"config.json": util.ExecuteTemplateFile(strings.ToLower(cr.Kind)+"/kolla_config.json", &opts),
			"neutron.conf":    util.ExecuteTemplateFile(strings.ToLower(cr.Kind)+"/config/neutron.conf", &opts),
			"networking-ovn-metadata-agent.ini":    util.ExecuteTemplateFile(strings.ToLower(cr.Kind)+"/etc/neutron/plugins/networking-ovn/networking-ovn-metadata-agent.ini", &opts),
		},
	}

	return cm
}
