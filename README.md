# neutron-operator

A Kubernetes Operator built using the [Operator Framework](https://github.com/operator-framework) for Go.
The Operator provides a way to easily install and manage an OpenStack Neutron installation on Kubernetes.
This Operator was developed using [RDO](https://www.rdoproject.org/) containers for openStack.

# Deployment

The operator is intended to be deployed via OLM [Operator Lifecycle Manager](https://github.com/operator-framework/operator-lifecycle-manager)

# API Example

The Operator creates a custom NeutronAPI resource that can be used to create Neutron API
instances within the cluster. Example CR to create a Neutron API in your cluster:

```yaml
apiVersion: neutron.openstack.org/v1beta1
kind: NeutronAPI
metadata:
  name: neutron
spec:
  containerImage: quay.io/tripleowallabycentos9/openstack-neutron-server:current-tripleo
  databaseInstance: openstack
  secret: neutron-secret
```

# Design
*TBD*

# Testing
The repository uses [EnvTest](https://book.kubebuilder.io/reference/envtest.html) to validate the operator in a self
contained environment.

The test can be run in the terminal with:
```shell
make test
```
or in Visual Studio Code by defining the following in your settings.json:
```json
"go.testEnvVars": {
    "KUBEBUILDER_ASSETS":"<location of kubebuilder-envtest installation>"
},
```
