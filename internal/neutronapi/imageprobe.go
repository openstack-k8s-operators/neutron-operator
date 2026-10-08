package neutronapi

import (
	"context"
	"fmt"
	"time"

	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/job"
	"github.com/openstack-k8s-operators/lib-common/modules/common/pod"
	"github.com/openstack-k8s-operators/lib-common/modules/users"
	neutronv1beta1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

// TEMPORARY (OSPRH-33113): the wsgi annotation (NeutronWSGILabel) now
// defaults to true on the openstack-operator side ahead of the separate,
// global rollout of WSGI-only (Eventlet-removed) images across every
// openstack-k8s-operators default image pin. Until that rollout completes,
// some deployments may still resolve ContainerImage to a pre-WSGI image. In
// that case IsWSGIEffective below falls back to the Eventlet strategy
// regardless of what the annotation says, by inspecting the resolved image
// for NeutronServerBinaryPath via ImageProbeJob.
//
// Remove this whole file and its two call sites (reverting them to
// instance.IsWSGI()) once WSGI-only images are the default everywhere.

// NeutronServerBinaryPath is only present in pre-WSGI (Eventlet-capable)
// Neutron images. Its absence indicates a WSGI-only (Eventlet-removed) image.
const NeutronServerBinaryPath = "/usr/bin/neutron-server"

// ImageProbeCommand exits 0 if NeutronServerBinaryPath is absent (a
// WSGI-only image) and non-zero if it is present (a pre-WSGI,
// Eventlet-capable image), so the result can be read straight off the Job's
// Succeeded/Failed status. The check is inverted relative to the path it
// tests so that the now-common case -- WSGI-only images -- produces a
// Succeeded Job instead of a Failed one that keeps hitting BackoffLimit on
// every re-run of this Job.
var ImageProbeCommand = fmt.Sprintf("test ! -f %s", NeutronServerBinaryPath)

// imageProbeHash is only used as the job.NewJob jobType for log messages.
const imageProbeHash = "imageprobe"

// ImageProbeJobName returns the name of the ImageProbeJob for cr.
func ImageProbeJobName(cr *neutronv1beta1.NeutronAPI) string {
	return cr.Name + "-image-probe"
}

// ImageProbeJob builds the Job used to detect whether cr.Spec.ContainerImage
// is a pre-WSGI (Eventlet-capable) image, by checking for the presence of
// NeutronServerBinaryPath.
func ImageProbeJob(
	cr *neutronv1beta1.NeutronAPI,
	labels map[string]string,
	annotations map[string]string,
) *batchv1.Job {
	name := ImageProbeJobName(cr)

	probeJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   cr.Namespace,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: batchv1.JobSpec{
			// A single attempt is enough: the probe command is a
			// deterministic file check, not something that benefits from
			// retries.
			BackoffLimit: ptr.To(int32(0)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:                corev1.RestartPolicyNever,
					ServiceAccountName:           cr.RbacResourceName(),
					AutomountServiceAccountToken: ptr.To(false),
					SecurityContext:              pod.RestrictivePodSecurityContext(users.NeutronUID, users.NeutronGID),
					Containers: []corev1.Container{
						{
							Name:            name,
							Command:         []string{"/bin/bash"},
							Args:            []string{"-c", ImageProbeCommand},
							Image:           cr.Spec.ContainerImage,
							SecurityContext: pod.RestrictiveSecurityContext(users.NeutronUID, users.NeutronGID),
						},
					},
				},
			},
		},
	}

	if cr.Spec.NodeSelector != nil {
		probeJob.Spec.Template.Spec.NodeSelector = *cr.Spec.NodeSelector
	}

	return probeJob
}

// IsWSGIEffective returns the deployment strategy to actually use, layering
// a runtime safety net on top of instance.IsWSGI(): if the wsgi annotation
// says wsgi, it fires off (but does not block reconcile on) ImageProbeJob
// and falls back to the Eventlet strategy once that Job finds
// NeutronServerBinaryPath in the resolved ContainerImage.
//
// The probe result is read straight off the Job's own status rather than
// being persisted on the NeutronAPI. Unlike most job.DoJob callers, the Job
// is only (re)created when it does not exist at all: once it exists --
// Succeeded, Failed, or still running -- its result/preserved state is
// authoritative and DoJob is not called again, so a stale hash can't make
// DoJob repeatedly recreate it. The Job is also always preserved (no TTL),
// so it isn't garbage-collected and then re-run by the next reconcile. If
// instance.Spec.ContainerImage no longer matches the image the existing Job
// was probing, the stale Job is deleted so a later reconcile re-probes the
// new image. Until the Job completes, this just keeps returning
// instance.IsWSGI().
func IsWSGIEffective(
	ctx context.Context,
	h *helper.Helper,
	instance *neutronv1beta1.NeutronAPI,
	labels map[string]string,
	annotations map[string]string,
) bool {
	if !instance.IsWSGI() {
		return false
	}

	probeJobDef := ImageProbeJob(instance, labels, annotations)

	existingProbeJob := &batchv1.Job{}
	err := h.GetClient().Get(ctx, types.NamespacedName{Name: probeJobDef.Name, Namespace: instance.Namespace}, existingProbeJob)

	if err == nil && len(existingProbeJob.Spec.Template.Spec.Containers) > 0 &&
		existingProbeJob.Spec.Template.Spec.Containers[0].Image != instance.Spec.ContainerImage {
		// ContainerImage changed since this Job was created/preserved: its
		// result is for a different image and no longer applies. Delete it
		// so a later reconcile (once the delete has actually completed)
		// recreates and re-probes it against the new image.
		h.GetLogger().Info(fmt.Sprintf("Image probe Job %s: ContainerImage changed, deleting stale Job", probeJobDef.Name))
		if delErr := job.DeleteJob(ctx, h, existingProbeJob.Name, existingProbeJob.Namespace); delErr != nil {
			h.GetLogger().Info(fmt.Sprintf("Image probe Job %s: failed to delete stale Job: %v", probeJobDef.Name, delErr))
		}
		return true
	}

	switch {
	case err == nil && existingProbeJob.Status.Succeeded > 0:
		// NeutronServerBinaryPath was not found: a WSGI-only image, the
		// common case as the global rollout progresses.
		return true
	case err == nil && existingProbeJob.Status.Failed > 0:
		// NeutronServerBinaryPath was found: a pre-WSGI image, the
		// shrinking legacy case. Fall back to the Eventlet strategy.
		return false
	case err == nil:
		// Job exists but hasn't finished yet.
		return true
	case !k8s_errors.IsNotFound(err):
		h.GetLogger().Info(fmt.Sprintf("Image probe Job %s: error getting Job: %v", probeJobDef.Name, err))
		return true
	}

	// Job doesn't exist yet: fire it off without blocking reconcile on it.
	// preserve is hardcoded true (regardless of instance.Spec.PreserveJobs)
	// so the Job is never TTL-deleted and thus never re-run once it has a
	// result.
	probeJob := job.NewJob(probeJobDef, imageProbeHash, true, time.Duration(5)*time.Second, "")
	if _, err := probeJob.DoJob(ctx, h); err != nil {
		h.GetLogger().Info(fmt.Sprintf("Image probe Job %s: %v", probeJobDef.Name, err))
	}

	return true
}
