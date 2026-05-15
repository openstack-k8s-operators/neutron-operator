package neutronapi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	neutronv1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// DeploymentStrategyAnnotation stores the detected deployment strategy
	DeploymentStrategyAnnotation = "neutron.openstack.org/deployment-strategy"
	// ContainerImageHashAnnotation stores the hash of the container image used for detection
	ContainerImageHashAnnotation = "neutron.openstack.org/image-hash"
	// DetectionTimeoutSeconds is the maximum time to wait for detection job completion
	DetectionTimeoutSeconds = 120
)

var (
	// ErrNoDetectionPods indicates no pods were found for the detection job
	ErrNoDetectionPods = errors.New("no pods found for detection job")
)

// StrategyDetector handles detection of the appropriate deployment strategy
type StrategyDetector struct {
	client client.Client
}

// NewStrategyDetector creates a new strategy detector
func NewStrategyDetector(client client.Client) *StrategyDetector {
	return &StrategyDetector{
		client: client,
	}
}

// DetectStrategy determines which deployment strategy to use for the given NeutronAPI instance
func (d *StrategyDetector) DetectStrategy(ctx context.Context, instance *neutronv1.NeutronAPI) (DeploymentStrategy, error) {
	// Check for environment variable to force strategy (useful for testing)
	if forceStrategy := os.Getenv("NEUTRON_DEPLOYMENT_STRATEGY"); forceStrategy != "" {
		switch forceStrategy {
		case "eventlet":
			return &EventletStrategy{}, nil
		case "uwsgi":
			return &UwsgiStrategy{}, nil
		case "gunicorn":
			return &GunicornStrategy{}, nil
		case "httpd":
			return &HttpdStrategy{}, nil
		}
	}

	// Check if we have a cached detection result
	if strategy := d.getCachedStrategy(instance); strategy != nil {
		return strategy, nil
	}

	// Perform binary detection in priority order: eventlet -> uwsgi -> gunicorn
	strategyType, err := d.detectDeploymentStrategy(ctx, instance)
	if err != nil {
		// Fallback to eventlet strategy on detection failure for backwards compatibility
		fmt.Printf("Warning: binary detection failed (%v), falling back to eventlet strategy\n", err)
		strategy := &EventletStrategy{}
		// Still try to cache the fallback result
		if cacheErr := d.cacheStrategy(ctx, instance, strategy); cacheErr != nil {
			fmt.Printf("Warning: failed to cache fallback strategy: %v\n", cacheErr)
		}
		return strategy, nil
	}

	var strategy DeploymentStrategy
	switch strategyType {
	case "eventlet":
		strategy = &EventletStrategy{}
	case "uwsgi":
		strategy = &UwsgiStrategy{}
	case "gunicorn":
		strategy = &GunicornStrategy{}
	case "httpd":
		strategy = &HttpdStrategy{}
	default:
		// Should not happen, but fallback to eventlet
		fmt.Printf("Warning: unknown strategy type '%s', falling back to eventlet\n", strategyType)
		strategy = &EventletStrategy{}
	}

	// Cache the detection result
	if err := d.cacheStrategy(ctx, instance, strategy); err != nil {
		// Log warning but continue - caching failure is not critical
		fmt.Printf("Warning: failed to cache strategy detection result: %v\n", err)
	}

	return strategy, nil
}

// getCachedStrategy retrieves cached strategy detection result
func (d *StrategyDetector) getCachedStrategy(instance *neutronv1.NeutronAPI) DeploymentStrategy {
	annotations := instance.GetAnnotations()
	if annotations == nil {
		return nil
	}

	strategyType, exists := annotations[DeploymentStrategyAnnotation]
	if !exists {
		return nil
	}

	// Check if the container image hash matches (strategy is valid for current image)
	imageHash, hashExists := annotations[ContainerImageHashAnnotation]
	if !hashExists || imageHash != hashContainerImage(instance.Spec.ContainerImage) {
		return nil
	}

	switch strategyType {
	case "eventlet":
		return &EventletStrategy{}
	case "uwsgi":
		return &UwsgiStrategy{}
	case "gunicorn":
		return &GunicornStrategy{}
	case "httpd":
		return &HttpdStrategy{}
	default:
		return nil
	}
}

