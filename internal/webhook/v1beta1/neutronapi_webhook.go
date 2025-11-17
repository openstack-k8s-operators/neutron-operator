/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package v1beta1 contains the webhook implementations for the NeutronAPI v1beta1 API.
package v1beta1

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	neutronv1beta1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
)

var (
	// ErrInvalidObject is returned when the object is not of the expected type
	ErrInvalidObject = errors.New("invalid object type")
	// ErrConvertExisting is returned when unable to convert existing object
	ErrConvertExisting = errors.New("unable to convert existing object")
)

// nolint:unused
// log is for logging in this package.
var neutronapilog = logf.Log.WithName("neutronapi-resource")

// SetupNeutronAPIWebhookWithManager registers the webhook for NeutronAPI in the manager.
func SetupNeutronAPIWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&neutronv1beta1.NeutronAPI{}).
		WithValidator(&NeutronAPICustomValidator{}).
		WithDefaulter(&NeutronAPICustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-neutron-openstack-org-v1beta1-neutronapi,mutating=true,failurePolicy=fail,sideEffects=None,groups=neutron.openstack.org,resources=neutronapis,verbs=create;update,versions=v1beta1,name=mneutronapi-v1beta1.kb.io,admissionReviewVersions=v1

// NeutronAPICustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind NeutronAPI when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type NeutronAPICustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &NeutronAPICustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind NeutronAPI.
func (d *NeutronAPICustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	neutronapi, ok := obj.(*neutronv1beta1.NeutronAPI)

	if !ok {
		return fmt.Errorf("%w: expected an NeutronAPI object but got %T", ErrInvalidObject, obj)
	}
	neutronapilog.Info("Defaulting for NeutronAPI", "name", neutronapi.GetName())

	neutronapi.Default()

	return nil
}

// +kubebuilder:webhook:path=/validate-neutron-openstack-org-v1beta1-neutronapi,mutating=false,failurePolicy=fail,sideEffects=None,groups=neutron.openstack.org,resources=neutronapis,verbs=create;update,versions=v1beta1,name=vneutronapi-v1beta1.kb.io,admissionReviewVersions=v1

// NeutronAPICustomValidator struct is responsible for validating the NeutronAPI resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type NeutronAPICustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &NeutronAPICustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type NeutronAPI.
func (v *NeutronAPICustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	neutronapi, ok := obj.(*neutronv1beta1.NeutronAPI)
	if !ok {
		return nil, fmt.Errorf("%w: expected a NeutronAPI object but got %T", ErrInvalidObject, obj)
	}
	neutronapilog.Info("Validation for NeutronAPI upon creation", "name", neutronapi.GetName())

	return neutronapi.ValidateCreate()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type NeutronAPI.
func (v *NeutronAPICustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	neutronapi, ok := newObj.(*neutronv1beta1.NeutronAPI)
	if !ok {
		return nil, fmt.Errorf("%w: expected a NeutronAPI object for the newObj but got %T", ErrInvalidObject, newObj)
	}
	neutronapilog.Info("Validation for NeutronAPI upon update", "name", neutronapi.GetName())

	return neutronapi.ValidateUpdate(oldObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type NeutronAPI.
func (v *NeutronAPICustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	neutronapi, ok := obj.(*neutronv1beta1.NeutronAPI)
	if !ok {
		return nil, fmt.Errorf("%w: expected a NeutronAPI object but got %T", ErrInvalidObject, obj)
	}
	neutronapilog.Info("Validation for NeutronAPI upon deletion", "name", neutronapi.GetName())

	return neutronapi.ValidateDelete()
}
