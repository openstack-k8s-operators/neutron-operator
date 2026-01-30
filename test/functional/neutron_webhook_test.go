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

package functional_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("NeutronAPI webhook", func() {
	It("rejects update to deprecated rabbitMqClusterName field", func() {
		spec := GetDefaultNeutronAPISpec()
		spec["rabbitMqClusterName"] = "rabbitmq"

		neutronName := types.NamespacedName{
			Namespace: namespace,
			Name:      "neutron-webhook-test",
		}

		raw := map[string]any{
			"apiVersion": "neutron.openstack.org/v1beta1",
			"kind":       "NeutronAPI",
			"metadata": map[string]any{
				"name":      neutronName.Name,
				"namespace": neutronName.Namespace,
			},
			"spec": spec,
		}

		// Create the NeutronAPI instance
		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			ctx, k8sClient, unstructuredObj, func() error { return nil })
		Expect(err).ShouldNot(HaveOccurred())

		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, unstructuredObj)
		})

		// Try to update rabbitMqClusterName
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, neutronName, unstructuredObj)).Should(Succeed())
			specMap := unstructuredObj.Object["spec"].(map[string]any)
			specMap["rabbitMqClusterName"] = "rabbitmq2"
			err := k8sClient.Update(ctx, unstructuredObj)
			g.Expect(err).Should(HaveOccurred())

			var statusError *k8s_errors.StatusError
			g.Expect(errors.As(err, &statusError)).To(BeTrue())
			g.Expect(statusError.ErrStatus.Details.Kind).To(Equal("NeutronAPI"))
			g.Expect(statusError.ErrStatus.Message).To(
				ContainSubstring("field \"spec.rabbitMqClusterName\" is deprecated"))
			g.Expect(statusError.ErrStatus.Message).To(
				ContainSubstring("use \"spec.messagingBus.cluster\" instead"))
		}, timeout, interval).Should(Succeed())
	})

	It("rejects update to deprecated notificationsBusInstance field", func() {
		spec := GetDefaultNeutronAPISpec()
		notificationsBusInstance := "notifications-rabbitmq"
		spec["notificationsBusInstance"] = notificationsBusInstance

		neutronName := types.NamespacedName{
			Namespace: namespace,
			Name:      "neutron-webhook-notif-test",
		}

		raw := map[string]any{
			"apiVersion": "neutron.openstack.org/v1beta1",
			"kind":       "NeutronAPI",
			"metadata": map[string]any{
				"name":      neutronName.Name,
				"namespace": neutronName.Namespace,
			},
			"spec": spec,
		}

		// Create the NeutronAPI instance
		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			ctx, k8sClient, unstructuredObj, func() error { return nil })
		Expect(err).ShouldNot(HaveOccurred())

		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, unstructuredObj)
		})

		// Try to update notificationsBusInstance
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, neutronName, unstructuredObj)).Should(Succeed())
			specMap := unstructuredObj.Object["spec"].(map[string]any)
			specMap["notificationsBusInstance"] = "notifications-rabbitmq2"
			err := k8sClient.Update(ctx, unstructuredObj)
			g.Expect(err).Should(HaveOccurred())

			var statusError *k8s_errors.StatusError
			g.Expect(errors.As(err, &statusError)).To(BeTrue())
			g.Expect(statusError.ErrStatus.Details.Kind).To(Equal("NeutronAPI"))
			g.Expect(statusError.ErrStatus.Message).To(
				ContainSubstring("field \"spec.notificationsBusInstance\" is deprecated"))
			g.Expect(statusError.ErrStatus.Message).To(
				ContainSubstring("use \"spec.notificationsBus.cluster\" instead"))
		}, timeout, interval).Should(Succeed())
	})
})
