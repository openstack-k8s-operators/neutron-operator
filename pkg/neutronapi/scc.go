package neutronapi

import corev1 "k8s.io/api/core/v1"

func getNeutronSecurityContext() *corev1.SecurityContext {
	trueVal := true
	runAsUser := int64(NeutronUid)
	runAsGroup := int64(NeutronGid)

	return &corev1.SecurityContext{
		RunAsUser:    &runAsUser,
		RunAsGroup:   &runAsGroup,
		RunAsNonRoot: &trueVal,
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{
				"MKNOD",
			},
		},
	}
}

func getNeutronHttpdSecurityContext() *corev1.SecurityContext {
	runAsUser := int64(0)

	return &corev1.SecurityContext{
		RunAsUser: &runAsUser,
	}
}
