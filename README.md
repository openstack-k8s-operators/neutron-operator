# neutron-operator

NOTE: 
- The current functionality is on install at the moment, no update/upgrades or other features.
- At the moment only covers netron-ovs-agent service

## Pre Req:
- OSP16 with OVS instead of OVN deployed
- worker nodes have connection to internalapi and tenant network VLAN

#### Clone it

    $ git clone https://github.com/stuggi/neutron-operator.git
    $ cd neutron-operator

#### Create the operator

Build the image
    
    $ oc create -f deploy/crds/neutron_v1_neutronovsagent_crd.yaml
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

    POD=`oc get pods -l name=neutron-operator --field-selector=status.phase=Running -o name | head -1 -`; echo $POD
    oc logs $POD -f

Create custom resource for a compute node which specifies the container images and the label
get latest container images from rdo rhel8-train from https://trunk.rdoproject.org/rhel8-train/current-tripleo/commit.yaml
or

    $ dnf install python2 python2-yaml
    $ python -c 'import urllib2;import yaml;c=yaml.load(urllib2.urlopen("https://trunk.rdoproject.org/rhel8-train/current-tripleo/commit.yaml"))["commits"][0];print "%s_%s" % (c["commit_hash"],c["distro_hash"][0:8])'
    f8b48998e5d600f24513848b600e84176ce90223_243bc231

Update `deploy/crds/neutron_v1_neutronovsagent_cr.yaml`

    apiVersion: neutron.openstack.org/v1
    kind: NeutronOvsAgent
    metadata:
      name: neutron-ovsagent
    spec:
      openvswitchImage: trunk.registry.rdoproject.org/tripleotrain/rhel-binary-neutron-openvswitch-agent:f8b48998e5d600f24513848b600e84176ce90223_243bc231
      label: compute

### Create required configMaps
TODO: move passwords, connection urls, ... to Secret

Get the following configs from a compute node in the OSP env:
- /etc/hosts
- /var/lib/config-data/puppet-generated/neutron/etc/neutron/neutron.conf
- /var/lib/config-data/puppet-generated/neutron/etc/neutron/plugins/ml2/openvswitch_agent.ini

Place each group in a config dir like:
- common-conf
- neutron-conf

Add OSP environment controller-0 short hostname in common-conf/osp_controller_hostname

    echo "SHORT OSP CTRL-0 HOSTNAME"> /root/common-conf/osp_controller_hostname

Create the configMaps

    oc create configmap common-config --from-file=/root/common-conf/
    oc create configmap neutron-config --from-file=./neutron-conf/

Note: if a later update is needed do e.g.

    oc create configmap neutron-config --from-file=./neutron-conf/ --dry-run -o yaml | oc apply -f -

!! Make sure we have the OSP needed network configs on the worker nodes. The workers need to be able to reach the internalapi and tenant network !!

Apply `deploy/crds/neutron_v1_neutronovsagent_cr.yaml`

    oc apply -f deploy/crds/neutron_v1_neutronovsagent_cr.yaml

    oc get pods
    NAME                               READY   STATUS    RESTARTS   AGE
    neutron-operator-5df665ff-79swh    1/1     Running   0          3m35s
    neutron-ovsagent-daemonset-2t8hs   1/1     Running   0          97s
    neutron-ovsagent-daemonset-s72r8   1/1     Running   0          97s
    ...

    oc get ds
    NAME                         DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR    AGE
    neutron-ovsagent-daemonset   2         2         2       2            2           daemon=compute   116s
    ...


Verify that the ovs-agent successfully registered in neutron:

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

    oc delete -f deploy/crds/neutron_v1_neutronovsagent_cr.yaml
    oc delete -f deploy/operator.yaml
    oc delete -f deploy/role.yaml
    oc delete -f deploy/role_binding.yaml
    oc delete -f deploy/service_account.yaml
    oc delete -f deploy/crds/neutron_v1_neutronovsagent_crd.yaml