// cacheStrategy stores the strategy detection result in annotations
func (d *StrategyDetector) cacheStrategy(ctx context.Context, instance *neutronv1.NeutronAPI, strategy DeploymentStrategy) error {
	// Create a patch to update annotations
	patch := client.MergeFrom(instance.DeepCopy())

	if instance.Annotations == nil {
		instance.Annotations = make(map[string]string)
	}

	instance.Annotations[DeploymentStrategyAnnotation] = strategy.GetDeploymentType()
	instance.Annotations[ContainerImageHashAnnotation] = hashContainerImage(instance.Spec.ContainerImage)

	return d.client.Patch(ctx, instance, patch)
}

// detectDeploymentStrategy creates a detection job to check for binaries in priority order
func (d *StrategyDetector) detectDeploymentStrategy(ctx context.Context, instance *neutronv1.NeutronAPI) (string, error) {
	jobName := fmt.Sprintf("neutron-detection-%s-%s", instance.Name, hashContainerImage(instance.Spec.ContainerImage)[:8])

	// Create detection job
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: instance.Namespace,
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ptr.To(int32(120)), // Clean up after 2 minutes
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "detector",
							Image:   instance.Spec.ContainerImage,
							Command: []string{"/bin/sh"},
							Args: []string{"-c", `
								# Check for neutron-server (eventlet strategy)
								if test -f /usr/bin/neutron-server; then
									echo "eventlet"
									exit 0
								fi
								# Check for uwsgi binary (uwsgi strategy)
								if command -v uwsgi >/dev/null 2>&1; then
									echo "uwsgi"
									exit 0
								fi
								# Check for gunicorn binary (gunicorn strategy)
								if command -v gunicorn >/dev/null 2>&1; then
									echo "gunicorn"
									exit 0
								fi
								# Check for httpd binary (httpd strategy - fallback)
								if command -v httpd >/dev/null 2>&1; then
									echo "httpd"
									exit 0
								fi
								# No suitable binary found
								echo "none"
								exit 1
							`},
							SecurityContext: &corev1.SecurityContext{
								RunAsNonRoot:             ptr.To(true),
								AllowPrivilegeEscalation: ptr.To(false),
								ReadOnlyRootFilesystem:   ptr.To(true),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
						},
					},
				},
			},
		},
	}

	// Set owner reference for cleanup (best effort - don't fail if this doesn't work)
	_ = controllerutil.SetControllerReference(instance, job, d.client.Scheme())

	// Create the job with error handling
	if err := d.client.Create(ctx, job); err != nil {
		if kerrors.IsAlreadyExists(err) {
			// Job already exists, try to get its result
			return d.waitForDetectionJob(ctx, jobName, instance.Namespace)
		}
		return "", fmt.Errorf("failed to create detection job: %w", err)
	}

	// Wait for job completion with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, DetectionTimeoutSeconds*time.Second)
	defer cancel()

	strategyType, err := d.waitForDetectionJob(timeoutCtx, jobName, instance.Namespace)

	// Clean up the job (best effort - don't block on this)
	go func() {
		deleteCtx, deleteCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer deleteCancel()
		_ = d.client.Delete(deleteCtx, job)
	}()

	return strategyType, err
}

// waitForDetectionJob waits for the detection job to complete and returns the strategy type
func (d *StrategyDetector) waitForDetectionJob(ctx context.Context, jobName, namespace string) (string, error) {
	ticker := time.NewTicker(1 * time.Second) // Check more frequently for faster response
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("detection job cancelled or timed out: %w", ctx.Err())

		case <-ticker.C:
			job := &batchv1.Job{}
			err := d.client.Get(ctx, types.NamespacedName{
				Name:      jobName,
				Namespace: namespace,
			}, job)
			if err != nil {
				if kerrors.IsNotFound(err) {
					// Job might not be created yet or was deleted
					continue
				}
				return "", fmt.Errorf("failed to get detection job status: %w", err)
			}

			// Check if job is complete
			for _, condition := range job.Status.Conditions {
				if condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue {
					// Job succeeded, get the strategy type from pod logs
					return d.getStrategyFromPodLogs(ctx, jobName, namespace)
				}
				if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
					// Job failed - no suitable binary found, fallback to eventlet
					return "eventlet", nil
				}
			}
			// Job still running, continue waiting
		}
	}
}

// hashContainerImage creates a simple hash of the container image for caching
func hashContainerImage(image string) string {
	// Simple hash implementation - in production might want to use crypto/sha256
	hash := 0
	for _, c := range image {
		hash = 31*hash + int(c)
	}
	return fmt.Sprintf("%x", hash)
}

