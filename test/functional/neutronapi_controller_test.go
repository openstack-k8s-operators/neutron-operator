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
	"strings"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"

	"github.com/openstack-k8s-operators/neutron-operator/pkg/neutronapi"
)

var _ = Describe("NeutronAPI controller", func() {

	var namespace string
	var secret *corev1.Secret
	var apiTransportURLName types.NamespacedName
	var neutronAPIName types.NamespacedName
	var memcachedSpec memcachedv1.MemcachedSpec
	var memcachedName types.NamespacedName

	BeforeEach(func() {
		// NOTE(gibi): We need to create a unique namespace for each test run
		// as namespaces cannot be deleted in a locally running envtest. See
		// https://book.kubebuilder.io/reference/envtest.html#namespace-usage-limitation
		namespace = uuid.New().String()
		th.CreateNamespace(namespace)
		// We still request the delete of the Namespace to properly cleanup if
		// we run the test in an existing cluster.
		DeferCleanup(th.DeleteNamespace, namespace)

		name := fmt.Sprintf("neutron-%s", uuid.New().String())
		apiTransportURLName = types.NamespacedName{
			Namespace: namespace,
			Name:      name + "-neutron-transport",
		}

		spec := GetDefaultNeutronAPISpec()
		spec["customServiceConfig"] = "[DEFAULT]\ndebug=True"
		neutronAPIName = CreateNeutronAPI(namespace, name, spec)
		memcachedSpec = memcachedv1.MemcachedSpec{
			Replicas: pointer.Int32(3),
		}
		memcachedName = types.NamespacedName{
			Name:      "memcached",
			Namespace: namespace,
		}
		DeferCleanup(DeleteNeutronAPI, neutronAPIName)
	})

	When("A NeutronAPI instance is created", func() {

		It("should have the Spec fields initialized", func() {
			NeutronAPI := GetNeutronAPI(neutronAPIName)
			Expect(NeutronAPI.Spec.DatabaseInstance).Should(Equal("test-neutron-db-instance"))
			Expect(NeutronAPI.Spec.DatabaseUser).Should(Equal("neutron"))
			Expect(NeutronAPI.Spec.RabbitMqClusterName).Should(Equal("rabbitmq"))
			Expect(NeutronAPI.Spec.MemcachedInstance).Should(Equal("memcached"))
			Expect(*(NeutronAPI.Spec.Replicas)).Should(Equal(int32(1)))
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
			for _, cond := range []condition.Type{
				condition.InputReadyCondition,
				condition.MemcachedReadyCondition,
			} {
				th.ExpectCondition(
					neutronAPIName,
					ConditionGetterFunc(NeutronAPIConditionGetter),
					cond,
					corev1.ConditionUnknown,
				)
			}
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
				return th.ListConfigMaps(fmt.Sprintf("%s-%s", neutronAPIName.Name, "config")).Items
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
				condition.RabbitMqTransportURLReadyCondition,
				corev1.ConditionFalse,
			)

		})
		It("should not create a config map", func() {
			Eventually(func() []corev1.ConfigMap {
				return th.ListConfigMaps(fmt.Sprintf("%s-%s", neutronAPIName.Name, "config")).Items
			}, timeout, interval).Should(BeEmpty())
		})
	})

	When("the proper secret is provided, TransportURL and Memcached are Created", func() {
		BeforeEach(func() {
			DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))

			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"transport_url": []byte("rabbit://user@svc:1234"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			SimulateTransportURLReady(apiTransportURLName)
			DeferCleanup(k8sClient.Delete, ctx, secret)
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			infra.SimulateMemcachedReady(memcachedName)
		})
		It("should be in a state of having the input ready", func() {

			th.ExpectCondition(
				neutronAPIName,
				ConditionGetterFunc(NeutronAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("should be in a state of having the TransportURL ready", func() {
			th.ExpectCondition(
				neutronAPIName,
				ConditionGetterFunc(NeutronAPIConditionGetter),
				condition.RabbitMqTransportURLReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should be in a state of having the Memcached ready", func() {
			th.ExpectCondition(
				neutronAPIName,
				ConditionGetterFunc(NeutronAPIConditionGetter),
				condition.MemcachedReadyCondition,
				corev1.ConditionTrue,
			)
		})
		It("should not create a config map", func() {
			Eventually(func() []corev1.ConfigMap {
				return th.ListConfigMaps(fmt.Sprintf("%s-%s", neutronAPIName.Name, "config")).Items
			}, timeout, interval).Should(BeEmpty())
		})

		It("should create an external SR-IOV Agent Secret with expected transport_url set", func() {
			externalSriovAgentSecret := types.NamespacedName{
				Namespace: neutronAPIName.Namespace,
				Name:      fmt.Sprintf("%s-sriov-agent-neutron-config", neutronAPIName.Name),
			}

			Eventually(func() corev1.Secret {
				return th.GetSecret(externalSriovAgentSecret)
			}, timeout, interval).ShouldNot(BeNil())

			transportURL := "rabbit://user@svc:1234"

			Expect(string(th.GetSecret(externalSriovAgentSecret).Data[neutronapi.NeutronSriovAgentSecretKey])).Should(
				ContainSubstring("transport_url = %s", transportURL))

			Eventually(func(g Gomega) {
				NeutronAPI := GetNeutronAPI(neutronAPIName)
				g.Expect(NeutronAPI.Status.Hash[externalSriovAgentSecret.Name]).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})

		It("should create an external DHCP Agent Secret with expected transport_url set", func() {
			externalDhcpAgentSecret := types.NamespacedName{
				Namespace: neutronAPIName.Namespace,
				Name:      fmt.Sprintf("%s-dhcp-agent-neutron-config", neutronAPIName.Name),
			}

			Eventually(func() corev1.Secret {
				return th.GetSecret(externalDhcpAgentSecret)
			}, timeout, interval).ShouldNot(BeNil())

			transportURL := "rabbit://user@svc:1234"

			Expect(string(th.GetSecret(externalDhcpAgentSecret).Data[neutronapi.NeutronDhcpAgentSecretKey])).Should(
				ContainSubstring("transport_url = %s", transportURL))

			Eventually(func(g Gomega) {
				NeutronAPI := GetNeutronAPI(neutronAPIName)
				g.Expect(NeutronAPI.Status.Hash[externalDhcpAgentSecret.Name]).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})
	})

	When("Memcached is available", func() {
		BeforeEach(func() {
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"transport_url": []byte("rabbit://user@svc:1234"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			infra.SimulateMemcachedReady(memcachedName)
			SimulateTransportURLReady(apiTransportURLName)
		})

		It("should have memcached ready", func() {
			th.ExpectCondition(
				neutronAPIName,
				ConditionGetterFunc(NeutronAPIConditionGetter),
				condition.MemcachedReadyCondition,
				corev1.ConditionTrue,
			)
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
					"transport_url":   []byte("rabbit://user@svc:1234"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)
			SimulateTransportURLReady(apiTransportURLName)
		})
		It("should not create a config map", func() {
			Eventually(func() []corev1.ConfigMap {
				return th.ListConfigMaps(fmt.Sprintf("%s-%s", neutronAPIName.Name, "config")).Items
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
					"transport_url":   []byte("rabbit://user@svc:1234"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)
			SimulateTransportURLReady(apiTransportURLName)
			DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
		})
		It("should not create a config map", func() {
			Eventually(func() []corev1.ConfigMap {
				return th.ListConfigMaps(fmt.Sprintf("%s-%s", neutronAPIName.Name, "config")).Items
			}, timeout, interval).Should(BeEmpty())
		})
	})

	When("keystoneAPI, OVNDBCluster and Memcached instances are available", func() {
		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      SecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"NeutronPassword": []byte("12345678"),
					"transport_url":   []byte("rabbit://user@svc:1234"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			infra.SimulateMemcachedReady(memcachedName)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					namespace,
					GetNeutronAPI(neutronAPIName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			SimulateTransportURLReady(apiTransportURLName)
			mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: neutronAPIName.Name})
		})

		It("should create a ConfigMap for 01-neutron.conf with the auth_url config option set based on the KeystoneAPI", func() {
			DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
			keystoneAPI := keystone.CreateKeystoneAPI(namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPI)

			secret := types.NamespacedName{
				Namespace: neutronAPIName.Namespace,
				Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "config"),
			}

			Eventually(func() corev1.Secret {
				return th.GetSecret(secret)
			}, timeout, interval).ShouldNot(BeNil())

			keystone := keystone.GetKeystoneAPI(keystoneAPI)
			Expect(th.GetSecret(secret).Data["01-neutron.conf"]).Should(
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
				corev1.ConditionTrue,
			)
		})

		It("should create a ConfigMap for 01-neutron.conf with api and rpc workers", func() {
			DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
			keystoneAPI := keystone.CreateKeystoneAPI(namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPI)

			secret := types.NamespacedName{
				Namespace: neutronAPIName.Namespace,
				Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "config"),
			}

			Eventually(func() corev1.Secret {
				return th.GetSecret(secret)
			}, timeout, interval).ShouldNot(BeNil())

			Expect(th.GetSecret(secret).Data["01-neutron.conf"]).Should(
				ContainSubstring("api_workers = 2"))
			Expect(th.GetSecret(secret).Data["01-neutron.conf"]).Should(
				ContainSubstring("rpc_workers = 1"))
			Expect(th.GetSecret(secret).Data["01-neutron.conf"]).Should(
				ContainSubstring("mysql_wsrep_sync_wait = 1"))

		})

		It("should create a ConfigMap for 01-neutron.conf with the ovn connection config option set based on the OVNDBCluster", func() {
			dbs := CreateOVNDBClusters(namespace)
			DeferCleanup(DeleteOVNDBClusters, dbs)
			keystoneAPI := keystone.CreateKeystoneAPI(namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPI)
			secret := types.NamespacedName{
				Namespace: neutronAPIName.Namespace,
				Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "config"),
			}

			Eventually(func() corev1.Secret {
				return th.GetSecret(secret)
			}, timeout, interval).ShouldNot(BeNil())
			for _, db := range dbs {
				ovndb := GetOVNDBCluster(db)
				Expect(th.GetSecret(secret).Data["01-neutron.conf"]).Should(
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
				corev1.ConditionTrue,
			)
		})
		It("should create a ConfigMap for 02-neutron-custom.conf with customServiceConfig input", func() {
			DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
			keystoneAPI := keystone.CreateKeystoneAPI(namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPI)

			secret := types.NamespacedName{
				Namespace: neutronAPIName.Namespace,
				Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "config"),
			}

			Eventually(func() corev1.Secret {
				return th.GetSecret(secret)
			}, timeout, interval).ShouldNot(BeNil())

			Expect(th.GetSecret(secret).Data["02-neutron-custom.conf"]).Should(
				ContainSubstring("[DEFAULT]\ndebug=True"))

		})
		It("should create an external Metadata Agent Secret with expected ovn_sb_connection set", func() {
			dbs := CreateOVNDBClusters(namespace)
			DeferCleanup(DeleteOVNDBClusters, dbs)

			externalSBEndpoint := "10.0.0.254"
			SetExternalDBEndpoint(dbs[1], externalSBEndpoint)

			externalMetadataAgentSecret := types.NamespacedName{
				Namespace: neutronAPIName.Namespace,
				Name:      fmt.Sprintf("%s-ovn-metadata-agent-neutron-config", neutronAPIName.Name),
			}

			Eventually(func() corev1.Secret {
				return th.GetSecret(externalMetadataAgentSecret)
			}, timeout, interval).ShouldNot(BeNil())

			Expect(th.GetSecret(externalMetadataAgentSecret).Data[neutronapi.NeutronOVNMetadataAgentSecretKey]).Should(
				ContainSubstring("ovn_sb_connection = %s", externalSBEndpoint))

			Eventually(func(g Gomega) {
				NeutronAPI := GetNeutronAPI(neutronAPIName)
				g.Expect(NeutronAPI.Status.Hash[externalMetadataAgentSecret.Name]).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})

		It("should delete Metadata Agent external Secret once SB DBCluster is deleted", func() {
			dbs := CreateOVNDBClusters(namespace)
			DeferCleanup(DeleteOVNDBClusters, dbs)

			externalSBEndpoint := "10.0.0.254"
			SetExternalDBEndpoint(dbs[1], externalSBEndpoint)

			externalMetadataAgentSecret := types.NamespacedName{
				Namespace: neutronAPIName.Namespace,
				Name:      fmt.Sprintf("%s-ovn-metadata-agent-neutron-config", neutronAPIName.Name),
			}

			Eventually(func() corev1.Secret {
				return th.GetSecret(externalMetadataAgentSecret)
			}, timeout, interval).ShouldNot(BeNil())

			Eventually(func(g Gomega) {
				NeutronAPI := GetNeutronAPI(neutronAPIName)
				g.Expect(NeutronAPI.Status.Hash[externalMetadataAgentSecret.Name]).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())

			DeleteOVNDBClusters([]types.NamespacedName{dbs[1]})

			Eventually(func(g Gomega) {
				secret := &corev1.Secret{}
				g.Expect(k8sClient.Get(ctx, externalMetadataAgentSecret, secret)).Should(HaveOccurred())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				NeutronAPI := GetNeutronAPI(neutronAPIName)
				g.Expect(NeutronAPI.Status.Hash[externalMetadataAgentSecret.Name]).To(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})

		It("should update Neutron Metadata Agent Secret once SB DBCluster is updated", func() {
			dbs := CreateOVNDBClusters(namespace)
			DeferCleanup(DeleteOVNDBClusters, dbs)

			externalSBEndpoint := "10.0.0.254"
			SetExternalDBEndpoint(dbs[1], externalSBEndpoint)

			externalMetadataAgentSecret := types.NamespacedName{
				Namespace: neutronAPIName.Namespace,
				Name:      fmt.Sprintf("%s-ovn-metadata-agent-neutron-config", neutronAPIName.Name),
			}

			Eventually(func() corev1.Secret {
				return th.GetSecret(externalMetadataAgentSecret)
			}, timeout, interval).ShouldNot(BeNil())
			Expect(th.GetSecret(externalMetadataAgentSecret).Data[neutronapi.NeutronOVNMetadataAgentSecretKey]).Should(
				ContainSubstring("ovn_sb_connection = %s", externalSBEndpoint))

			Eventually(func(g Gomega) {
				NeutronAPI := GetNeutronAPI(neutronAPIName)
				initialHash := NeutronAPI.Status.Hash[externalMetadataAgentSecret.Name]
				g.Expect(initialHash).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())

			NeutronAPI := GetNeutronAPI(neutronAPIName)
			initialHash := NeutronAPI.Status.Hash[externalMetadataAgentSecret.Name]

			newExternalSBEndpoint := "10.0.0.250"
			SetExternalDBEndpoint(dbs[1], newExternalSBEndpoint)

			Eventually(func(g Gomega) {
				g.Expect(th.GetSecret(externalMetadataAgentSecret).Data[neutronapi.NeutronOVNMetadataAgentSecretKey]).Should(
					ContainSubstring("ovn_sb_connection = %s", newExternalSBEndpoint))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				NeutronAPI := GetNeutronAPI(neutronAPIName)
				newHash := NeutronAPI.Status.Hash[externalMetadataAgentSecret.Name]
				g.Expect(newHash).NotTo(Equal(initialHash))
			}, timeout, interval).Should(Succeed())
		})
		It("should create a ConfigMap for neutron.conf with memcached servers set", func() {
			DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
			keystoneAPI := keystone.CreateKeystoneAPI(namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPI)

			secret := types.NamespacedName{
				Namespace: neutronAPIName.Namespace,
				Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "config"),
			}

			Eventually(func() corev1.Secret {
				return th.GetSecret(secret)
			}, timeout, interval).ShouldNot(BeNil())
			Expect(th.GetSecret(secret).Data["01-neutron.conf"]).Should(
				ContainSubstring("memcache_servers=memcached-0.memcached:11211,memcached-1.memcached:11211,memcached-2.memcached:11211"))
			Expect(th.GetSecret(secret).Data["01-neutron.conf"]).Should(
				ContainSubstring("memcached_servers=inet:[memcached-0.memcached]:11211,inet:[memcached-1.memcached]:11211,inet:[memcached-2.memcached]:11211"))

		})

		It("should create an external Ovn Agent Secret with expected ovn nb and sb connection set", func() {
			dbs := CreateOVNDBClusters(namespace)
			DeferCleanup(DeleteOVNDBClusters, dbs)

			externalNBEndpoint := "10.0.0.253"
			SetExternalDBEndpoint(dbs[0], externalNBEndpoint)
			externalSBEndpoint := "10.0.0.254"
			SetExternalDBEndpoint(dbs[1], externalSBEndpoint)

			externalOvnAgentSecret := types.NamespacedName{
				Namespace: neutronAPIName.Namespace,
				Name:      fmt.Sprintf("%s-ovn-agent-neutron-config", neutronAPIName.Name),
			}

			Eventually(func() corev1.Secret {
				return th.GetSecret(externalOvnAgentSecret)
			}, timeout, interval).ShouldNot(BeNil())

			Expect(th.GetSecret(externalOvnAgentSecret).Data[neutronapi.NeutronOVNAgentSecretKey]).Should(
				ContainSubstring("ovn_nb_connection = %s", externalNBEndpoint))
			Expect(th.GetSecret(externalOvnAgentSecret).Data[neutronapi.NeutronOVNAgentSecretKey]).Should(
				ContainSubstring("ovn_sb_connection = %s", externalSBEndpoint))

			Eventually(func(g Gomega) {
				NeutronAPI := GetNeutronAPI(neutronAPIName)
				g.Expect(NeutronAPI.Status.Hash[externalOvnAgentSecret.Name]).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})

		It("should delete Ovn Agent external Secret once NB or SB DBClusters are deleted", func() {
			dbs := CreateOVNDBClusters(namespace)
			DeferCleanup(DeleteOVNDBClusters, dbs)
			externalNBEndpoint := "10.0.0.253"
			SetExternalDBEndpoint(dbs[0], externalNBEndpoint)

			externalSBEndpoint := "10.0.0.254"
			SetExternalDBEndpoint(dbs[1], externalSBEndpoint)

			externalOvnAgentSecret := types.NamespacedName{
				Namespace: neutronAPIName.Namespace,
				Name:      fmt.Sprintf("%s-ovn-agent-neutron-config", neutronAPIName.Name),
			}

			Eventually(func() corev1.Secret {
				return th.GetSecret(externalOvnAgentSecret)
			}, timeout, interval).ShouldNot(BeNil())

			Eventually(func(g Gomega) {
				NeutronAPI := GetNeutronAPI(neutronAPIName)
				g.Expect(NeutronAPI.Status.Hash[externalOvnAgentSecret.Name]).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())

			DeleteOVNDBClusters([]types.NamespacedName{dbs[1]})

			Eventually(func(g Gomega) {
				secret := &corev1.Secret{}
				g.Expect(k8sClient.Get(ctx, externalOvnAgentSecret, secret)).Should(HaveOccurred())
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				NeutronAPI := GetNeutronAPI(neutronAPIName)
				g.Expect(NeutronAPI.Status.Hash[externalOvnAgentSecret.Name]).To(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})

		It("should update Neutron Ovn Agent Secret once NB or SB DBCluster is updated", func() {
			dbs := CreateOVNDBClusters(namespace)
			DeferCleanup(DeleteOVNDBClusters, dbs)
			externalNBEndpoint := "10.0.0.253"
			SetExternalDBEndpoint(dbs[0], externalNBEndpoint)

			externalSBEndpoint := "10.0.0.254"
			SetExternalDBEndpoint(dbs[1], externalSBEndpoint)

			externalOvnAgentSecret := types.NamespacedName{
				Namespace: neutronAPIName.Namespace,
				Name:      fmt.Sprintf("%s-ovn-agent-neutron-config", neutronAPIName.Name),
			}

			Eventually(func() corev1.Secret {
				return th.GetSecret(externalOvnAgentSecret)
			}, timeout, interval).ShouldNot(BeNil())
			Expect(th.GetSecret(externalOvnAgentSecret).Data[neutronapi.NeutronOVNAgentSecretKey]).Should(
				ContainSubstring("ovn_sb_connection = %s", externalSBEndpoint))

			Eventually(func(g Gomega) {
				NeutronAPI := GetNeutronAPI(neutronAPIName)
				initialHash := NeutronAPI.Status.Hash[externalOvnAgentSecret.Name]
				g.Expect(initialHash).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())

			NeutronAPI := GetNeutronAPI(neutronAPIName)
			initialHash := NeutronAPI.Status.Hash[externalOvnAgentSecret.Name]

			newExternalNBEndpoint := "10.0.0.249"
			SetExternalDBEndpoint(dbs[0], newExternalNBEndpoint)
			newExternalSBEndpoint := "10.0.0.250"
			SetExternalDBEndpoint(dbs[1], newExternalSBEndpoint)

			Eventually(func(g Gomega) {
				g.Expect(th.GetSecret(externalOvnAgentSecret).Data[neutronapi.NeutronOVNAgentSecretKey]).Should(
					ContainSubstring("ovn_nb_connection = %s", newExternalNBEndpoint))
			}, timeout, interval).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(th.GetSecret(externalOvnAgentSecret).Data[neutronapi.NeutronOVNAgentSecretKey]).Should(
					ContainSubstring("ovn_sb_connection = %s", newExternalSBEndpoint))
			}, timeout, interval).Should(Succeed())

			Eventually(func(g Gomega) {
				NeutronAPI := GetNeutronAPI(neutronAPIName)
				newHash := NeutronAPI.Status.Hash[externalOvnAgentSecret.Name]
				g.Expect(newHash).NotTo(Equal(initialHash))
			}, timeout, interval).Should(Succeed())
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
					"transport_url":   []byte("rabbit://user@svc:1234"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					namespace,
					GetNeutronAPI(neutronAPIName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			SimulateTransportURLReady(apiTransportURLName)
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			infra.SimulateMemcachedReady(memcachedName)
			DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(namespace))
		})
		It("Should set DBReady Condition and set DatabaseHostname Status when DB is Created", func() {
			mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: neutronAPIName.Name})
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
					"transport_url":   []byte("rabbit://user@svc:1234"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)
			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					namespace,
					GetNeutronAPI(neutronAPIName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			SimulateTransportURLReady(apiTransportURLName)
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			infra.SimulateMemcachedReady(memcachedName)
			DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
			DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(namespace))
			mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: neutronAPIName.Name})
			th.SimulateJobSuccess(types.NamespacedName{Namespace: namespace, Name: neutronAPIName.Name + "-db-sync"})
			keystone.SimulateKeystoneServiceReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
			keystone.SimulateKeystoneEndpointReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
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

		It("Endpoints are created", func() {

			th.ExpectCondition(
				neutronAPIName,
				ConditionGetterFunc(NeutronAPIConditionGetter),
				condition.KeystoneEndpointReadyCondition,
				corev1.ConditionTrue,
			)
			keystoneEndpoint := keystone.GetKeystoneEndpoint(types.NamespacedName{Namespace: namespace, Name: "neutron"})
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
			Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(2))
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(2))

			nSvcContainer := deployment.Spec.Template.Spec.Containers[0]
			Expect(nSvcContainer.LivenessProbe.HTTPGet.Port.IntVal).To(Equal(int32(9696)))
			Expect(nSvcContainer.ReadinessProbe.HTTPGet.Port.IntVal).To(Equal(int32(9696)))
			Expect(nSvcContainer.VolumeMounts).To(HaveLen(1))
			Expect(nSvcContainer.Image).To(Equal("test-neutron-container-image"))

			nHttpdProxyContainer := deployment.Spec.Template.Spec.Containers[1]
			Expect(nHttpdProxyContainer.LivenessProbe.HTTPGet.Port.IntVal).To(Equal(int32(9696)))
			Expect(nHttpdProxyContainer.ReadinessProbe.HTTPGet.Port.IntVal).To(Equal(int32(9696)))
			Expect(nHttpdProxyContainer.VolumeMounts).To(HaveLen(2))
			Expect(nHttpdProxyContainer.Image).To(Equal("test-neutron-container-image"))
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
					"transport_url":   []byte("rabbit://user@svc:1234"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, secret)
			DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
			infra.SimulateMemcachedReady(memcachedName)
			SimulateTransportURLReady(apiTransportURLName)

			DeferCleanup(
				mariadb.DeleteDBService,
				mariadb.CreateDBService(
					namespace,
					GetNeutronAPI(neutronAPIName).Spec.DatabaseInstance,
					corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 3306}},
					},
				),
			)
			mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: neutronAPIName.Name})
		})

		It("removes the Config MAP", func() {
			DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
			keystoneAPI := keystone.CreateKeystoneAPI(namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPI)
			secret := types.NamespacedName{
				Namespace: neutronAPIName.Namespace,
				Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "config"),
			}

			Eventually(func() corev1.Secret {
				return th.GetSecret(secret)
			}, timeout, interval).ShouldNot(BeNil())

			DeleteNeutronAPI(neutronAPIName)

			// TODO(ihar) uncomment when we actually handle finalizers for created Secrets on neutronapi delete
			//Eventually(func() corev1.Secret {
			//	return th.GetSecret(secret)
			//}, timeout, interval).Should(BeNil())
		})
	})
})
