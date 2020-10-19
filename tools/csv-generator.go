package main

import (
	"flag"
	"os"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/blang/semver"
	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	"github.com/openstack-k8s-operators/neutron-operator/tools/helper"
	csvv1 "github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators/v1alpha1"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/lib/version"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	csvVersion         = flag.String("csv-version", "", "")
	replacesCsvVersion = flag.String("replaces-csv-version", "", "")
	namespace          = flag.String("namespace", "", "")
	pullPolicy         = flag.String("pull-policy", "Always", "")

	logoBase64 = flag.String("logo-base64", "", "")
	verbosity  = flag.String("verbosity", "1", "")

	operatorImage = flag.String("operator-image-name", "quay.io/openstack-k8s-operators/neutron-operator:devel", "optional")
)

func main() {
	flag.Parse()

	data := NewClusterServiceVersionData{
		CsvVersion:         *csvVersion,
		ReplacesCsvVersion: *replacesCsvVersion,
		Namespace:          *namespace,
		ImagePullPolicy:    *pullPolicy,
		IconBase64:         *logoBase64,
		Verbosity:          *verbosity,
		OperatorImage:      *operatorImage,
	}

	csv, err := createClusterServiceVersion(&data)
	if err != nil {
		panic(err)
	}
	util.MarshallObject(csv, os.Stdout)

}

//NewClusterServiceVersionData - Data arguments used to create neutron operators's CSV manifest
type NewClusterServiceVersionData struct {
	CsvVersion         string
	ReplacesCsvVersion string
	Namespace          string
	ImagePullPolicy    string
	IconBase64         string
	Verbosity          string

	DockerPrefix string
	DockerTag    string

	OperatorImage string
}

func createOperatorDeployment(repo, namespace, deployClusterResources, operatorImage, tag, verbosity, pullPolicy string) *appsv1.Deployment {
	deployment := helper.CreateOperatorDeployment("neutron-operator", namespace, "name", "neutron-operator", "neutron-operator", int32(1))
	container := helper.CreateOperatorContainer("neutron-operator", operatorImage, verbosity, corev1.PullPolicy(pullPolicy))
	container.Env = *helper.CreateOperatorEnvVar(repo, deployClusterResources, operatorImage, pullPolicy)
	deployment.Spec.Template.Spec.Containers = []corev1.Container{container}
	return deployment
}

func createClusterServiceVersion(data *NewClusterServiceVersionData) (*csvv1.ClusterServiceVersion, error) {

	description := `
Install and configure OpenStack Neutron containers.
`
	deployment := createOperatorDeployment(
		data.DockerPrefix,
		data.Namespace,
		"true",
		data.OperatorImage,
		data.DockerTag,
		data.Verbosity,
		data.ImagePullPolicy)

	rules := getOperatorRules()
	serviceRules := getServiceRules()

	strategySpec := csvv1.StrategyDetailsDeployment{
		Permissions: []csvv1.StrategyDeploymentPermissions{
			{
				ServiceAccountName: "neutron-operator",
				Rules:              *rules,
			},
			{
				ServiceAccountName: "neutron",
				Rules:              *serviceRules,
			},
		},
		DeploymentSpecs: []csvv1.StrategyDeploymentSpec{
			{
				Name: "neutron-operator",
				Spec: deployment.Spec,
			},
		},
	}

	csvVersion, err := semver.New(data.CsvVersion)
	if err != nil {
		return nil, err
	}

	return &csvv1.ClusterServiceVersion{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterServiceVersion",
			APIVersion: "operators.coreos.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "neutron-operator." + data.CsvVersion,
			Namespace: data.Namespace,
			Annotations: map[string]string{

				"capabilities": "Basic Install",
				"categories":   "Compute",
				"description":  "Creates and maintains Neutron Compute daemonsets",
			},
		},

		Spec: csvv1.ClusterServiceVersionSpec{
			DisplayName: "Neutron Operator",
			Description: description,
			Keywords:    []string{"Neutron Operator", "OpenStack", "Neutron", "Compute"},
			Version:     version.OperatorVersion{Version: *csvVersion},
			Maturity:    "alpha",
			Replaces:    data.ReplacesCsvVersion,
			Maintainers: []csvv1.Maintainer{{
				Name:  "OpenStack k8s Operators",
				Email: "openstack-k8s-operators@googlegroups.com",
			}},
			Provider: csvv1.AppLink{
				Name: "OpenStack K8s Operators Neutron Operator project",
			},
			Links: []csvv1.AppLink{
				{
					Name: "Neutron Operator",
					URL:  "https://github.com/openstack-k8s-operators/neutron-operator/blob/master/README.md",
				},
				{
					Name: "Source Code",
					URL:  "https://github.com/openstack-k8s-operators/neutron-operator",
				},
			},
			Icon: []csvv1.Icon{{
				Data:      data.IconBase64,
				MediaType: "image/png",
			}},
			Labels: map[string]string{
				"alm-owner-neutron-operator": "neutron-operator",
				"operated-by":                "neutron-operator",
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"alm-owner-neutron-operator": "neutron-operator",
					"operated-by":                "neutron-operator",
				},
			},
			InstallModes: []csvv1.InstallMode{
				{
					Type:      csvv1.InstallModeTypeOwnNamespace,
					Supported: true,
				},
				{
					Type:      csvv1.InstallModeTypeSingleNamespace,
					Supported: true,
				},
				{
					Type:      csvv1.InstallModeTypeMultiNamespace,
					Supported: true,
				},
				{
					Type:      csvv1.InstallModeTypeAllNamespaces,
					Supported: true,
				},
			},
			InstallStrategy: csvv1.NamedInstallStrategy{
				StrategyName: "deployment",
				StrategySpec: strategySpec,
			},
			CustomResourceDefinitions: csvv1.CustomResourceDefinitions{

				Owned: []csvv1.CRDDescription{
					{
						Name:        "neutronsriovagents.neutron.openstack.org",
						Version:     "v1beta1",
						Kind:        "NeutronSriovAgent",
						DisplayName: "Neutron Sriov Agent",
						Description: "NeutronSriovAgent is the Schema for the neutronsriovagents API",
					},
					{
						Name:        "ovncontrollers.neutron.openstack.org",
						Version:     "v1beta1",
						Kind:        "OVNController",
						DisplayName: "OVN Controller",
						Description: "OVNController is the Schema for the ovncontrollers API",
					},
					{
						Name:        "ovsnodeosps.neutron.openstack.org",
						Version:     "v1beta1",
						Kind:        "OVSNodeOsp",
						DisplayName: "OVS Node OSP",
						Description: "OVSNodeOsp is the Schema for the ovsnodeosps API",
					},
					{
						Name:        "neutronapis.neutron.openstack.org",
						Version:     "v1beta1",
						Kind:        "NeutronAPI",
						DisplayName: "Neutron Server",
						Description: "NeutronAPI is the Schema for the neutron servers API",
					},
				},
			},
		},
	}, nil
}