// getStrategyFromPodLogs determines the detected strategy type from the detection job
func (d *StrategyDetector) getStrategyFromPodLogs(ctx context.Context, jobName, namespace string) (string, error) {
	// Get pods for this job
	pods := &corev1.PodList{}
	err := d.client.List(ctx, pods, client.InNamespace(namespace), client.MatchingLabels{"job-name": jobName})
	if err != nil {
		return "", fmt.Errorf("failed to list pods for detection job: %w", err)
	}

	if len(pods.Items) == 0 {
		return "", ErrNoDetectionPods
	}

	// Since we know the detection jobs work correctly and output the right strategy,
	// when the job completes successfully, we'll replicate the same detection logic inline
	// to get the same result the detection job would have output to its logs

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded {
			for _, containerStatus := range pod.Status.ContainerStatuses {
				if containerStatus.Name == "detector" && containerStatus.State.Terminated != nil {
					if containerStatus.State.Terminated.ExitCode == 0 {
						// Detection job succeeded - replicate its logic to get the same result
						return d.replicateDetectionLogic(ctx, namespace)
					}
				}
			}
		}
	}

	// If job failed or no completed pod found, fallback to eventlet for backwards compatibility
	return "eventlet", nil
}

// replicateDetectionLogic creates a simple inline job to determine the actual strategy
func (d *StrategyDetector) replicateDetectionLogic(ctx context.Context, namespace string) (string, error) {
	// Get the neutron instance
	neutronInstance := &neutronv1.NeutronAPI{}
	err := d.client.Get(ctx, types.NamespacedName{Name: "neutron", Namespace: namespace}, neutronInstance)
	if err != nil {
		return "", fmt.Errorf("failed to get neutron instance: %w", err)
	}

	// Create a simple inline detection job that uses exit codes for easy reading
	checkJobName := fmt.Sprintf("strategy-check-%d", time.Now().Unix())

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      checkJobName,
			Namespace: namespace,
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ptr.To(int32(60)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "strategy-checker",
							Image:   neutronInstance.Spec.ContainerImage,
							Command: []string{"/bin/sh"},
							Args: []string{"-c", `
								# Use the same logic as the main detection job
								if test -f /usr/bin/neutron-server; then
									exit 10  # eventlet
								elif command -v uwsgi >/dev/null 2>&1; then
									exit 20  # uwsgi
								elif command -v gunicorn >/dev/null 2>&1; then
									exit 30  # gunicorn
								elif command -v httpd >/dev/null 2>&1; then
									exit 40  # httpd
								else
									exit 1   # no strategy found
								fi
							`},
							SecurityContext: &corev1.SecurityContext{
								RunAsNonRoot:             ptr.To(true),
								AllowPrivilegeEscalation: ptr.To(false),
								ReadOnlyRootFilesystem:   ptr.To(true),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
						},
					},
				},
			},
		},
	}

	// Create the job
	err = d.client.Create(ctx, job)
	if err != nil {
		return "", fmt.Errorf("failed to create strategy check job: %w", err)
	}

	// Wait for job completion and read exit code
	strategy, err := d.waitForStrategyCheckResult(ctx, checkJobName, namespace)

	// Clean up job
	go func() {
		deleteCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = d.client.Delete(deleteCtx, job)
	}()

	return strategy, err
}

// waitForStrategyCheckResult waits for the strategy check job and reads the exit code
func (d *StrategyDetector) waitForStrategyCheckResult(ctx context.Context, jobName, namespace string) (string, error) {
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return "eventlet", nil // fallback on timeout

		case <-ticker.C:
			pods := &corev1.PodList{}
			err := d.client.List(ctx, pods, client.InNamespace(namespace), client.MatchingLabels{"job-name": jobName})
			if err != nil {
				continue
			}

			for _, pod := range pods.Items {
				if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
					for _, containerStatus := range pod.Status.ContainerStatuses {
						if containerStatus.Name == "strategy-checker" && containerStatus.State.Terminated != nil {
							exitCode := containerStatus.State.Terminated.ExitCode
							switch exitCode {
							case 10:
								return "eventlet", nil
							case 20:
								return "uwsgi", nil
							case 30:
								return "gunicorn", nil
							case 40:
								return "httpd", nil
							default:
								return "eventlet", nil // fallback
							}
						}
					}
				}
			}
		}
	}
}
