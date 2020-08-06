package ovsnodeosp

import (
	"strings"

	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	neutronv1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScriptsConfigMap - scripts config map
func ScriptsConfigMap(cr *neutronv1.OVSNodeOsp, cmName string) *corev1.ConfigMap {

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
			"ovsnode.sh": util.ExecuteTemplateFile(strings.ToLower(cr.Kind)+"/ovsnode.sh", nil),
		},
	}

	return cm
}

// TemplatesConfigMap - custom neutron config map
func TemplatesConfigMap(cr *neutronv1.OVSNodeOsp, cmName string) *corev1.ConfigMap {

	// (TODO)(ksambor) move neutron.conf here and use it in ovsnode.sh
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
			"temp": "temp",
		},
	}

	return cm
}
