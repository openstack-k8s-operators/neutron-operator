package controllers

import (
	"context"
	appsv1 "k8s.io/api/apps/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	neutronv1beta1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
	"k8s.io/apimachinery/pkg/types"
)

var (
	//name      string
	//namespace string
	//request   reconcile.Request
	ctxovs     = context.Background()
	fetchedovs = &neutronv1beta1.OVSNodeOsp{}
)

var _ = Describe("OVSNodeController", func() {

	BeforeEach(func() {
	})
	Describe("OVSNode", func() {
		var (
			instance *neutronv1beta1.OVSNodeOsp
		)
		Context("Initially  ", func() {
			It("Should create CR Successfully ", func() {
				instance = createOVSInstanceInCluster()
				Expect(k8sClient.Create(ctx, instance)).Should(Succeed())
			})
			It("Should created CR exist", func() {
				By("Expecting kind created")
				Eventually(func() error {
					err := k8sClient.Get(ctxovs, types.NamespacedName{Name: instance.Name, Namespace: "default"}, fetchedovs)
					return err
				}, timeout, interval).ShouldNot(HaveOccurred())
			})

			Context("When CR is Exists    ", func() {
				It("Should create DaemonSet(s) ", func() {
					ovnDaemonSet := &appsv1.DaemonSet{}
					Eventually(func() error {
						err := k8sClient.Get(ctxovs, types.NamespacedName{Name: instance.Name, Namespace: "default"}, ovnDaemonSet)
						return err
					}, timeout, interval).ShouldNot(HaveOccurred())
					Expect(ovnDaemonSet.Name).To(Equal(instance.Name))
				})
			})
		})
	})
})

func createOVSInstanceInCluster() *neutronv1beta1.OVSNodeOsp {
	spec := neutronv1beta1.OVSNodeOspSpec{
		OvsNodeOspImage: "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:7d6ba1bcc4f403733e44cd2a008dafe1887501f7a3c74084566a4f4d1e46250d",
		ServiceAccount:  "neutron",
		RoleName:        "worker-osp",
		OvsLogLevel:     "info",
		Nic:             "enp2s0",
		Gateway:         true,
		BridgeMappings:  "datacentre:br-ex",
	}

	toCreate := neutronv1beta1.OVSNodeOsp{
		TypeMeta: v1.TypeMeta{},
		ObjectMeta: v1.ObjectMeta{
			Name:      "ovsnode",
			Namespace: "default",
		},
		Spec: spec,
	}
	return &toCreate
}
