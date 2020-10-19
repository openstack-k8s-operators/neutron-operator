module github.com/openstack-k8s-operators/neutron-operator

go 1.13

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/go-logr/logr v0.1.0
	github.com/onsi/ginkgo v1.12.1
	github.com/onsi/gomega v1.10.1
	github.com/openshift/api v0.0.0-20200205133042-34f0ec8dab87
	github.com/openstack-k8s-operators/cinder-operator v0.0.0-20201014122125-8dfff57510fc
	github.com/openstack-k8s-operators/keystone-operator v0.0.0-20201012214326-7b0b20e9777b
	github.com/openstack-k8s-operators/lib-common v0.0.0-20201012132655-247b83b2fafa
	github.com/operator-framework/operator-lifecycle-manager v0.0.0-20200321030439-57b580e57e88
	github.com/prometheus/common v0.7.0
	golang.org/x/lint v0.0.0-20200302205851-738671d3881b // indirect
	golang.org/x/tools v0.0.0-20200831203904-5a2aa26beb65 // indirect
	k8s.io/api v0.18.6
	k8s.io/apimachinery v0.18.6
	k8s.io/client-go v0.18.6
	sigs.k8s.io/controller-runtime v0.6.2
)
