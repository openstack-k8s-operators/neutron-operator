# neutron-operator
POC neutron-operator

NOTE: 
- The current functionality is on install at the moment, no update/upgrades or other features.
- At the moment only covers netron-ovs-agent service

## Pre Req:
- OSP16 with OVS instead of OVN deployed
- worker nodes have connection to internalapi and tenant network VLAN


#### Clone it

    $ git clone https://github.com/stuggi/neutron-operator.git
    $ cd neutron-operator

#### Updates required to pkg/controller/ovsagent/ovsagent_controller.go atm:
  - update opsHostAliases to reflect the hosts entries of your OSP env
  - update `ip route get 172.17.1.29` to reflect the overcloud.internalapi.localdomain IP address

#### Create the operator

Build the image
    
    $ oc create -f deploy/crds/neutron_v1alpha1_ovsagent_crd.yaml
    $ operator-sdk build <image e.g quay.io/mschuppe/neutron-operator:v0.0.1>

Replace `image:` in deploy/operator.yaml with your image

    $ sed -i 's|REPLACE_IMAGE|quay.io/mschuppe/neutron-operator:v0.0.1|g' deploy/operator.yaml
    $ podman push --authfile ~/mschuppe-auth.json quay.io/mschuppe/neutron-operator:v0.0.1

Create role, binding service_account

    $ oc create -f deploy/role.yaml
    $ oc create -f deploy/role_binding.yaml
    $ oc create -f deploy/service_account.yaml

Create operator

    $ oc create -f deploy/operator.yaml

    POD=`oc get pods -l name=nova-operator --field-selector=status.phase=Running -o name | head -1 -`; echo $POD
    oc logs $POD -f

Create custom resource for a compute node which specifies the container images and the label
get latest container images from rdo rhel8-train from https://trunk.rdoproject.org/rhel8-train/current-tripleo/commit.yaml
or

    $ dnf install python2 python2-yaml
    $ python -c 'import urllib2;import yaml;c=yaml.load(urllib2.urlopen("https://trunk.rdoproject.org/rhel8-train/current-tripleo/commit.yaml"))["commits"][0];print "%s_%s" % (c["commit_hash"],c["distro_hash"][0:8])'
    f8b48998e5d600f24513848b600e84176ce90223_243bc231

Update `deploy/crds/neutron_v1alpha1_ovsagent_cr.yaml`

    apiVersion: neutron.openstack-k8s-operators/v1alpha1
    kind: OvsAgent
    metadata:
      name: ovsagent
    spec:
      openvswitchImage: trunk.registry.rdoproject.org/tripleotrain/rhel-binary-neutron-openvswitch-agent:f8b48998e5d600f24513848b600e84176ce90223_243bc231
      label: compute

Required configMap got already create in nova-operator readme (TODO -split part into this one)

Apply `deploy/crds/neutron_v1alpha1_ovsagent_cr.yaml`

    oc apply -f deploy/crds/neutron_v1alpha1_ovsagent_cr.yaml

    oc get pods
    NAME                              READY   STATUS     RESTARTS   AGE
    neutron-operator-5df665ff-97t5w   1/1     Running    0          5m37s
    nova-compute-daemonset-gjjnm      3/3     Running    0          31m
    nova-compute-daemonset-grb7j      3/3     Running    0          29m
    nova-operator-5d56d8459b-mbqrn    1/1     Running    0          39m
    ovsagent-daemonset-k2dgz          1/1     Running   0          28s
    ovsagent-daemonset-nvdfr          1/1     Running   0          28s

    oc get ds
    NAME                     DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR    AGE
    nova-compute-daemonset   2         2         2       2            2           daemon=compute   37m
    ovsagent-daemonset       2         2         2       2            2           daemon=compute   24s

    oc describe OvsAgent
    Name:         ovsagent
    Namespace:    default
    Labels:       <none>
    Annotations:  kubectl.kubernetes.io/last-applied-configuration:
                    {"apiVersion":"neutron.openstack-k8s-operators/v1alpha1","kind":"OvsAgent","metadata":{"annotations":{},"name":"ovsagent","namespace":"def...
    API Version:  neutron.openstack-k8s-operators/v1alpha1
    Kind:         OvsAgent
    Metadata:
      Creation Timestamp:  2020-01-24T14:44:13Z
      Generation:          1
      Resource Version:    3830313
      Self Link:           /apis/neutron.openstack-k8s-operators/v1alpha1/namespaces/default/ovsagents/ovsagent
      UID:                 fd4bfa07-3eb7-11ea-a590-5254002c0120
    Spec:
      Label:              compute
      Openvswitch Image:  trunk.registry.rdoproject.org/tripleotrain/rhel-binary-neutron-openvswitch-agent:f8b48998e5d600f24513848b600e84176ce90223_243bc231
    Events:               <none>

    (overcloud) $ openstack network agent list -c ID -c 'Agent Type' -c Host -c State
    +--------------------------------------+--------------------+---------------------------+-------+
    | ID                                   | Agent Type         | Host                      | State |
    +--------------------------------------+--------------------+---------------------------+-------+
    | 64c0bb15-1d76-4490-a44f-897dc0f12842 | Open vSwitch agent | worker-0                  | UP    |
    | 69e646d2-9e80-439b-89fc-7a7aa92eb3ad | Open vSwitch agent | worker-1                  | UP    |
    | 9739dba6-f14a-45cc-9f25-cc27f757588d | Open vSwitch agent | controller-0.redhat.local | UP    |
    | 984a65b1-ba5c-4999-88eb-8d155644b419 | L3 agent           | controller-0.redhat.local | UP    |
    | b0c55ea9-e06e-43d5-b43c-b1f5e81faa92 | Metadata agent     | controller-0.redhat.local | UP    |
    | b8d6478b-bc07-4292-9932-09277a30f8b8 | Open vSwitch agent | compute-1.redhat.local    | UP    |
    | c7ff7e42-020e-4699-9fe4-55e749c9cb4f | Open vSwitch agent | compute-0.redhat.local    | UP    |
    | d6a7ce33-e1b0-412a-99ef-0f2a0fc247fb | DHCP agent         | controller-0.redhat.local | UP    |
    +--------------------------------------+--------------------+---------------------------+-------+


## Cleanup

    oc delete -f deploy/crds/neutron_v1alpha1_ovsagent_cr.yaml
    oc delete -f deploy/operator.yaml
    oc delete -f deploy/role.yaml
    oc delete -f deploy/role_binding.yaml
    oc delete -f deploy/service_account.yaml
    oc delete -f deploy/crds/neutron_v1alpha1_ovsagent_crd.yaml
