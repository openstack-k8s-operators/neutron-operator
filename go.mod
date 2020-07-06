module github.com/openstack-k8s-operators/neutron-operator

go 1.13

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/go-openapi/spec v0.19.7
	github.com/openshift/api v0.0.0-20200205133042-34f0ec8dab87
	github.com/openshift/cluster-api v0.0.0-20191129101638-b09907ac6668
	github.com/openstack-k8s-operators/lib-common v0.0.0-20200506095056-36244492b7a8
	github.com/operator-framework/operator-lifecycle-manager v0.0.0-20200321030439-57b580e57e88
	github.com/operator-framework/operator-sdk v0.17.0
	github.com/spf13/pflag v1.0.5
	golang.org/x/tools v0.0.0-20200504022951-6b6965ac5dd1 // indirect
	k8s.io/api v0.17.4
	k8s.io/apimachinery v0.17.4
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/kube-openapi v0.0.0-20191107075043-30be4d16710a
	sigs.k8s.io/controller-runtime v0.5.2
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	k8s.io/client-go => k8s.io/client-go v0.17.4 // Required by prometheus-operator
)
