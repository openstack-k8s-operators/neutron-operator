/*
Copyright 2022.

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
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openstack-k8s-operators/lib-common/modules/test/helpers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	neutronv1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
)

var _ = Describe("NeutronAPI controller", func() {

	var namespace string
	var secret *corev1.Secret
	var apiTransportURLName types.NamespacedName
	var neutronAPIName types.NamespacedName

	BeforeEach(func() {
		// NOTE(gibi): We need to create a unique namespace for each test run
		// as namespaces cannot be deleted in a locally running envtest. See
		// https://book.kubebuilder.io/reference/envtest.html#namespace-usage-limitation
		namespace = uuid.New().String()
		th.CreateNamespace(namespace)
		// We still request the delete of the Namespace to properly cleanup if
		// we run the test in an existing cluster.
		DeferCleanup(th.DeleteNamespace, namespace)
		// lib-common uses OPERATOR_TEMPLATES env var to locate the "templates"
		// directory of the operator. We need to set them otherwise lib-common
		// will fail to generate the ConfigMap as it does not find common.sh
		err := os.Setenv("OPERATOR_TEMPLATES", "../../templates")
		Expect(err).NotTo(HaveOccurred())

		name := fmt.Sprintf("neutron-%s", uuid.New().String())
		apiTransportURLName = types.NamespacedName{
			Namespace: namespace,
			Name:      name + "-neutron-transport",
		}

		neutronAPIName = CreateNeutronAPI(namespace, name, GetDefaultNeutronAPISpec())
		DeferCleanup(DeleteNeutronAPI, neutronAPIName)
	})

	When("A NeutronAPI instance is created", func() {

		It("should have the Spec fields initialized", func() {
			NeutronAPI := GetNeutronAPI(neutronAPIName)
			Expect(NeutronAPI.Spec.DatabaseInstance).Should(Equal("test-neutron-db-instance"))
			Expect(NeutronAPI.Spec.DatabaseUser).Should(Equal("neutron"))
			Expect(NeutronAPI.Spec.RabbitMqClusterName).Should(Equal("rabbitmq"))
			Expect(NeutronAPI.Spec.Replicas).Should(Equal(int32(1)))
			Expect(NeutronAPI.Spec.ServiceUser).Should(Equal("neutron"))
		})

		It("should have the Status fields initialized", func() {
			NeutronAPI := GetNeutronAPI(neutronAPIName)
			Expect(NeutronAPI.Status.Hash).To(BeEmpty())
			Expect(NeutronAPI.Status.DatabaseHostname).To(Equal(""))
			Expect(NeutronAPI.Status.TransportURLSecret).To(Equal(""))
			Expect(NeutronAPI.Status.ReadyCount).To(Equal(int32(0)))
		})

		It("should have Unknown Conditions initialized as transporturl not created", func() {
			th.ExpectCondition(
				neutronAPIName,
				ConditionGetterFunc(NeutronAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionUnknown,
			)
		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return GetNeutronAPI(neutronAPIName).Finalizers
			}, timeout, interval).Should(ContainElement("NeutronAPI"))
		})

		It("should not create a config map", func() {
			Eventually(func() []corev1.ConfigMap {
				return th.ListConfigMaps(fmt.Sprintf("%s-%s", neutronAPIName.Name, "config-data")).Items
			}, timeout, interval).Should(BeEmpty())
		})
	})

	When("an unrelated secret is provided", func() {
		It("should remain in a state of waiting for the proper secret", func() {
			SimulateTransportURLReady(apiTransportURLName)
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "an-unrelated-secret",
					Namespace: namespace,
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)

			th.ExpectCondition(
				neutronAPIName,
				ConditionGetterFunc(NeutronAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
			)
			th.ExpectCondition(
				neutronAPIName,
				ConditionGetterFunc(NeutronAPIConditionGetter),
				neutronv1.NeutronRabbitMqTransportURLReadyCondition,
				corev1.ConditionFalse,
			)

		})
		It("should not create a config map", func() {
			Eventually(func() []corev1.ConfigMap {
				return th.ListConfigMaps(fmt.Sprintf("%s-%s", neutronAPIName.Name, "config-data")).Items
			}, timeout, interval).Should(BeEmpty())
		})
	})

	When("the proper secret is provided and TransportURL Created", func() {
		BeforeEach(func() {
			SimulateTransportURLReady(apiTransportURLName)
			DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))

			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretName,
					Namespace: namespace,
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)
		})
		It("should not be in a state of having the input ready", func() {

			th.ExpectCondition(
				neutronAPIName,
				ConditionGetterFunc(NeutronAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
			)
		})

		It("should be in a state of having the TransportURL ready", func() {
			th.ExpectCondition(
				neutronAPIName,
				ConditionGetterFunc(NeutronAPIConditionGetter),
				neutronv1.NeutronRabbitMqTransportURLReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("should not create a config map", func() {
			Eventually(func() []corev1.ConfigMap {
				return th.ListConfigMaps(fmt.Sprintf("%s-%s", neutronAPIName.Name, "config-data")).Items
			}, timeout, interval).Should(BeEmpty())
		})
	})

	When("OVNDBCluster instance is not available", func() {
		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"NeutronPassword": []byte("12345678"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)
			SimulateTransportURLReady(apiTransportURLName)
		})
		It("should not create a config map", func() {
			Eventually(func() []corev1.ConfigMap {
				return th.ListConfigMaps(fmt.Sprintf("%s-%s", neutronAPIName.Name, "config-data")).Items
			}, timeout, interval).Should(BeEmpty())
		})
	})

	When("keystoneAPI instance is not available", func() {
		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"NeutronPassword": []byte("12345678"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)
			SimulateTransportURLReady(apiTransportURLName)
			DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
		})
		It("should not create a config map", func() {
			Eventually(func() []corev1.ConfigMap {
				return th.ListConfigMaps(fmt.Sprintf("%s-%s", neutronAPIName.Name, "config-data")).Items
			}, timeout, interval).Should(BeEmpty())
		})
	})

	When("keystoneAPI and OVNDBCluster instances are available", func() {
		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"NeutronPassword": []byte("12345678"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)
			SimulateTransportURLReady(apiTransportURLName)
		})

		It("should create a ConfigMap for neutron.conf with the auth_url config option set based on the KeystoneAPI", func() {
			DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
			keystoneAPI := th.CreateKeystoneAPI(namespace)
			DeferCleanup(th.DeleteKeystoneAPI, keystoneAPI)

			configataCM := types.NamespacedName{
				Namespace: neutronAPIName.Namespace,
				Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "config-data"),
			}

			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(configataCM)
			}, timeout, interval).ShouldNot(BeNil())

			keystone := th.GetKeystoneAPI(keystoneAPI)
			Expect(th.GetConfigMap(configataCM).Data["neutron.conf"]).Should(
				ContainSubstring("auth_url = %s", keystone.Status.APIEndpoints["internal"]))

			th.ExpectCondition(
				neutronAPIName,
				ConditionGetterFunc(NeutronAPIConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				neutronAPIName,
				ConditionGetterFunc(NeutronAPIConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionFalse,
			)
		})
		It("should create a ConfigMap for neutron.conf with the ovn connection config option set based on the OVNDBCluster", func() {
			dbs := CreateOVNDBClusters(namespace)
			DeferCleanup(DeleteOVNDBClusters, dbs)
			keystoneAPI := th.CreateKeystoneAPI(namespace)
			DeferCleanup(th.DeleteKeystoneAPI, keystoneAPI)
			configataCM := types.NamespacedName{
				Namespace: neutronAPIName.Namespace,
				Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "config-data"),
			}

			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(configataCM)
			}, timeout, interval).ShouldNot(BeNil())
			for _, db := range dbs {
				ovndb := GetOVNDBCluster(db)
				Expect(th.GetConfigMap(configataCM).Data["neutron.conf"]).Should(
					ContainSubstring("ovn_%s_connection = %s", strings.ToLower(string(ovndb.Spec.DBType)), ovndb.Status.InternalDBAddress))
			}

			th.ExpectCondition(
				neutronAPIName,
				ConditionGetterFunc(NeutronAPIConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				neutronAPIName,
				ConditionGetterFunc(NeutronAPIConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionFalse,
			)
		})
	})

	When("DB is created", func() {
		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"NeutronPassword": []byte("12345678"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)
			DeferCleanup(
				th.DeleteDBService,
				th.CreateDBService(
					namespace,
					GetNeutronAPI(neutronAPIName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			SimulateTransportURLReady(apiTransportURLName)
			DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
			DeferCleanup(th.DeleteKeystoneAPI, th.CreateKeystoneAPI(namespace))
		})
		It("Should set DBReady Condition and set DatabaseHostname Status when DB is Created", func() {
			th.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: neutronAPIName.Name})
			th.SimulateJobSuccess(types.NamespacedName{Namespace: namespace, Name: neutronAPIName.Name + "-db-sync"})
			NeutronAPI := GetNeutronAPI(neutronAPIName)
			Expect(NeutronAPI.Status.DatabaseHostname).To(Equal("hostname-for-" + NeutronAPI.Spec.DatabaseInstance))
			th.ExpectCondition(
				neutronAPIName,
				ConditionGetterFunc(NeutronAPIConditionGetter),
				condition.DBReadyCondition,
				corev1.ConditionTrue,
			)
			th.ExpectCondition(
				neutronAPIName,
				ConditionGetterFunc(NeutronAPIConditionGetter),
				condition.DBSyncReadyCondition,
				corev1.ConditionFalse,
			)
		})
	})

	When("Keystone Resources are created", func() {
		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"NeutronPassword": []byte("12345678"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)
			DeferCleanup(
				th.DeleteDBService,
				th.CreateDBService(
					namespace,
					GetNeutronAPI(neutronAPIName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			SimulateTransportURLReady(apiTransportURLName)
			DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
			DeferCleanup(th.DeleteKeystoneAPI, th.CreateKeystoneAPI(namespace))
			th.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: neutronAPIName.Name})
			th.SimulateJobSuccess(types.NamespacedName{Namespace: namespace, Name: neutronAPIName.Name + "-db-sync"})
			th.SimulateKeystoneServiceReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
			th.SimulateKeystoneEndpointReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
		})
		It("Should set ExposeServiceReadyCondition Condition", func() {

			th.ExpectCondition(
				neutronAPIName,
				ConditionGetterFunc(NeutronAPIConditionGetter),
				condition.ExposeServiceReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("Assert Services are created", func() {
			th.AssertServiceExists(types.NamespacedName{Namespace: namespace, Name: "neutron-public"})
			th.AssertServiceExists(types.NamespacedName{Namespace: namespace, Name: "neutron-internal"})
		})

		It("Assert Routes are created", func() {
			th.AssertRouteExists(types.NamespacedName{Namespace: namespace, Name: "neutron-public"})
		})

		It("Endpoints are created", func() {

			th.ExpectCondition(
				neutronAPIName,
				ConditionGetterFunc(NeutronAPIConditionGetter),
				condition.KeystoneEndpointReadyCondition,
				corev1.ConditionTrue,
			)
			keystoneEndpoint := th.GetKeystoneEndpoint(types.NamespacedName{Namespace: namespace, Name: "neutron"})
			endpoints := keystoneEndpoint.Spec.Endpoints
			regexp := `http:.*:?\d*$`
			Expect(endpoints).To(HaveKeyWithValue("public", MatchRegexp(regexp)))
			Expect(endpoints).To(HaveKeyWithValue("internal", MatchRegexp(regexp)))
		})

		It("Deployment is created as expected", func() {
			deployment := th.GetDeployment(
				types.NamespacedName{
					Namespace: neutronAPIName.Namespace,
					Name:      "neutron",
				},
			)
			Expect(int(*deployment.Spec.Replicas)).To(Equal(1))
			Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(5))
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))

			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.LivenessProbe.HTTPGet.Port.IntVal).To(Equal(int32(9696)))
			Expect(container.ReadinessProbe.HTTPGet.Port.IntVal).To(Equal(int32(9696)))
			Expect(container.VolumeMounts).To(HaveLen(4))
			Expect(container.Image).To(Equal("test-neutron-container-image"))

			Expect(container.LivenessProbe.HTTPGet.Port.IntVal).To(Equal(int32(9696)))
			Expect(container.ReadinessProbe.HTTPGet.Port.IntVal).To(Equal(int32(9696)))
		})
	})

	When("NeutronAPI CR is deleted", func() {
		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"NeutronPassword": []byte("12345678"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)
			SimulateTransportURLReady(apiTransportURLName)
		})

		It("removes the Config MAP", func() {
			DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
			keystoneAPI := th.CreateKeystoneAPI(namespace)
			DeferCleanup(th.DeleteKeystoneAPI, keystoneAPI)
			configataCM := types.NamespacedName{
				Namespace: neutronAPIName.Namespace,
				Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "config-data"),
			}

			scriptsCM := types.NamespacedName{
				Namespace: neutronAPIName.Namespace,
				Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "scripts"),
			}

			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(configataCM)
			}, timeout, interval).ShouldNot(BeNil())
			Eventually(func() corev1.ConfigMap {
				return *th.GetConfigMap(scriptsCM)
			}, timeout, interval).ShouldNot(BeNil())

			DeleteNeutronAPI(neutronAPIName)

			Eventually(func() []corev1.ConfigMap {
				return th.ListConfigMaps(configataCM.Name).Items
			}, timeout, interval).Should(BeEmpty())
			Eventually(func() []corev1.ConfigMap {
				return th.ListConfigMaps(scriptsCM.Name).Items
			}, timeout, interval).Should(BeEmpty())
		})
	})
})