func getOperatorRules() *[]rbacv1.PolicyRule {
	return &[]rbacv1.PolicyRule{
		{
			APIGroups: []string{
				"",
			},
			Resources: []string{
				"pods",
				"services",
				"services/finalizers",
				"endpoints",
				"persistentvolumeclaims",
				"events",
				"configmaps",
				"secrets",
			},
			Verbs: []string{
				"*",
			},
		},
		{
			APIGroups: []string{
				"apps",
			},
			Resources: []string{
				"deployments",
				"daemonsets",
				"replicasets",
				"statefulsets",
			},
			Verbs: []string{
				"*",
			},
		},
		{
			APIGroups: []string{
				"apps",
			},
			Resources: []string{
				"daemonsets",
			},
			ResourceNames: []string{
				"neutron-operator",
			},
			Verbs: []string{
				"delete",
				"update",
			},
		},
		{
			APIGroups: []string{
				"monitoring.coreos.com",
			},
			Resources: []string{
				"servicemonitors",
			},
			Verbs: []string{
				"get",
				"create",
			},
		},
		{
			APIGroups: []string{
				"apps",
			},
			Resources: []string{
				"deployments/finalizers",
			},
			ResourceNames: []string{
				"neutron-operator",
			},
			Verbs: []string{
				"update",
			},
		},
		{
			APIGroups: []string{
				"batch",
			},
			Resources: []string{
				"jobs",
			},
			Verbs: []string{
				"*",
			},
		},
		{
			APIGroups: []string{
				"route.openshift.io",
			},
			Resources: []string{
				"routes",
			},
			Verbs: []string{
				"list",
				"watch",
				"create",
				"patch",
				"update",
			},
		},
		{
			APIGroups: []string{
				"neutron.openstack.org",
			},
			Resources: []string{
				"*",
				"neutronsriovagents",
				"ovsnodeosps",
				"ovncontrollers",
				"neutronapis",
			},
			Verbs: []string{
				"*",
			},
		},
		{
			APIGroups: []string{
				"keystone.openstack.org",
			},
			Resources: []string{
				"keystoneservices",
			},
			Verbs: []string{
				"list",
				"create",
				"update",
				"watch",
			},
		},
		{
			APIGroups: []string{
				"database.openstack.org",
			},
			Resources: []string{
				"mariadbdatabases",
			},
			Verbs: []string{
				"get",
				"create",
			},
		},
	}
}

func getServiceRules() *[]rbacv1.PolicyRule {
	return &[]rbacv1.PolicyRule{
		{
			APIGroups: []string{
				"",
			},
			Resources: []string{
				"pods",
			},
			Verbs: []string{
				"*",
			},
		},
		{
			APIGroups: []string{
				"security.openshift.io",
			},
			Resources: []string{
				"securitycontextconstraints",
			},
			ResourceNames: []string{
				"privileged",
			},
			Verbs: []string{
				"use",
			},
		},
	}
}
