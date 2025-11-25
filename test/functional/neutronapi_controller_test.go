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
	"encoding/json"
	"fmt"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	//revive:disable-next-line:dot-imports
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"

	"github.com/google/uuid"
	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	mariadb_test "github.com/openstack-k8s-operators/mariadb-operator/api/test/helpers"
	neutronv1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"

	"github.com/openstack-k8s-operators/neutron-operator/internal/neutronapi"
)

func getNeutronAPIControllerSuite(ml2MechanismDrivers []string) func() {
	isOVNEnabled := len(ml2MechanismDrivers) == 0
	for _, v := range ml2MechanismDrivers {
		if v == "ovn" {
			isOVNEnabled = true
		}
	}
	return func() {
		var secret *corev1.Secret
		var apiTransportURLName types.NamespacedName
		var neutronAPIName types.NamespacedName
		var memcachedSpec memcachedv1.MemcachedSpec
		var memcachedName types.NamespacedName
		var name string
		var spec map[string]any
		var caBundleSecretName types.NamespacedName
		var internalCertSecretName types.NamespacedName
		var publicCertSecretName types.NamespacedName
		var ovnDbCertSecretName types.NamespacedName
		var neutronDeploymentName types.NamespacedName
		var neutronDBSyncJobName types.NamespacedName
		var neutronAPITopologies []types.NamespacedName

		BeforeEach(func() {
			name = fmt.Sprintf("neutron-%s", uuid.New().String())
			apiTransportURLName = types.NamespacedName{
				Namespace: namespace,
				Name:      name + "-neutron-transport",
			}

			spec = GetDefaultNeutronAPISpec()
			if len(ml2MechanismDrivers) > 0 {
				spec["ml2MechanismDrivers"] = ml2MechanismDrivers
			}

			neutronAPIName = types.NamespacedName{
				Namespace: namespace,
				Name:      name,
			}
			memcachedSpec = infra.GetDefaultMemcachedSpec()
			memcachedName = types.NamespacedName{
				Name:      "memcached",
				Namespace: namespace,
			}
			caBundleSecretName = types.NamespacedName{
				Name:      CABundleSecretName,
				Namespace: namespace,
			}
			internalCertSecretName = types.NamespacedName{
				Name:      InternalCertSecretName,
				Namespace: namespace,
			}
			publicCertSecretName = types.NamespacedName{
				Name:      PublicCertSecretName,
				Namespace: namespace,
			}
			ovnDbCertSecretName = types.NamespacedName{
				Name:      OVNDbCertSecretName,
				Namespace: namespace,
			}
			neutronDeploymentName = types.NamespacedName{
				Name:      neutronapi.ServiceName,
				Namespace: namespace,
			}
			neutronDBSyncJobName = types.NamespacedName{
				Name:      neutronAPIName.Name + "-db-sync",
				Namespace: namespace,
			}
			neutronAPITopologies = []types.NamespacedName{
				{
					Namespace: namespace,
					Name:      fmt.Sprintf("%s-topology", neutronAPIName.Name),
				},
				{
					Namespace: namespace,
					Name:      fmt.Sprintf("%s-topology-alt", neutronAPIName.Name),
				},
			}
		})

		When("A NeutronAPI instance is created", func() {
			BeforeEach(func() {
				DeferCleanup(th.DeleteInstance, CreateNeutronAPI(neutronAPIName.Namespace, neutronAPIName.Name, spec))
			})

			It("should have the Spec fields initialized", func() {
				NeutronAPI := GetNeutronAPI(neutronAPIName)
				Expect(NeutronAPI.Spec.DatabaseInstance).Should(Equal("test-neutron-db-instance"))
				Expect(NeutronAPI.Spec.DatabaseAccount).Should(Equal("neutron"))
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
				}, timeout, interval).Should(ContainElement("openstack.org/neutronapi"))
			})

			It("should not create a secret", func() {
				secret := types.NamespacedName{
					Namespace: neutronAPIName.Namespace,
					Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "config"),
				}
				th.AssertSecretDoesNotExist(secret)
			})
		})

		When("an unrelated secret is provided", func() {
			BeforeEach(func() {
				DeferCleanup(th.DeleteInstance, CreateNeutronAPI(neutronAPIName.Namespace, neutronAPIName.Name, spec))
			})

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
					corev1.ConditionUnknown,
				)

			})
			It("should not create a secret", func() {
				secret := types.NamespacedName{
					Namespace: neutronAPIName.Namespace,
					Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "config"),
				}
				th.AssertSecretDoesNotExist(secret)
			})
		})

		When("the proper secret is provided, TransportURL and Memcached are Created", func() {
			BeforeEach(func() {
				DeferCleanup(th.DeleteInstance, CreateNeutronAPI(neutronAPIName.Namespace, neutronAPIName.Name, spec))
				DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))

				secret = &corev1.Secret{
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
			It("should not create a secret", func() {
				secret := types.NamespacedName{
					Namespace: neutronAPIName.Namespace,
					Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "config"),
				}
				th.AssertSecretDoesNotExist(secret)
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
				DeferCleanup(th.DeleteInstance, CreateNeutronAPI(neutronAPIName.Namespace, neutronAPIName.Name, spec))

				secret = &corev1.Secret{
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
				DeferCleanup(th.DeleteInstance, CreateNeutronAPI(neutronAPIName.Namespace, neutronAPIName.Name, spec))
				DeferCleanup(k8sClient.Delete, ctx, CreateNeutronAPISecret(namespace, SecretName))
				SimulateTransportURLReady(apiTransportURLName)
			})
			It("should not create a secret", func() {
				secret := types.NamespacedName{
					Namespace: neutronAPIName.Namespace,
					Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "config"),
				}
				th.AssertSecretDoesNotExist(secret)
			})
		})

		When("keystoneAPI instance is not available", func() {
			BeforeEach(func() {
				DeferCleanup(th.DeleteInstance, CreateNeutronAPI(neutronAPIName.Namespace, neutronAPIName.Name, spec))
				DeferCleanup(k8sClient.Delete, ctx, CreateNeutronAPISecret(namespace, SecretName))
				SimulateTransportURLReady(apiTransportURLName)
				DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
			})
			It("should not create a secret", func() {
				secret := types.NamespacedName{
					Namespace: neutronAPIName.Namespace,
					Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "config"),
				}
				th.AssertSecretDoesNotExist(secret)
			})
		})

		When("required dependency services are available", func() {
			BeforeEach(func() {
				spec["customServiceConfig"] = "[DEFAULT]\ndebug=True"
				spec["notificationsBusInstance"] = "rabbitmq-br"
				notificationsTransportURLName := types.NamespacedName{
					Namespace: namespace,
					Name:      "notifications-neutron-transport",
				}
				DeferCleanup(th.DeleteInstance, CreateNeutronAPI(neutronAPIName.Namespace, neutronAPIName.Name, spec))
				DeferCleanup(k8sClient.Delete, ctx, CreateNeutronAPISecret(namespace, SecretName))
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
				SimulateTransportURLReady(notificationsTransportURLName)
				mariadb.SimulateMariaDBAccountCompleted(types.NamespacedName{Namespace: namespace, Name: GetNeutronAPI(neutronAPIName).Spec.DatabaseAccount})
				mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: neutronapi.DatabaseCRName})
			})

			It("should create a Secret for 01-neutron.conf with the auth_url config option set based on the KeystoneAPI", func() {
				if isOVNEnabled {
					DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
				}
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

			It("should create a Secret for 01-neutron.conf with notifications, api and rpc workers and my.cnf", func() {
				if isOVNEnabled {
					DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
				}
				keystoneAPI := keystone.CreateKeystoneAPI(namespace)
				DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPI)

				secret := types.NamespacedName{
					Namespace: neutronAPIName.Namespace,
					Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "config"),
				}

				Eventually(func() corev1.Secret {
					return th.GetSecret(secret)
				}, timeout, interval).ShouldNot(BeNil())

				configData := th.GetSecret(secret)
				Expect(configData).ShouldNot(BeNil())
				conf := string(configData.Data["01-neutron.conf"])
				Expect(conf).Should(
					ContainSubstring("api_workers = 2"))
				Expect(conf).Should(
					ContainSubstring("rpc_workers = 1"))
				Expect(conf).Should(
					ContainSubstring("mysql_wsrep_sync_wait = 1"))

				databaseAccount := mariadb.GetMariaDBAccount(types.NamespacedName{Namespace: namespace, Name: GetNeutronAPI(neutronAPIName).Spec.DatabaseAccount})
				databaseSecret := th.GetSecret(types.NamespacedName{Namespace: namespace, Name: databaseAccount.Spec.Secret})

				Expect(conf).Should(
					ContainSubstring(
						fmt.Sprintf(
							"connection=mysql+pymysql://%s:%s@hostname-for-test-neutron-db-instance.%s.svc/neutron?read_default_file=/etc/my.cnf",
							databaseAccount.Spec.UserName,
							databaseSecret.Data[mariadbv1.DatabasePasswordSelector],
							namespace,
						)))

				myCnf := configData.Data["my.cnf"]
				Expect(myCnf).To(
					ContainSubstring("[client]\nssl=0"))

				// verify exernal notifications driver and transport_url key
				Expect(conf).Should(
					ContainSubstring("[oslo_messaging_notifications]\ndriver = messagingv2\ntransport_url ="))

			})

			It("should create a Secret for 01-neutron.conf with expected ml2 backend settings", func() {
				var dbs []types.NamespacedName
				if isOVNEnabled {
					dbs := CreateOVNDBClusters(namespace)
					DeferCleanup(DeleteOVNDBClusters, dbs)
				}
				keystoneAPI := keystone.CreateKeystoneAPI(namespace)
				DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPI)
				secret := types.NamespacedName{
					Namespace: neutronAPIName.Namespace,
					Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "config"),
				}

				Eventually(func() corev1.Secret {
					return th.GetSecret(secret)
				}, timeout, interval).ShouldNot(BeNil())

				data := th.GetSecret(secret).Data["01-neutron.conf"]
				Expect(data).Should(
					ContainSubstring(
						fmt.Sprintf("mechanism_drivers = %s", strings.Join(ml2MechanismDrivers, ","))))

				if isOVNEnabled {
					Expect(data).Should(ContainSubstring("ovn-router"))
					for _, db := range dbs {
						ovndb := GetOVNDBCluster(db)
						Expect(data).Should(ContainSubstring("ovn_%s_connection = %s", strings.ToLower(string(ovndb.Spec.DBType)), ovndb.Status.InternalDBAddress))
					}
				} else {
					Expect(data).ShouldNot(ContainSubstring("ovn-router"))
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

			It("should create a Secret for 10-neutron-httpd.conf with Timeout set", func() {
				if isOVNEnabled {
					DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
				}
				keystoneAPI := keystone.CreateKeystoneAPI(namespace)
				DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPI)

				secret := types.NamespacedName{
					Namespace: neutronAPIName.Namespace,
					Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "httpd-config"),
				}

				Eventually(func() corev1.Secret {
					return th.GetSecret(secret)
				}, timeout, interval).ShouldNot(BeNil())

				configData := th.GetSecret(secret)
				Expect(configData).ShouldNot(BeNil())
				conf := string(configData.Data["10-neutron-httpd.conf"])
				Expect(conf).Should(
					ContainSubstring("TimeOut 120"))
			})

			It("should create secret with OwnerReferences set", func() {
				if isOVNEnabled {
					dbs := CreateOVNDBClusters(namespace)
					DeferCleanup(DeleteOVNDBClusters, dbs)
				}
				keystoneAPI := keystone.CreateKeystoneAPI(namespace)
				DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPI)
				secret := types.NamespacedName{
					Namespace: neutronAPIName.Namespace,
					Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "config"),
				}

				Eventually(func() corev1.Secret {
					return th.GetSecret(secret)
				}, timeout, interval).ShouldNot(BeNil())

				// Check OwnerReferences set correctly for the Config Map
				Expect(th.GetSecret(secret).ObjectMeta.OwnerReferences[0].Name).To(Equal(neutronAPIName.Name))
				Expect(th.GetSecret(secret).ObjectMeta.OwnerReferences[0].Kind).To(Equal("NeutronAPI"))
			})

			It("should create a Secret for 02-neutron-custom.conf with customServiceConfig input", func() {
				if isOVNEnabled {
					DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
				}
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

			It("updates the KeystoneAuthURL if keystone internal endpoint changes", func() {
				if isOVNEnabled {
					DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
				}

				keystoneAPIName := keystone.CreateKeystoneAPI(namespace)
				DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPIName)

				newInternalEndpoint := "https://keystone-internal"

				keystone.UpdateKeystoneAPIEndpoint(keystoneAPIName, "internal", newInternalEndpoint)
				logger.Info("Reconfigured")

				secret := types.NamespacedName{
					Namespace: neutronAPIName.Namespace,
					Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "config"),
				}

				Eventually(func(g Gomega) {
					confSecret := th.GetSecret(secret)
					g.Expect(confSecret).ShouldNot(BeNil())

					conf := string(confSecret.Data["01-neutron.conf"])
					g.Expect(string(conf)).Should(
						ContainSubstring("auth_url = %s", newInternalEndpoint))
				}, timeout, interval).Should(Succeed())
			})

			if isOVNEnabled {
				It("should create an external Metadata Agent Secret with expected ovn_sb_connection set", func() {
					dbs := CreateOVNDBClusters(namespace)
					DeferCleanup(DeleteOVNDBClusters, dbs)

					externalSBEndpoint := "10.0.0.254"
					SetExternalDBEndpoint(dbs[1], externalSBEndpoint)

					externalMetadataAgentSecret := types.NamespacedName{
						Namespace: neutronAPIName.Namespace,
						Name:      fmt.Sprintf("%s-ovn-metadata-agent-neutron-config", neutronAPIName.Name),
					}

					Eventually(func(g Gomega) {
						g.Expect(th.GetSecret(externalMetadataAgentSecret).Data[neutronapi.NeutronOVNMetadataAgentSecretKey]).Should(
							ContainSubstring("ovn_sb_connection = %s", externalSBEndpoint))
					}, timeout, interval).Should(Succeed())

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

					Eventually(func(g Gomega) {
						g.Expect(th.GetSecret(externalMetadataAgentSecret).Data[neutronapi.NeutronOVNMetadataAgentSecretKey]).Should(
							ContainSubstring("ovn_sb_connection = %s", externalSBEndpoint))
					}, timeout, interval).Should(Succeed())

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
			}

			It("should create a Secret for neutron.conf with memcached servers set", func() {
				if isOVNEnabled {
					DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
				}
				keystoneAPI := keystone.CreateKeystoneAPI(namespace)
				DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPI)

				secret := types.NamespacedName{
					Namespace: neutronAPIName.Namespace,
					Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "config"),
				}

				Eventually(func() corev1.Secret {
					return th.GetSecret(secret)
				}, timeout, interval).ShouldNot(BeNil())
				memcacheInstance := infra.GetMemcached(memcachedName)
				neutronCfg := string(th.GetSecret(secret).Data["01-neutron.conf"])
				if memcacheInstance.GetMemcachedTLSSupport() {
					Expect(neutronCfg).Should(
						ContainSubstring("backend = oslo_cache.memcache_pool"))
					Expect(neutronCfg).Should(
						ContainSubstring(fmt.Sprintf("memcache_servers = %s", memcacheInstance.GetMemcachedServerListString())))
					Expect(neutronCfg).Should(
						ContainSubstring(fmt.Sprintf("memcached_servers=%s", memcacheInstance.GetMemcachedServerListString())))
				} else {
					Expect(neutronCfg).Should(
						ContainSubstring("backend = dogpile.cache.memcached"))
					Expect(neutronCfg).Should(
						ContainSubstring(fmt.Sprintf("memcache_servers = %s", memcacheInstance.GetMemcachedServerListWithInetString())))
					Expect(neutronCfg).Should(
						ContainSubstring(fmt.Sprintf("memcached_servers=%s", memcacheInstance.GetMemcachedServerListString())))
				}
			})

			It("should create a Secret for 01-neutron.conf with quorum queues config when transporturl has quorumqueues=true", func() {
				if isOVNEnabled {
					DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
				}
				keystoneAPI := keystone.CreateKeystoneAPI(namespace)
				DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPI)

				// Get the existing secret first to obtain the ResourceVersion
				existingSecret := &corev1.Secret{}
				secretKey := types.NamespacedName{
					Name:      SecretName,
					Namespace: namespace,
				}
				Expect(k8sClient.Get(ctx, secretKey, existingSecret)).Should(Succeed())

				// Update the secret with quorumqueues=true
				existingSecret.Data["quorumqueues"] = []byte("true")
				Expect(k8sClient.Update(ctx, existingSecret)).Should(Succeed())

				secret := types.NamespacedName{
					Namespace: neutronAPIName.Namespace,
					Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "config"),
				}

				// Wait for the configuration to be regenerated after the secret update
				Eventually(func(g Gomega) {
					configData := th.GetSecret(secret)
					g.Expect(configData).ShouldNot(BeNil())
					conf := string(configData.Data["01-neutron.conf"])
					g.Expect(conf).Should(ContainSubstring("[oslo_messaging_rabbit]"))
				}, timeout, interval).Should(Succeed())

				configData := th.GetSecret(secret)
				Expect(configData).ShouldNot(BeNil())
				conf := string(configData.Data["01-neutron.conf"])

				// Verify that the quorum queues configuration is present
				Expect(conf).Should(ContainSubstring("[oslo_messaging_rabbit]"))
				Expect(conf).Should(ContainSubstring("rabbit_quorum_queue=true"))
				Expect(conf).Should(ContainSubstring("rabbit_transient_quorum_queue=true"))
				Expect(conf).Should(ContainSubstring("amqp_durable_queues=true"))
			})

			if isOVNEnabled {
				It("should create an external OVN Agent Secret with expected ovn nb and sb connection set", func() {
					dbs := CreateOVNDBClusters(namespace)
					DeferCleanup(DeleteOVNDBClusters, dbs)

					externalNBEndpoint := "10.0.0.253"
					SetExternalDBEndpoint(dbs[0], externalNBEndpoint)
					externalSBEndpoint := "10.0.0.254"
					SetExternalDBEndpoint(dbs[1], externalSBEndpoint)

					externalOVNAgentSecret := types.NamespacedName{
						Namespace: neutronAPIName.Namespace,
						Name:      fmt.Sprintf("%s-ovn-agent-neutron-config", neutronAPIName.Name),
					}

					Eventually(func(g Gomega) {
						g.Expect(th.GetSecret(externalOVNAgentSecret).Data[neutronapi.NeutronOVNAgentSecretKey]).Should(
							ContainSubstring("ovn_nb_connection = %s", externalNBEndpoint))
						g.Expect(th.GetSecret(externalOVNAgentSecret).Data[neutronapi.NeutronOVNAgentSecretKey]).Should(
							ContainSubstring("ovn_sb_connection = %s", externalSBEndpoint))
					}, timeout, interval).Should(Succeed())

					Eventually(func(g Gomega) {
						NeutronAPI := GetNeutronAPI(neutronAPIName)
						g.Expect(NeutronAPI.Status.Hash[externalOVNAgentSecret.Name]).NotTo(BeEmpty())
					}, timeout, interval).Should(Succeed())
				})

				It("should delete OVN Agent external Secret once NB or SB DBClusters are deleted", func() {
					dbs := CreateOVNDBClusters(namespace)
					DeferCleanup(DeleteOVNDBClusters, dbs)
					externalNBEndpoint := "10.0.0.253"
					SetExternalDBEndpoint(dbs[0], externalNBEndpoint)

					externalSBEndpoint := "10.0.0.254"
					SetExternalDBEndpoint(dbs[1], externalSBEndpoint)

					externalOVNAgentSecret := types.NamespacedName{
						Namespace: neutronAPIName.Namespace,
						Name:      fmt.Sprintf("%s-ovn-agent-neutron-config", neutronAPIName.Name),
					}

					Eventually(func() corev1.Secret {
						return th.GetSecret(externalOVNAgentSecret)
					}, timeout, interval).ShouldNot(BeNil())

					Eventually(func(g Gomega) {
						NeutronAPI := GetNeutronAPI(neutronAPIName)
						g.Expect(NeutronAPI.Status.Hash[externalOVNAgentSecret.Name]).NotTo(BeEmpty())
					}, timeout, interval).Should(Succeed())

					DeleteOVNDBClusters([]types.NamespacedName{dbs[1]})

					Eventually(func(g Gomega) {
						secret := &corev1.Secret{}
						g.Expect(k8sClient.Get(ctx, externalOVNAgentSecret, secret)).Should(HaveOccurred())
					}, timeout, interval).Should(Succeed())

					Eventually(func(g Gomega) {
						NeutronAPI := GetNeutronAPI(neutronAPIName)
						g.Expect(NeutronAPI.Status.Hash[externalOVNAgentSecret.Name]).To(BeEmpty())
					}, timeout, interval).Should(Succeed())
				})

				It("should update Neutron OVN Agent Secret once NB or SB DBCluster is updated", func() {
					dbs := CreateOVNDBClusters(namespace)
					DeferCleanup(DeleteOVNDBClusters, dbs)
					externalNBEndpoint := "10.0.0.253"
					SetExternalDBEndpoint(dbs[0], externalNBEndpoint)

					externalSBEndpoint := "10.0.0.254"
					SetExternalDBEndpoint(dbs[1], externalSBEndpoint)

					externalOVNAgentSecret := types.NamespacedName{
						Namespace: neutronAPIName.Namespace,
						Name:      fmt.Sprintf("%s-ovn-agent-neutron-config", neutronAPIName.Name),
					}

					Eventually(func(g Gomega) {
						g.Expect(th.GetSecret(externalOVNAgentSecret).Data[neutronapi.NeutronOVNAgentSecretKey]).Should(
							ContainSubstring("ovn_sb_connection = %s", externalSBEndpoint))
					}, timeout, interval).Should(Succeed())

					Eventually(func(g Gomega) {
						NeutronAPI := GetNeutronAPI(neutronAPIName)
						initialHash := NeutronAPI.Status.Hash[externalOVNAgentSecret.Name]
						g.Expect(initialHash).NotTo(BeEmpty())
					}, timeout, interval).Should(Succeed())

					NeutronAPI := GetNeutronAPI(neutronAPIName)
					initialHash := NeutronAPI.Status.Hash[externalOVNAgentSecret.Name]

					newExternalNBEndpoint := "10.0.0.249"
					SetExternalDBEndpoint(dbs[0], newExternalNBEndpoint)
					newExternalSBEndpoint := "10.0.0.250"
					SetExternalDBEndpoint(dbs[1], newExternalSBEndpoint)

					Eventually(func(g Gomega) {
						g.Expect(th.GetSecret(externalOVNAgentSecret).Data[neutronapi.NeutronOVNAgentSecretKey]).Should(
							ContainSubstring("ovn_nb_connection = %s", newExternalNBEndpoint))
					}, timeout, interval).Should(Succeed())
					Eventually(func(g Gomega) {
						g.Expect(th.GetSecret(externalOVNAgentSecret).Data[neutronapi.NeutronOVNAgentSecretKey]).Should(
							ContainSubstring("ovn_sb_connection = %s", newExternalSBEndpoint))
					}, timeout, interval).Should(Succeed())

					Eventually(func(g Gomega) {
						NeutronAPI := GetNeutronAPI(neutronAPIName)
						newHash := NeutronAPI.Status.Hash[externalOVNAgentSecret.Name]
						g.Expect(newHash).NotTo(Equal(initialHash))
					}, timeout, interval).Should(Succeed())
				})
			}
		})

		When("DB is created", func() {
			BeforeEach(func() {
				DeferCleanup(th.DeleteInstance, CreateNeutronAPI(neutronAPIName.Namespace, neutronAPIName.Name, spec))
				DeferCleanup(k8sClient.Delete, ctx, CreateNeutronAPISecret(namespace, SecretName))
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
				mariadb.SimulateMariaDBAccountCompleted(types.NamespacedName{Namespace: namespace, Name: GetNeutronAPI(neutronAPIName).Spec.DatabaseAccount})
				mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: neutronapi.DatabaseCRName})
				th.SimulateJobSuccess(neutronDBSyncJobName)
				NeutronAPI := GetNeutronAPI(neutronAPIName)
				hostname := "hostname-for-" + NeutronAPI.Spec.DatabaseInstance + "." + namespace + ".svc"
				Expect(NeutronAPI.Status.DatabaseHostname).To(Equal(hostname))
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
				DeferCleanup(th.DeleteInstance, CreateNeutronAPI(neutronAPIName.Namespace, neutronAPIName.Name, spec))
				DeferCleanup(k8sClient.Delete, ctx, CreateNeutronAPISecret(namespace, SecretName))
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
				mariadb.SimulateMariaDBAccountCompleted(types.NamespacedName{Namespace: namespace, Name: GetNeutronAPI(neutronAPIName).Spec.DatabaseAccount})
				mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: neutronapi.DatabaseCRName})
				th.SimulateJobSuccess(neutronDBSyncJobName)
				keystone.SimulateKeystoneServiceReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
				keystone.SimulateKeystoneEndpointReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
			})
			It("Should set CreateServiceReadyCondition Condition", func() {

				th.ExpectCondition(
					neutronAPIName,
					ConditionGetterFunc(NeutronAPIConditionGetter),
					condition.CreateServiceReadyCondition,
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
				Expect(nSvcContainer.VolumeMounts).To(HaveLen(2))
				Expect(nSvcContainer.Image).To(Equal(util.GetEnvVar("RELATED_IMAGE_NEUTRON_API_IMAGE_URL_DEFAULT", neutronv1.NeutronAPIContainerImage)))

				nHttpdProxyContainer := deployment.Spec.Template.Spec.Containers[1]
				Expect(nHttpdProxyContainer.LivenessProbe.HTTPGet.Port.IntVal).To(Equal(int32(9696)))
				Expect(nHttpdProxyContainer.ReadinessProbe.HTTPGet.Port.IntVal).To(Equal(int32(9696)))
				Expect(nHttpdProxyContainer.VolumeMounts).To(HaveLen(2))
				Expect(nHttpdProxyContainer.Image).To(Equal(util.GetEnvVar("RELATED_IMAGE_NEUTRON_API_IMAGE_URL_DEFAULT", neutronv1.NeutronAPIContainerImage)))
			})
		})

		When("Deployment rollout is progressing", func() {
			deplName := types.NamespacedName{}
			BeforeEach(func() {
				deplName = types.NamespacedName{
					Namespace: namespace,
					Name:      "neutron",
				}

				DeferCleanup(th.DeleteInstance, CreateNeutronAPI(neutronAPIName.Namespace, neutronAPIName.Name, spec))

				DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(caBundleSecretName))
				DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(internalCertSecretName))
				DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(publicCertSecretName))
				DeferCleanup(k8sClient.Delete, ctx, CreateNeutronAPISecret(namespace, SecretName))
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
				infra.SimulateTLSMemcachedReady(memcachedName)
				DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
				DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(namespace))
				mariadb.SimulateMariaDBAccountCompleted(types.NamespacedName{Namespace: namespace, Name: GetNeutronAPI(neutronAPIName).Spec.DatabaseAccount})
				mariadb.SimulateMariaDBTLSDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: neutronapi.Database})
				th.SimulateJobSuccess(neutronDBSyncJobName)
				keystone.SimulateKeystoneServiceReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
				keystone.SimulateKeystoneEndpointReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})

				th.SimulateDeploymentProgressing(deplName)
			})

			It("shows the deployment progressing in DeploymentReadyCondition", func() {
				th.ExpectConditionWithDetails(
					neutronAPIName,
					ConditionGetterFunc(NeutronAPIConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionFalse,
					condition.RequestedReason,
					condition.DeploymentReadyRunningMessage,
				)
				th.ExpectCondition(
					neutronAPIName,
					ConditionGetterFunc(NeutronAPIConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionFalse,
				)
			})

			It("still shows the deployment progressing in DeploymentReadyCondition when rollout hits ProgressDeadlineExceeded", func() {
				th.SimulateDeploymentProgressDeadlineExceeded(deplName)
				th.ExpectConditionWithDetails(
					neutronAPIName,
					ConditionGetterFunc(NeutronAPIConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionFalse,
					condition.RequestedReason,
					condition.DeploymentReadyRunningMessage,
				)
				th.ExpectCondition(
					neutronAPIName,
					ConditionGetterFunc(NeutronAPIConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionFalse,
				)
			})

			It("reaches Ready when deployment rollout finished", func() {
				th.ExpectConditionWithDetails(
					neutronAPIName,
					ConditionGetterFunc(NeutronAPIConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionFalse,
					condition.RequestedReason,
					condition.DeploymentReadyRunningMessage,
				)
				th.ExpectCondition(
					neutronAPIName,
					ConditionGetterFunc(NeutronAPIConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionFalse,
				)

				th.SimulateDeploymentReplicaReady(deplName)
				th.ExpectCondition(
					neutronAPIName,
					ConditionGetterFunc(NeutronAPIConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionTrue,
				)

				th.ExpectCondition(
					neutronAPIName,
					ConditionGetterFunc(NeutronAPIConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionTrue,
				)
			})
		})

		When("NeutronAPI is created with networkAttachments", func() {
			BeforeEach(func() {
				spec["networkAttachments"] = []string{"internalapi"}
				DeferCleanup(th.DeleteInstance, CreateNeutronAPI(neutronAPIName.Namespace, neutronAPIName.Name, spec))
				DeferCleanup(k8sClient.Delete, ctx, CreateNeutronAPISecret(namespace, SecretName))
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

			It("reports that the definition is missing", func() {
				th.ExpectConditionWithDetails(
					neutronAPIName,
					ConditionGetterFunc(NeutronAPIConditionGetter),
					condition.NetworkAttachmentsReadyCondition,
					corev1.ConditionFalse,
					condition.ErrorReason,
					"NetworkAttachment resources missing: internalapi",
				)
			})
			It("reports that network attachment is missing", func() {

				internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
				nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
				DeferCleanup(th.DeleteInstance, nad)
				mariadb.SimulateMariaDBAccountCompleted(types.NamespacedName{Namespace: namespace, Name: GetNeutronAPI(neutronAPIName).Spec.DatabaseAccount})
				mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: neutronapi.DatabaseCRName})
				th.SimulateJobSuccess(neutronDBSyncJobName)
				keystone.SimulateKeystoneServiceReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
				keystone.SimulateKeystoneEndpointReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})

				deplName := types.NamespacedName{
					Namespace: namespace,
					Name:      "neutron",
				}
				depl := th.GetDeployment(deplName)
				th.SimulateDeploymentReadyWithPods(deplName, map[string][]string{})

				expectedAnnotation, err := json.Marshal(
					[]networkv1.NetworkSelectionElement{
						{
							Name:             "internalapi",
							Namespace:        namespace,
							InterfaceRequest: "internalapi",
						}})
				Expect(err).ShouldNot(HaveOccurred())
				Expect(depl.Spec.Template.ObjectMeta.Annotations).To(
					HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
				)

				// We don't add network attachment status annotations to the Pods
				// to simulate that the network attachments are missing.
				//th.SimulateDeploymentReadyWithPods(deplName, map[string][]string{})

				th.ExpectConditionWithDetails(
					neutronAPIName,
					ConditionGetterFunc(NeutronAPIConditionGetter),
					condition.NetworkAttachmentsReadyCondition,
					corev1.ConditionFalse,
					condition.ErrorReason,
					"NetworkAttachments error occurred "+
						"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
				)
			})
			It("reports that an IP is missing", func() {

				internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
				nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
				DeferCleanup(th.DeleteInstance, nad)
				mariadb.SimulateMariaDBAccountCompleted(types.NamespacedName{Namespace: namespace, Name: GetNeutronAPI(neutronAPIName).Spec.DatabaseAccount})
				mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: neutronapi.DatabaseCRName})
				th.SimulateJobSuccess(neutronDBSyncJobName)
				keystone.SimulateKeystoneServiceReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
				keystone.SimulateKeystoneEndpointReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
				deplName := types.NamespacedName{
					Namespace: namespace,
					Name:      "neutron",
				}
				depl := th.GetDeployment(deplName)

				expectedAnnotation, err := json.Marshal(
					[]networkv1.NetworkSelectionElement{
						{
							Name:             "internalapi",
							Namespace:        namespace,
							InterfaceRequest: "internalapi",
						}})
				Expect(err).ShouldNot(HaveOccurred())
				Expect(depl.Spec.Template.ObjectMeta.Annotations).To(
					HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
				)

				// We simulate that there is no IP associated with the internalapi
				// network attachment
				th.SimulateDeploymentReadyWithPods(
					deplName,
					map[string][]string{namespace + "/internalapi": {}},
				)

				th.ExpectConditionWithDetails(
					neutronAPIName,
					ConditionGetterFunc(NeutronAPIConditionGetter),
					condition.NetworkAttachmentsReadyCondition,
					corev1.ConditionFalse,
					condition.ErrorReason,
					"NetworkAttachments error occurred "+
						"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
				)
			})
			It("reports NetworkAttachmentsReady if the Pods got the proper annotations", func() {

				internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
				nad := th.CreateNetworkAttachmentDefinition(internalAPINADName)
				DeferCleanup(th.DeleteInstance, nad)
				mariadb.SimulateMariaDBAccountCompleted(types.NamespacedName{Namespace: namespace, Name: GetNeutronAPI(neutronAPIName).Spec.DatabaseAccount})
				mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: neutronapi.DatabaseCRName})
				th.SimulateJobSuccess(neutronDBSyncJobName)
				keystone.SimulateKeystoneServiceReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
				keystone.SimulateKeystoneEndpointReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})

				deplName := types.NamespacedName{
					Namespace: namespace,
					Name:      "neutron",
				}
				th.SimulateDeploymentReadyWithPods(
					deplName,
					map[string][]string{namespace + "/internalapi": {"10.0.0.1"}},
				)

				th.ExpectCondition(
					neutronAPIName,
					ConditionGetterFunc(NeutronAPIConditionGetter),
					condition.NetworkAttachmentsReadyCondition,
					corev1.ConditionTrue,
				)

				Eventually(func(g Gomega) {
					NeutronAPI := GetNeutronAPI(neutronAPIName)
					g.Expect(NeutronAPI.Status.NetworkAttachments).To(
						Equal(map[string][]string{namespace + "/internalapi": {"10.0.0.1"}}))

				}, timeout, interval).Should(Succeed())
			})
		})

		When("A NeutronAPI is created with TLS", func() {
			BeforeEach(func() {
				spec["tls"] = map[string]any{
					"api": map[string]any{
						"internal": map[string]any{
							"secretName": InternalCertSecretName,
						},
						"public": map[string]any{
							"secretName": PublicCertSecretName,
						},
					},
					"caBundleSecretName": CABundleSecretName,
					"ovn": map[string]any{
						"secretName": InternalCertSecretName,
					},
				}
				DeferCleanup(th.DeleteInstance, CreateNeutronAPI(neutronAPIName.Namespace, neutronAPIName.Name, spec))

				DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(caBundleSecretName))
				DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(internalCertSecretName))
				DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(publicCertSecretName))
				DeferCleanup(k8sClient.Delete, ctx, CreateNeutronAPISecret(namespace, SecretName))
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
				infra.SimulateTLSMemcachedReady(memcachedName)
				DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
				DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(namespace))
				mariadb.SimulateMariaDBAccountCompleted(types.NamespacedName{Namespace: namespace, Name: GetNeutronAPI(neutronAPIName).Spec.DatabaseAccount})
				mariadb.SimulateMariaDBTLSDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: neutronapi.Database})
				th.SimulateJobSuccess(neutronDBSyncJobName)
				keystone.SimulateKeystoneServiceReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
				keystone.SimulateKeystoneEndpointReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
			})

			It("it creates deployment with certs mounted", func() {
				th.ExpectCondition(
					neutronAPIName,
					ConditionGetterFunc(NeutronAPIConditionGetter),
					condition.TLSInputReadyCondition,
					corev1.ConditionTrue,
				)

				deployment := th.GetDeployment(
					types.NamespacedName{
						Namespace: neutronAPIName.Namespace,
						Name:      "neutron",
					},
				)

				// cert deployment volumes
				th.AssertVolumeExists(caBundleSecretName.Name, deployment.Spec.Template.Spec.Volumes)
				th.AssertVolumeExists(internalCertSecretName.Name, deployment.Spec.Template.Spec.Volumes)
				th.AssertVolumeExists(publicCertSecretName.Name, deployment.Spec.Template.Spec.Volumes)
				if isOVNEnabled {
					th.AssertVolumeExists(ovnDbCertSecretName.Name, deployment.Spec.Template.Spec.Volumes)
				}

				// svc container ca cert
				nSvcContainer := deployment.Spec.Template.Spec.Containers[0]
				th.AssertVolumeMountPathExists(caBundleSecretName.Name, "", "tls-ca-bundle.pem", nSvcContainer.VolumeMounts)
				if isOVNEnabled {
					th.AssertVolumeMountPathExists(ovnDbCertSecretName.Name, "", "tls.key", nSvcContainer.VolumeMounts)
					th.AssertVolumeMountPathExists(ovnDbCertSecretName.Name, "", "tls.crt", nSvcContainer.VolumeMounts)
					th.AssertVolumeMountPathExists(ovnDbCertSecretName.Name, "", "ca.crt", nSvcContainer.VolumeMounts)
				}

				// httpd container certs
				nHttpdProxyContainer := deployment.Spec.Template.Spec.Containers[1]
				th.AssertVolumeMountPathExists(publicCertSecretName.Name, "", "tls.key", nHttpdProxyContainer.VolumeMounts)
				th.AssertVolumeMountPathExists(publicCertSecretName.Name, "", "tls.crt", nHttpdProxyContainer.VolumeMounts)
				th.AssertVolumeMountPathExists(internalCertSecretName.Name, "", "tls.key", nHttpdProxyContainer.VolumeMounts)
				th.AssertVolumeMountPathExists(internalCertSecretName.Name, "", "tls.crt", nHttpdProxyContainer.VolumeMounts)
				th.AssertVolumeMountPathExists(caBundleSecretName.Name, "", "tls-ca-bundle.pem", nHttpdProxyContainer.VolumeMounts)

				Expect(nHttpdProxyContainer.ReadinessProbe.HTTPGet.Scheme).To(Equal(corev1.URISchemeHTTPS))
				Expect(nHttpdProxyContainer.LivenessProbe.HTTPGet.Scheme).To(Equal(corev1.URISchemeHTTPS))

				secret := types.NamespacedName{
					Namespace: neutronAPIName.Namespace,
					Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "config"),
				}

				configData := th.GetSecret(secret)
				Expect(configData).ShouldNot(BeNil())
				conf := string(configData.Data["01-neutron.conf"])
				Expect(conf).Should(
					ContainSubstring("api_workers = 2"))
				Expect(conf).Should(
					ContainSubstring("rpc_workers = 1"))
				Expect(conf).Should(
					ContainSubstring("mysql_wsrep_sync_wait = 1"))

				databaseAccount := mariadb.GetMariaDBAccount(types.NamespacedName{Namespace: namespace, Name: GetNeutronAPI(neutronAPIName).Spec.DatabaseAccount})
				databaseSecret := th.GetSecret(types.NamespacedName{Namespace: namespace, Name: databaseAccount.Spec.Secret})

				Expect(conf).Should(
					ContainSubstring(
						fmt.Sprintf(
							"connection=mysql+pymysql://%s:%s@hostname-for-test-neutron-db-instance.%s.svc/neutron?read_default_file=/etc/my.cnf",
							databaseAccount.Spec.UserName,
							databaseSecret.Data[mariadbv1.DatabasePasswordSelector],
							namespace)))

				if isOVNEnabled {
					Expect(conf).Should(And(
						ContainSubstring("ovn_nb_private_key = /etc/pki/tls/private/ovndb.key"),
						ContainSubstring("ovn_nb_certificate = /etc/pki/tls/certs/ovndb.crt"),
						ContainSubstring("ovn_nb_ca_cert = /etc/pki/tls/certs/ovndbca.crt"),
						ContainSubstring("ovn_sb_private_key = /etc/pki/tls/private/ovndb.key"),
						ContainSubstring("ovn_sb_certificate = /etc/pki/tls/certs/ovndb.crt"),
						ContainSubstring("ovn_sb_ca_cert = /etc/pki/tls/certs/ovndbca.crt"),
					))
				} else {
					Expect(conf).ShouldNot(Or(
						ContainSubstring("ovn_nb_private_key = /etc/pki/tls/private/ovndb.key"),
						ContainSubstring("ovn_nb_certificate = /etc/pki/tls/certs/ovndb.crt"),
						ContainSubstring("ovn_nb_ca_cert = /etc/pki/tls/certs/ovndbca.crt"),
						ContainSubstring("ovn_sb_private_key = /etc/pki/tls/private/ovndb.key"),
						ContainSubstring("ovn_sb_certificate = /etc/pki/tls/certs/ovndb.crt"),
						ContainSubstring("ovn_sb_ca_cert = /etc/pki/tls/certs/ovndbca.crt"),
					))
				}

				conf = string(configData.Data["my.cnf"])
				Expect(conf).To(
					ContainSubstring("[client]\nssl-ca=/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem\nssl=1"))
			})

			It("TLS Endpoints are created", func() {

				th.ExpectCondition(
					neutronAPIName,
					ConditionGetterFunc(NeutronAPIConditionGetter),
					condition.KeystoneEndpointReadyCondition,
					corev1.ConditionTrue,
				)
				keystoneEndpoint := keystone.GetKeystoneEndpoint(types.NamespacedName{Namespace: namespace, Name: "neutron"})
				endpoints := keystoneEndpoint.Spec.Endpoints
				Expect(endpoints).To(HaveKeyWithValue("public", "https://neutron-public."+neutronAPIName.Namespace+".svc:9696"))
				Expect(endpoints).To(HaveKeyWithValue("internal", "https://neutron-internal."+neutronAPIName.Namespace+".svc:9696"))
			})

			It("reconfigures the neutron pod when CA changes", func() {
				// Grab the current config hash
				originalHash := GetEnvVarValue(
					th.GetDeployment(
						types.NamespacedName{
							Namespace: neutronAPIName.Namespace,
							Name:      "neutron",
						},
					).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
				Expect(originalHash).NotTo(BeEmpty())

				// Change the content of the CA secret
				th.UpdateSecret(caBundleSecretName, "tls-ca-bundle.pem", []byte("DifferentCAData"))

				// Assert that the deployment is updated
				Eventually(func(g Gomega) {
					newHash := GetEnvVarValue(
						th.GetDeployment(
							types.NamespacedName{
								Namespace: neutronAPIName.Namespace,
								Name:      "neutron",
							},
						).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
					g.Expect(newHash).NotTo(BeEmpty())
					g.Expect(newHash).NotTo(Equal(originalHash))
				}, timeout, interval).Should(Succeed())
			})
		})

		When("A NeutronAPI is created with TLS and service override endpointURL set", func() {
			BeforeEach(func() {
				spec["tls"] = map[string]any{
					"api": map[string]any{
						"internal": map[string]any{
							"secretName": InternalCertSecretName,
						},
						"public": map[string]any{
							"secretName": PublicCertSecretName,
						},
					},
					"caBundleSecretName": CABundleSecretName,
				}

				serviceOverride := map[string]any{}
				serviceOverride["public"] = map[string]any{
					"endpointURL": "https://neutron-openstack.apps-crc.testing",
				}

				spec["override"] = map[string]any{
					"service": serviceOverride,
				}

				DeferCleanup(th.DeleteInstance, CreateNeutronAPI(neutronAPIName.Namespace, neutronAPIName.Name, spec))

				DeferCleanup(k8sClient.Delete, ctx, th.CreateCABundleSecret(caBundleSecretName))
				DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(internalCertSecretName))
				DeferCleanup(k8sClient.Delete, ctx, th.CreateCertSecret(publicCertSecretName))
				DeferCleanup(k8sClient.Delete, ctx, CreateNeutronAPISecret(namespace, SecretName))
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
				infra.SimulateTLSMemcachedReady(memcachedName)
				DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))
				DeferCleanup(keystone.DeleteKeystoneAPI, keystone.CreateKeystoneAPI(namespace))
				mariadb.SimulateMariaDBAccountCompleted(types.NamespacedName{Namespace: namespace, Name: GetNeutronAPI(neutronAPIName).Spec.DatabaseAccount})
				mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: neutronapi.DatabaseCRName})
				th.SimulateJobSuccess(neutronDBSyncJobName)
				keystone.SimulateKeystoneServiceReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
				keystone.SimulateKeystoneEndpointReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
			})

			It("registers endpointURL as public neutron endpoint", func() {

				th.ExpectCondition(
					neutronAPIName,
					ConditionGetterFunc(NeutronAPIConditionGetter),
					condition.KeystoneEndpointReadyCondition,
					corev1.ConditionTrue,
				)
				keystoneEndpoint := keystone.GetKeystoneEndpoint(types.NamespacedName{Namespace: namespace, Name: "neutron"})
				endpoints := keystoneEndpoint.Spec.Endpoints
				Expect(endpoints).To(HaveKeyWithValue("public", "https://neutron-openstack.apps-crc.testing"))
				Expect(endpoints).To(HaveKeyWithValue("internal", "https://neutron-internal."+neutronAPIName.Namespace+".svc:9696"))
			})
		})

		When("A NeutronAPI is created with topologyRef", func() {
			var topologyRef, topologyRefAlt *topologyv1.TopoRef
			BeforeEach(func() {
				// Define the two topology references used in this test
				topologyRef = &topologyv1.TopoRef{
					Name:      neutronAPITopologies[0].Name,
					Namespace: neutronAPITopologies[0].Namespace,
				}
				topologyRefAlt = &topologyv1.TopoRef{
					Name:      neutronAPITopologies[1].Name,
					Namespace: neutronAPITopologies[1].Namespace,
				}

				// Create Test Topologies
				for _, t := range neutronAPITopologies {
					// Build the topology Spec
					topologySpec, _ := GetSampleTopologySpec(t.Name)
					infra.CreateTopology(t, topologySpec)
				}
				spec["topologyRef"] = map[string]any{
					"name": topologyRef.Name,
				}

				DeferCleanup(th.DeleteInstance, CreateNeutronAPI(neutronAPIName.Namespace, neutronAPIName.Name, spec))
				DeferCleanup(k8sClient.Delete, ctx, CreateNeutronAPISecret(namespace, SecretName))
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
				mariadb.SimulateMariaDBAccountCompleted(types.NamespacedName{Namespace: namespace, Name: GetNeutronAPI(neutronAPIName).Spec.DatabaseAccount})
				mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: neutronapi.DatabaseCRName})
				th.SimulateJobSuccess(neutronDBSyncJobName)
				keystone.SimulateKeystoneServiceReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
				keystone.SimulateKeystoneEndpointReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
			})
			It("sets topology in CR status", func() {
				Eventually(func(g Gomega) {
					tp := infra.GetTopology(types.NamespacedName{
						Name:      topologyRef.Name,
						Namespace: topologyRef.Namespace,
					})
					finalizers := tp.GetFinalizers()
					g.Expect(finalizers).To(HaveLen(1))
					neutron := GetNeutronAPI(neutronAPIName)
					g.Expect(neutron.Status.LastAppliedTopology).ToNot(BeNil())
					g.Expect(neutron.Status.LastAppliedTopology).To(Equal(topologyRef))
					g.Expect(finalizers).To(ContainElement(
						fmt.Sprintf("openstack.org/neutronapi-%s", neutronAPIName.Name)))
				}, timeout, interval).Should(Succeed())
			})
			It("sets topology in resource specs", func() {
				Eventually(func(g Gomega) {
					_, topologySpecObj := GetSampleTopologySpec(topologyRef.Name)
					g.Expect(th.GetDeployment(neutronDeploymentName).Spec.Template.Spec.TopologySpreadConstraints).ToNot(BeNil())
					g.Expect(th.GetDeployment(neutronDeploymentName).Spec.Template.Spec.TopologySpreadConstraints).To(Equal(topologySpecObj))
					g.Expect(th.GetDeployment(neutronDeploymentName).Spec.Template.Spec.Affinity).To(BeNil())
				}, timeout, interval).Should(Succeed())
			})
			It("updates topology when the reference changes", func() {
				Eventually(func(g Gomega) {
					neutron := GetNeutronAPI(neutronAPIName)
					neutron.Spec.TopologyRef.Name = neutronAPITopologies[1].Name
					g.Expect(k8sClient.Update(ctx, neutron)).To(Succeed())
				}, timeout, interval).Should(Succeed())

				Eventually(func(g Gomega) {
					th.SimulateJobSuccess(neutronDBSyncJobName)

					tp := infra.GetTopology(types.NamespacedName{
						Name:      topologyRefAlt.Name,
						Namespace: topologyRefAlt.Namespace,
					})
					finalizers := tp.GetFinalizers()
					g.Expect(finalizers).To(HaveLen(1))
					neutron := GetNeutronAPI(neutronAPIName)
					g.Expect(neutron.Status.LastAppliedTopology).ToNot(BeNil())
					g.Expect(neutron.Status.LastAppliedTopology).To(Equal(topologyRefAlt))
					g.Expect(finalizers).To(ContainElement(
						fmt.Sprintf("openstack.org/neutronapi-%s", neutronAPIName.Name)))
					// Verify the previous referenced topology has no finalizers
					tp = infra.GetTopology(types.NamespacedName{
						Name:      topologyRef.Name,
						Namespace: topologyRef.Namespace,
					})
					finalizers = tp.GetFinalizers()
					g.Expect(finalizers).To(BeEmpty())
				}, timeout, interval).Should(Succeed())
			})

			It("removes topologyRef from the spec", func() {
				Eventually(func(g Gomega) {
					neutron := GetNeutronAPI(neutronAPIName)
					// Remove the TopologyRef from the existing Neutron .Spec
					neutron.Spec.TopologyRef = nil
					g.Expect(k8sClient.Update(ctx, neutron)).To(Succeed())
				}, timeout, interval).Should(Succeed())

				Eventually(func(g Gomega) {
					th.SimulateJobSuccess(neutronDBSyncJobName)
					neutron := GetNeutronAPI(neutronAPIName)
					g.Expect(neutron.Status.LastAppliedTopology).Should(BeNil())
				}, timeout, interval).Should(Succeed())

				Eventually(func(g Gomega) {
					g.Expect(th.GetDeployment(neutronDeploymentName).Spec.Template.Spec.TopologySpreadConstraints).To(BeNil())
					g.Expect(th.GetDeployment(neutronDeploymentName).Spec.Template.Spec.Affinity).ToNot(BeNil())
				}, timeout, interval).Should(Succeed())

				// Verify the existing topologies have no finalizer anymore
				Eventually(func(g Gomega) {
					for _, topology := range neutronAPITopologies {
						tp := infra.GetTopology(types.NamespacedName{
							Name:      topology.Name,
							Namespace: topology.Namespace,
						})
						finalizers := tp.GetFinalizers()
						g.Expect(finalizers).To(BeEmpty())
					}
				}, timeout, interval).Should(Succeed())
			})
		})
		When("A NeutronAPI is created with extraMounts", func() {
			var neutronExtraMountsPath, neutronExtraMountsSecretName string
			BeforeEach(func() {
				neutronExtraMountsPath = "/var/log/foo"
				neutronExtraMountsSecretName = "foo"
				spec["extraMounts"] = GetExtraMounts(neutronExtraMountsSecretName, neutronExtraMountsPath)

				DeferCleanup(th.DeleteInstance, CreateNeutronAPI(neutronAPIName.Namespace, neutronAPIName.Name, spec))
				DeferCleanup(k8sClient.Delete, ctx, CreateNeutronAPISecret(namespace, SecretName))
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
				mariadb.SimulateMariaDBAccountCompleted(types.NamespacedName{Namespace: namespace, Name: GetNeutronAPI(neutronAPIName).Spec.DatabaseAccount})
				mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: neutronapi.DatabaseCRName})
				th.SimulateJobSuccess(neutronDBSyncJobName)
				keystone.SimulateKeystoneServiceReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
				keystone.SimulateKeystoneEndpointReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
			})

			It("Check extraMounts of the resulting Deployment", func() {
				// Get Neutron Deployment
				dp := th.GetDeployment(neutronDeploymentName)
				// Check the resulting deployment fields
				Expect(dp.Spec.Template.Spec.Volumes).To(HaveLen(3))
				Expect(dp.Spec.Template.Spec.Containers).To(HaveLen(2))
				// Get the neutron container
				container := dp.Spec.Template.Spec.Containers[0]
				// Fail if neutron doesn't have the right number of VolumeMounts
				// entries
				Expect(container.VolumeMounts).To(HaveLen(3))
				// Inspect VolumeMounts and make sure we have the Foo MountPath
				// provided through extraMounts
				th.AssertVolumeMountPathExists(neutronExtraMountsSecretName,
					neutronExtraMountsPath, "", container.VolumeMounts)
			})
		})

		When("A NeutronAPI is created with nodeSelector", func() {
			BeforeEach(func() {
				spec["nodeSelector"] = map[string]any{
					"foo": "bar",
				}

				DeferCleanup(th.DeleteInstance, CreateNeutronAPI(neutronAPIName.Namespace, neutronAPIName.Name, spec))
				DeferCleanup(k8sClient.Delete, ctx, CreateNeutronAPISecret(namespace, SecretName))
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
				mariadb.SimulateMariaDBAccountCompleted(types.NamespacedName{Namespace: namespace, Name: GetNeutronAPI(neutronAPIName).Spec.DatabaseAccount})
				mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: neutronapi.DatabaseCRName})
				th.SimulateJobSuccess(neutronDBSyncJobName)
				keystone.SimulateKeystoneServiceReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
				keystone.SimulateKeystoneEndpointReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
			})

			It("sets nodeSelector in resource specs", func() {
				Eventually(func(g Gomega) {
					g.Expect(th.GetDeployment(neutronDeploymentName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
					g.Expect(th.GetJob(neutronDBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				}, timeout, interval).Should(Succeed())
			})

			It("updates nodeSelector in resource specs when changed", func() {
				Eventually(func(g Gomega) {
					g.Expect(th.GetDeployment(neutronDeploymentName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
					g.Expect(th.GetJob(neutronDBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				}, timeout, interval).Should(Succeed())

				Eventually(func(g Gomega) {
					neutron := GetNeutronAPI(neutronAPIName)
					newNodeSelector := map[string]string{
						"foo2": "bar2",
					}
					neutron.Spec.NodeSelector = &newNodeSelector
					g.Expect(k8sClient.Update(ctx, neutron)).Should(Succeed())
				}, timeout, interval).Should(Succeed())

				Eventually(func(g Gomega) {
					th.SimulateJobSuccess(neutronDBSyncJobName)
					g.Expect(th.GetDeployment(neutronDeploymentName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo2": "bar2"}))
					g.Expect(th.GetJob(neutronDBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo2": "bar2"}))
				}, timeout, interval).Should(Succeed())
			})

			It("removes nodeSelector from resource specs when cleared", func() {
				Eventually(func(g Gomega) {
					g.Expect(th.GetDeployment(neutronDeploymentName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
					g.Expect(th.GetJob(neutronDBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				}, timeout, interval).Should(Succeed())

				Eventually(func(g Gomega) {
					neutron := GetNeutronAPI(neutronAPIName)
					emptyNodeSelector := map[string]string{}
					neutron.Spec.NodeSelector = &emptyNodeSelector
					g.Expect(k8sClient.Update(ctx, neutron)).Should(Succeed())
				}, timeout, interval).Should(Succeed())

				Eventually(func(g Gomega) {
					th.SimulateJobSuccess(neutronDBSyncJobName)
					g.Expect(th.GetDeployment(neutronDeploymentName).Spec.Template.Spec.NodeSelector).To(BeNil())
					g.Expect(th.GetJob(neutronDBSyncJobName).Spec.Template.Spec.NodeSelector).To(BeNil())
				}, timeout, interval).Should(Succeed())
			})

			It("removes nodeSelector from resource specs when nilled", func() {
				Eventually(func(g Gomega) {
					g.Expect(th.GetDeployment(neutronDeploymentName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
					g.Expect(th.GetJob(neutronDBSyncJobName).Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"foo": "bar"}))
				}, timeout, interval).Should(Succeed())

				Eventually(func(g Gomega) {
					neutron := GetNeutronAPI(neutronAPIName)
					neutron.Spec.NodeSelector = nil
					g.Expect(k8sClient.Update(ctx, neutron)).Should(Succeed())
				}, timeout, interval).Should(Succeed())

				Eventually(func(g Gomega) {
					th.SimulateJobSuccess(neutronDBSyncJobName)
					g.Expect(th.GetDeployment(neutronDeploymentName).Spec.Template.Spec.NodeSelector).To(BeNil())
					g.Expect(th.GetJob(neutronDBSyncJobName).Spec.Template.Spec.NodeSelector).To(BeNil())
				}, timeout, interval).Should(Succeed())
			})
		})

		// Run MariaDBAccount suite tests.  these are pre-packaged ginkgo tests
		// that exercise standard account create / update patterns that should be
		// common to all controllers that ensure MariaDBAccount CRs.
		mariadbSuite := &mariadb_test.MariaDBTestHarness{
			PopulateHarness: func(harness *mariadb_test.MariaDBTestHarness) {
				harness.Setup(
					"Neutron API",
					neutronAPIName.Namespace,
					neutronapi.Database,
					"openstack.org/neutronapi",
					mariadb,
					timeout,
					interval,
				)
			},
			// Generate a fully running Neutron service given an accountName
			// needs to make it all the way to the end where the mariadb finalizers
			// are removed from unused accounts since that's part of what we are testing
			SetupCR: func(accountName types.NamespacedName) {

				spec["databaseAccount"] = accountName.Name

				DeferCleanup(th.DeleteInstance, CreateNeutronAPI(neutronAPIName.Namespace, neutronAPIName.Name, spec))
				DeferCleanup(DeleteOVNDBClusters, CreateOVNDBClusters(namespace))

				secret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      SecretName,
						Namespace: GetNeutronAPI(neutronAPIName).Namespace,
					},
					Data: map[string][]byte{
						"NeutronPassword": []byte("12345678"),
						"transport_url":   []byte("rabbit://user@svc:1234"),
					},
				}
				Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
				SimulateTransportURLReady(apiTransportURLName)
				DeferCleanup(k8sClient.Delete, ctx, secret)
				DeferCleanup(infra.DeleteMemcached, infra.CreateMemcached(namespace, "memcached", memcachedSpec))
				infra.SimulateMemcachedReady(memcachedName)

				keystoneAPI := keystone.CreateKeystoneAPI(namespace)
				DeferCleanup(keystone.DeleteKeystoneAPI, keystoneAPI)

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

				mariadb.SimulateMariaDBAccountCompleted(accountName)
				mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: neutronapi.DatabaseCRName})
				th.SimulateJobSuccess(neutronDBSyncJobName)
				keystone.SimulateKeystoneServiceReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
				keystone.SimulateKeystoneEndpointReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})

				th.SimulateDeploymentReadyWithPods(
					neutronDeploymentName,
					map[string][]string{namespace + "/internalapi": {}},
				)

				// ensure deployment is fully ready; old account finalizers aren't
				// removed until we get here
				th.ExpectCondition(
					neutronAPIName,
					ConditionGetterFunc(NeutronAPIConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionTrue,
				)

			},
			// update to a new account name
			UpdateAccount: func(newAccountName types.NamespacedName) {
				Eventually(func(g Gomega) {
					NeutronAPI := GetNeutronAPI(neutronAPIName)
					NeutronAPI.Spec.DatabaseAccount = newAccountName.Name
					g.Expect(th.K8sClient.Update(ctx, NeutronAPI)).Should(Succeed())
				}, timeout, interval).Should(Succeed())

			},
			// delete NeutronAPI to exercise finalizer removal
			DeleteCR: func() {
				th.DeleteInstance(GetNeutronAPI(neutronAPIName))
			},
		}

		mariadbSuite.RunBasicSuite()

		mariadbSuite.RunURLAssertSuite(func(_ types.NamespacedName, username string, password string) {
			Eventually(func(g Gomega) {
				secret := types.NamespacedName{
					Namespace: neutronAPIName.Namespace,
					Name:      fmt.Sprintf("%s-%s", neutronAPIName.Name, "config"),
				}

				configData := th.GetSecret(secret)
				conf := string(configData.Data["01-neutron.conf"])

				g.Expect(conf).Should(
					ContainSubstring(
						fmt.Sprintf(
							"connection=mysql+pymysql://%s:%s@hostname-for-test-neutron-db-instance.%s.svc/neutron?read_default_file=/etc/my.cnf",
							username,
							password,
							namespace,
						)))
			}).Should(Succeed())
		})

		mariadbSuite.RunConfigHashSuite(func() string {
			return GetEnvVarValue(
				th.GetDeployment(
					types.NamespacedName{
						Namespace: neutronAPIName.Namespace,
						Name:      "neutron",
					},
				).Spec.Template.Spec.Containers[0].Env, "CONFIG_HASH", "")
		})
	}
}

var _ = Describe("NeutronAPI controller (ovn, implicit)", getNeutronAPIControllerSuite([]string{}))
var _ = Describe("NeutronAPI controller (ovn, explicit)", getNeutronAPIControllerSuite([]string{"ovn"}))
var _ = Describe("NeutronAPI controller (ovn, openvswitch)", getNeutronAPIControllerSuite([]string{"ovn", "openvswitch"}))
var _ = Describe("NeutronAPI controller (openvswitch)", getNeutronAPIControllerSuite([]string{"openvswitch"}))

var _ = Describe("NeutronAPI Webhook", func() {

	var apiTransportURLName types.NamespacedName
	var neutronAPIName types.NamespacedName
	var memcachedSpec memcachedv1.MemcachedSpec
	var memcachedName types.NamespacedName
	var name string

	BeforeEach(func() {
		name = fmt.Sprintf("neutron-%s", uuid.New().String())
		apiTransportURLName = types.NamespacedName{
			Namespace: namespace,
			Name:      name + "-neutron-transport",
		}

		neutronAPIName = types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}
		memcachedSpec = infra.GetDefaultMemcachedSpec()
		memcachedName = types.NamespacedName{
			Name:      "memcached",
			Namespace: namespace,
		}

		err := os.Setenv("OPERATOR_TEMPLATES", "../../templates")
		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects with wrong NeutronAPI service override endpoint type", func() {
		spec := GetDefaultNeutronAPISpec()
		spec["override"] = map[string]any{
			"service": map[string]any{
				"internal": map[string]any{},
				"wrooong":  map[string]any{},
			},
		}

		raw := map[string]any{
			"apiVersion": "neutron.openstack.org/v1beta1",
			"kind":       "NeutronAPI",
			"metadata": map[string]any{
				"name":      neutronAPIName.Name,
				"namespace": neutronAPIName.Namespace,
			},
			"spec": spec,
		}

		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(
			ContainSubstring(
				"invalid: spec.override.service[wrooong]: " +
					"Invalid value: \"wrooong\": invalid endpoint type: wrooong"),
		)
	})

	It("rejects NeutronAPI with wrong field in defaultConfigOverwrite", func() {
		spec := GetDefaultNeutronAPISpec()
		spec["defaultConfigOverwrite"] = map[string]any{
			"policy.yaml": "support",
			"policy.json": "not supported",
		}

		raw := map[string]any{
			"apiVersion": "neutron.openstack.org/v1beta1",
			"kind":       "NeutronAPI",
			"metadata": map[string]any{
				"name":      neutronAPIName.Name,
				"namespace": neutronAPIName.Namespace,
			},
			"spec": spec,
		}

		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(
			ContainSubstring(
				"invalid: spec.defaultConfigOverwrite: " +
					"Invalid value: \"policy.json\": " +
					"Only the following keys are valid: policy.yaml"),
		)
	})

	It("rejects a wrong TopologyRef on a different namespace", func() {
		spec := GetDefaultNeutronAPISpec()
		// Inject a topologyRef that points to a different namespace
		spec["topologyRef"] = map[string]any{
			"name":      "foo",
			"namespace": "bar",
		}
		raw := map[string]any{
			"apiVersion": "neutron.openstack.org/v1beta1",
			"kind":       "NeutronAPI",
			"metadata": map[string]any{
				"name":      neutronAPIName.Name,
				"namespace": neutronAPIName.Namespace,
			},
			"spec": spec,
		}
		unstructuredObj := &unstructured.Unstructured{Object: raw}
		_, err := controllerutil.CreateOrPatch(
			th.Ctx, th.K8sClient, unstructuredObj, func() error { return nil })
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(
			ContainSubstring(
				"spec.topologyRef.namespace: Invalid value: \"namespace\": Customizing namespace field is not supported"),
		)
	})

	When("A NeutronAPI instance is updated with unsupported fields", func() {
		BeforeEach(func() {
			spec := GetDefaultNeutronAPISpec()
			DeferCleanup(th.DeleteInstance, CreateNeutronAPI(neutronAPIName.Namespace, neutronAPIName.Name, spec))
			DeferCleanup(k8sClient.Delete, ctx, CreateNeutronAPISecret(namespace, SecretName))
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
			mariadb.SimulateMariaDBAccountCompleted(types.NamespacedName{Namespace: namespace, Name: GetNeutronAPI(neutronAPIName).Spec.DatabaseAccount})
			mariadb.SimulateMariaDBDatabaseCompleted(types.NamespacedName{Namespace: namespace, Name: neutronapi.DatabaseCRName})
			th.SimulateJobSuccess(types.NamespacedName{Namespace: namespace, Name: neutronAPIName.Name + "-db-sync"})
			keystone.SimulateKeystoneServiceReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
			keystone.SimulateKeystoneEndpointReady(types.NamespacedName{Namespace: namespace, Name: "neutron"})
			deplName := types.NamespacedName{
				Namespace: namespace,
				Name:      "neutron",
			}
			th.SimulateDeploymentReadyWithPods(
				deplName,
				map[string][]string{namespace + "/internalapi": {}},
			)

			th.ExpectCondition(
				neutronAPIName,
				ConditionGetterFunc(NeutronAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("rejects update with wrong service override endpoint type", func() {
			NeutronAPI := GetNeutronAPI(neutronAPIName)
			Expect(NeutronAPI).NotTo(BeNil())
			if NeutronAPI.Spec.Override.Service == nil {
				NeutronAPI.Spec.Override.Service = map[service.Endpoint]service.RoutedOverrideSpec{}
			}
			NeutronAPI.Spec.Override.Service["wrooong"] = service.RoutedOverrideSpec{}
			err := k8sClient.Update(ctx, NeutronAPI)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(
				ContainSubstring(
					"invalid: spec.override.service[wrooong]: " +
						"Invalid value: \"wrooong\": invalid endpoint type: wrooong"),
			)
		})

		It("rejects update with wrong defaultConfigOverwrite", func() {
			NeutronAPI := GetNeutronAPI(neutronAPIName)
			Expect(NeutronAPI).NotTo(BeNil())
			if NeutronAPI.Spec.DefaultConfigOverwrite == nil {
				NeutronAPI.Spec.DefaultConfigOverwrite = map[string]string{}
			}
			NeutronAPI.Spec.DefaultConfigOverwrite["foo.json"] = "unsupported"
			err := k8sClient.Update(ctx, NeutronAPI)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(
				ContainSubstring(
					"invalid: spec.defaultConfigOverwrite: " +
						"Invalid value: \"foo.json\": " +
						"Only the following keys are valid: policy.yaml"),
			)
		})
	})

})
