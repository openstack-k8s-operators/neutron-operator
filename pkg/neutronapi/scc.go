package neutronapi

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

func getNeutronSecurityContext() *corev1.SecurityContext {

	return &corev1.SecurityContext{
		RunAsUser:    ptr.To(NeutronUID),
		RunAsGroup:   ptr.To(NeutronGID),
		RunAsNonRoot: ptr.To(true),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{
				"MKNOD",
			},
		},
	}
}
