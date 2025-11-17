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
	. "github.com/onsi/gomega" //revive:disable:dot-imports
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	neutronv1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/neutron-operator/internal/neutronapi"
	ovnv1 "github.com/openstack-k8s-operators/ovn-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	SecretName = "test-secret"

	PublicCertSecretName   = "public-tls-certs"   // #nosec G101 -- Test constant, not a credential
	InternalCertSecretName = "internal-tls-certs" // #nosec G101 -- Test constant, not a credential
	CABundleSecretName     = "combined-ca-bundle" // #nosec G101 -- Test constant, not a credential
	OVNDbCertSecretName    = "ovndb-tls-certs"    // #nosec G101 -- Test constant, not a credential

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

func GetDefaultNeutronAPISpec() map[string]any {
	return map[string]any{
		"databaseInstance": "test-neutron-db-instance",
		"secret":           SecretName,
	}
}

func CreateNeutronAPI(namespace string, NeutronAPIName string, spec map[string]any) client.Object {

	raw := map[string]any{
		"apiVersion": "neutron.openstack.org/v1beta1",
		"kind":       "NeutronAPI",
		"metadata": map[string]any{
			"name":      NeutronAPIName,
			"namespace": namespace,
		},
		"spec": spec,
	}

	return th.CreateUnstructured(raw)
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
				OVNDBClusterSpecCore: ovnv1.OVNDBClusterSpecCore{
					DBType: db,
				},
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
			g.Expect(err).ShouldNot(HaveOccurred())

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

func CreateNeutronAPISecret(namespace string, name string) *corev1.Secret {
	return th.CreateSecret(
		types.NamespacedName{Namespace: namespace, Name: name},
		map[string][]byte{
			"NeutronPassword": []byte("12345678"),
			"transport_url":   []byte("rabbit://user@svc:1234"),
		},
	)
}

// GetSampleTopologySpec - A sample (and opinionated) Topology Spec used to
// test NeutronAPI
// Note this is just an example that should not be used in production for
// multiple reasons:
// 1. It uses ScheduleAnyway as strategy, which is something we might
// want to avoid by default
// 2. Usually a topologySpreadConstraints is used to take care about
// multi AZ, which is not applicable in this context
func GetSampleTopologySpec(label string) (map[string]any, []corev1.TopologySpreadConstraint) {
	// Build the topology Spec
	topologySpec := map[string]any{
		"topologySpreadConstraints": []map[string]any{
			{
				"maxSkew":           1,
				"topologyKey":       corev1.LabelHostname,
				"whenUnsatisfiable": "ScheduleAnyway",
				"labelSelector": map[string]any{
					"matchLabels": map[string]any{
						"service":   neutronapi.ServiceName,
						"component": label,
					},
				},
			},
		},
	}
	// Build the topologyObj representation
	topologySpecObj := []corev1.TopologySpreadConstraint{
		{
			MaxSkew:           1,
			TopologyKey:       corev1.LabelHostname,
			WhenUnsatisfiable: corev1.ScheduleAnyway,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"service":   neutronapi.ServiceName,
					"component": label,
				},
			},
		},
	}
	return topologySpec, topologySpecObj
}

// GetExtraMounts - Utility function that simulates extraMounts pointing
// to a  secret
func GetExtraMounts(nemName string, nemPath string) []map[string]any {
	return []map[string]any{
		{
			"name":   nemName,
			"region": "az0",
			"extraVol": []map[string]any{
				{
					"extraVolType": nemName,
					"propagation": []string{
						"Neutron",
					},
					"volumes": []map[string]any{
						{
							"name": nemName,
							"secret": map[string]any{
								"secretName": nemName,
							},
						},
					},
					"mounts": []map[string]any{
						{
							"name":      nemName,
							"mountPath": nemPath,
							"readOnly":  true,
						},
					},
				},
			},
		},
	}
}
