package common

import (
	corev1 "k8s.io/api/core/v1"
)

// GetComputeWorkerTolerations ...
func GetComputeWorkerTolerations(roleName string) []corev1.Toleration {

	return []corev1.Toleration{
		// Add toleration
		{
			Operator: corev1.TolerationOpExists,
		},
	}
}
