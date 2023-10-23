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
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	neutronv1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
)

const (
	SecretName = "test-secret"

	timeout  = time.Second * 10
	interval = timeout / 100
)

func SetExternalDBEndpoint(name types.NamespacedName, endpoint string) {
	Eventually(func(g Gomega) {
		cluster := GetOVNDBCluster(name)
		cluster.Status.DBAddress = endpoint
		g.Expect(k8sClient.Status().Update(ctx, cluster)).To(Succeed())
	}, timeout, interval).Should(Succeed())
}

func SimulateTransportURLReady(name types.NamespacedName) {
	Eventually(func(g Gomega) {
		transport := infra.GetTransportURL(name)
		// To avoid another secret creation and cleanup
		transport.Status.SecretName = SecretName
		transport.Status.Conditions.MarkTrue("TransportURLReady", "Ready")
		g.Expect(k8sClient.Status().Update(ctx, transport)).To(Succeed())

	}, timeout, interval).Should(Succeed())
	logger.Info("Simulated TransportURL ready", "on", name)
}

func GetDefaultNeutronAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"databaseInstance": "test-neutron-db-instance",
		"containerImage":   "test-neutron-container-image",
		"secret":           SecretName,
	}
}

func CreateNeutronAPI(namespace string, NeutronAPIName string, spec map[string]interface{}) types.NamespacedName {

	raw := map[string]interface{}{
		"apiVersion": "neutron.openstack.org/v1beta1",
		"kind":       "NeutronAPI",
		"metadata": map[string]interface{}{
			"name":      NeutronAPIName,
			"namespace": namespace,
		},
		"spec": spec,
	}
	th.CreateUnstructured(raw)

	return types.NamespacedName{Name: NeutronAPIName, Namespace: namespace}
}

func DeleteNeutronAPI(name types.NamespacedName) {
	// We have to wait for the controller to fully delete the instance
	Eventually(func(g Gomega) {
		NeutronAPI := &neutronv1.NeutronAPI{}
		err := k8sClient.Get(ctx, name, NeutronAPI)
		// if it is already gone that is OK
		if k8s_errors.IsNotFound(err) {
			return
		}
		g.Expect(err).Should(BeNil())

		g.Expect(k8sClient.Delete(ctx, NeutronAPI)).Should(Succeed())

		err = k8sClient.Get(ctx, name, NeutronAPI)
		g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
	}, timeout, interval).Should(Succeed())
}

func GetNeutronAPI(name types.NamespacedName) *neutronv1.NeutronAPI {
	instance := &neutronv1.NeutronAPI{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}

func NeutronAPIConditionGetter(name types.NamespacedName) condition.Conditions {
	instance := GetNeutronAPI(name)
	return instance.Status.Conditions
}

// CreateOVNDBClusters Creates NB and SB OVNDBClusters
func CreateOVNDBClusters(namespace string) []types.NamespacedName {
	dbs := []types.NamespacedName{}
	for _, db := range []string{"NB", "SB"} {
		ovndbcluster := &ovnv1.OVNDBCluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "ovn.openstack.org/v1beta1",
				Kind:       "OVNDBCluster",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ovn-" + uuid.New().String(),
				Namespace: namespace,
			},
			Spec: ovnv1.OVNDBClusterSpec{
				DBType: db,
			},
		}

		Expect(k8sClient.Create(ctx, ovndbcluster.DeepCopy())).Should(Succeed())
		name := types.NamespacedName{Namespace: namespace, Name: ovndbcluster.Name}

		dbaddr := "tcp:10.1.1.1:6641"
		if db == "SB" {
			dbaddr = "tcp:10.1.1.1:6642"
		}

		// the Status field needs to be written via a separate client
		ovndbcluster = GetOVNDBCluster(name)
		ovndbcluster.Status = ovnv1.OVNDBClusterStatus{
			InternalDBAddress: dbaddr,
		}
		Expect(k8sClient.Status().Update(ctx, ovndbcluster.DeepCopy())).Should(Succeed())
		dbs = append(dbs, name)

	}

	logger.Info("OVNDBClusters created", "OVNDBCluster", dbs)
	return dbs
}

// DeleteOVNDBClusters Delete OVN DBClusters
func DeleteOVNDBClusters(names []types.NamespacedName) {
	for _, db := range names {
		Eventually(func(g Gomega) {
			ovndbcluster := &ovnv1.OVNDBCluster{}
			err := k8sClient.Get(ctx, db, ovndbcluster)
			// if it is already gone that is OK
			if k8s_errors.IsNotFound(err) {
				return
			}
			g.Expect(err).Should(BeNil())

			g.Expect(k8sClient.Delete(ctx, ovndbcluster)).Should(Succeed())

			err = k8sClient.Get(ctx, db, ovndbcluster)
			g.Expect(k8s_errors.IsNotFound(err)).To(BeTrue())
		}, timeout, interval).Should(Succeed())
	}
}

// GetOVNDBCluster Get OVNDBCluster
func GetOVNDBCluster(name types.NamespacedName) *ovnv1.OVNDBCluster {
	instance := &ovnv1.OVNDBCluster{}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, name, instance)).Should(Succeed())
	}, timeout, interval).Should(Succeed())
	return instance
}
