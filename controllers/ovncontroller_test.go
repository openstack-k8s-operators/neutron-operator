package controllers

import (
	"context"
	appsv1 "k8s.io/api/apps/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"

	neutronv1beta1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
	"k8s.io/apimachinery/pkg/types"
)

const timeout = time.Second * 60
const interval = time.Second * 1

var (
	//name      string
	//namespace string
	//request   reconcile.Request
	ctx     = context.Background()
	fetched = &neutronv1beta1.OVNController{}
)

var _ = Describe("OVNcontroller", func() {

	BeforeEach(func() {
	})
	Describe("OVN Controller", func() {
		var (
			instance *neutronv1beta1.OVNController
		)
		Context("Initially  ", func() {
			It("Should create CR Successfully ", func() {
				instance = createInstanceInCluster()
				Expect(k8sClient.Create(ctx, instance)).Should(Succeed())
			})
			It("Should created CR exist", func() {
				By("Expecting kind created")
				Eventually(func() error {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: "default"}, fetched)
					return err
				}, timeout, interval).ShouldNot(HaveOccurred())
			})

			Context("When CR is Exists    ", func() {
				It("Should create DaemonSet(s) ", func() {
					ovnDaemonset := &appsv1.DaemonSet{}
					Eventually(func() error {
						err := k8sClient.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: "default"}, ovnDaemonset)
						return err
					}, timeout, interval).ShouldNot(HaveOccurred())
					Expect(ovnDaemonset.Name).To(Equal(instance.Name))
				})
			})
		})
	})
})

func createInstanceInCluster() *neutronv1beta1.OVNController {
	spec := neutronv1beta1.OVNControllerSpec{
		OvnControllerImage: "quay.io/ltomasbo/ovn-controller:multibridge",
		ServiceAccount:     "neutron",
		RoleName:           "worker-osp",
		OvnLogLevel:        "info",
	}

	toCreate := neutronv1beta1.OVNController{
		TypeMeta: v1.TypeMeta{},
		ObjectMeta: v1.ObjectMeta{
			Name:      "ovncontroller",
			Namespace: "default",
		},
		Spec: spec,
	}
	return &toCreate
}
