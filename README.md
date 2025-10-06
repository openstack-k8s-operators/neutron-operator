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
  containerImage: quay.io/tripleowallabycentos9/openstack-neutron-server:current-podified
  databaseInstance: openstack
  secret: neutron-secret
```

## Example: configure Neutron with additional networks

The Neutron spec can be used to configure Neutron to have the pods
being attached to additional networks.

Create a network-attachement-definition which then can be referenced
from the Neutron API CR.

```
---
apiVersion: k8s.cni.cncf.io/v1
kind: NetworkAttachmentDefinition
metadata:
  name: storage
  namespace: openstack
spec:
  config: |
    {
      "cniVersion": "0.3.1",
      "name": "storage",
      "type": "macvlan",
      "master": "enp7s0.21",
      "ipam": {
        "type": "whereabouts",
        "range": "172.18.0.0/24",
        "range_start": "172.18.0.50",
        "range_end": "172.18.0.100"
      }
    }
```

The following represents an example of Neutron resource that can be used
to trigger the service deployment, and have the service pods attached to
the storage network using the above NetworkAttachmentDefinition.

```
apiVersion: neutron.openstack.org/v1beta1
kind: NeutronAPI
metadata:
  name: neutron
spec:
  ...
  networkAttachents:
  - storage
...
```

When the service is up and running, it will now have an additional nic
configured for the storage network:

```
# oc rsh neutron-75f5cd6595-kpfr2
sh-5.1# ip a
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN group default qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
    inet6 ::1/128 scope host
       valid_lft forever preferred_lft forever
3: eth0@if298: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1450 qdisc noqueue state UP group default
    link/ether 0a:58:0a:82:01:18 brd ff:ff:ff:ff:ff:ff link-netnsid 0
    inet 10.130.1.24/23 brd 10.130.1.255 scope global eth0
       valid_lft forever preferred_lft forever
    inet6 fe80::4cf2:a3ff:feb0:932/64 scope link
       valid_lft forever preferred_lft forever
4: net1@if26: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP group default
    link/ether a2:f1:3b:12:fd:be brd ff:ff:ff:ff:ff:ff link-netnsid 0
    inet 172.18.0.52/24 brd 172.18.0.255 scope global net1
       valid_lft forever preferred_lft forever
    inet6 fe80::a0f1:3bff:fe12:fdbe/64 scope link
       valid_lft forever preferred_lft forever
```

## Example: expose Neutron to an isolated network

The Neutron spec can be used to configure Neutron to register e.g.
the internal endpoint to an isolated network. MetalLB is used for this
scenario.

As a pre requisite, MetalLB needs to be installed and worker nodes
prepared to work as MetalLB nodes to serve the LoadBalancer service.

In this example the following MetalLB IPAddressPool is used:

```
---
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: osp-internalapi
  namespace: metallb-system
spec:
  addresses:
  - 172.17.0.200-172.17.0.210
  autoAssign: false
```

The following represents an example of Neutron resource that can be used
to trigger the service deployment, and have the internal neutronAPI endpoint
registerd as a MetalLB service using the IPAddressPool `osp-internal`,
request to use the IP `172.17.0.202` as the VIP and the IP is shared with
other services.

```
apiVersion: neutron.openstack.org/v1beta1
kind: NeutronAPI
metadata:
  name: neutron
spec:
  ...
  externalEndpoints:
  - endpoint: internal
    ipAddressPool: osp-internalapi
    loadBalancerIPs:
    - 172.17.0.202
    sharedIP: true
    sharedIPKey: ""
  ...
...
```

The internal neutron endpoint gets registered with its service name. This
service name needs to resolve to the `LoadBalancerIP` on the isolated network
either by DNS or via /etc/hosts:

```
# openstack endpoint list -c 'Service Name' -c Interface -c URL --service network
+--------------+-----------+----------------------------------------------------------------+
| Service Name | Interface | URL                                                            |
+--------------+-----------+----------------------------------------------------------------+
| neutron      | public    | http://neutron-public-openstack.apps.ostest.test.metalkube.org |
| neutron      | internal  | http://neutron-internal.openstack.svc:9696                     |
+--------------+-----------+----------------------------------------------------------------+
```

## Provide additional volumes to Neutron

The NeutronAPI spec can be used to configure Neutron to have multiple volumes
attached to the deployed Pods. The operator will be able to define a set of
`extraVolumes` using the standard k8s spec (they can be `Secrets`, `ConfigMaps`
or even regular `PVCs`) and propagate the volumes to the Pods deployed by the
neutron-operator.

## Example: Neutron Spec with a secret

Create a `Secret` which contains the following data:

```
---
apiVersion: v1
kind: Secret
metadata:
  name: partner1
  namespace: openstack
stringData:
  partner1-credentials: |
    supersecret-credential = ******
    supersecret-credential = ******
    supersecret-credential = ******
  partner1-custom-config: |
    <data>
```

Create a `NeutronAPI` CR that defines `extraMounts` in the spec and reference
the secret which has been created:

```
apiVersion: neutron.openstack.org/v1beta1
kind: NeutronAPI
metadata:
  name: neutron
  namespace: openstack
spec:
  serviceUser: neutron
  customServiceConfig: |
    [DEFAULT]
    debug = true
  databaseInstance: openstack
  databaseUser: neutron
  rabbitMqClusterName: rabbitmq
  memcachedInstance: memcached
  preserveJobs: false
  containerImage: quay.io/podified-antelope-centos9/openstack-neutron-server:current-podified
  replicas: 1
  secret: neutron-secret
  extraMounts:
    - extraVolType: Partner1
      volumes:
      - name: partner1
        secret:
          secretName: partner1-config
      mounts:
      - name: partner1
        mountPath: "/var/lib/neutron/third_party/partner1"
        readOnly: true
```

The data defined in `/var/lib/neutron/third_party/partner1` will be mounted
to the resulting neutronAPI pod.

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

## Run kuttl test
The easiest way is to run kuttl test in fresh system with crc deployed with

- **create enough pv's**
`PV_NUM=30 make crc_storage`
- make your changes in neutron-operator
- add required kuttl tests
- start webhook `make run-with-webhook`

- navigate to install_yamls
- run kuttl tests
`make neutron_kuttl_run OPERATOR_BASE_DIR=/home/stack/repos NAMESPACE=neutron-kuttl-tests`

- To run a single test
`make neutron_kuttl_run OPERATOR_BASE_DIR=/home/stack/repos NAMESPACE=neutron-kuttl-tests KUTTL_ARGS="--test notifications"`
