# neutron-operator

NOTE: 
- The current functionality is on install at the moment, no update/upgrades.
- At the moment only covers neutron-ovs-agent and neutron-sriov-agent service

## Pre Req:
- OSP16 with OVS instead of OVN deployed
- worker nodes have connection to internalapi and tenant network VLAN

#### Clone it

    mkdir openstack-k8s-operators
    cd openstack-k8s-operators
    git clone https://github.com/openstack-k8s-operators/neutron-operator.git
    cd neutron-operator

#### Create the operator

This is optional, a prebuild operator from quay.io/openstack-k8s-operators/neutron-operator could be used, e.g. quay.io/openstack-k8s-operators/neutron-operator:v0.0.1 .

Create CRDs

    oc create -f deploy/crds/neutron_v1_neutronovsagent_crd.yaml
    oc create -f deploy/crds/neutron_v1_neutronsriovagent_crd.yaml
    oc create -f deploy/crds/neutron.openstack.org_ovsnodeosps_crd.yaml
    oc create -f deploy/crds/neutron.openstack.org_ovncontrollers_crd.yaml 

Build the image, using your custom registry you have write access to

    make # creates a custom csv-generator tool
    operator-sdk build --image-builder buildah <image e.g quay.io/openstack-k8s-operators/neutron-operator:v0.0.X>

Replace `image:` in deploy/operator.yaml with your custom registry

    sed -i 's|REPLACE_IMAGE|quay.io/openstack-k8s-operators/neutron-operator:v0.0.X|g' deploy/operator.yaml
    podman push --authfile ~/mschuppe-auth.json quay.io/openstack-k8s-operators/neutron-operator:v0.0.X


#### Install the operator

Create CRDs. For ovs:

    oc create -f deploy/crds/neutron_v1_neutronovsagent_crd.yaml
    oc create -f deploy/crds/neutron_v1_neutronsriovagent_crd.yaml
    oc create -f deploy/crds/neutron.openstack.org_ovsnodeosps_crd.yaml
    oc create -f deploy/crds/neutron.openstack.org_ovncontrollers_crd.yaml 

Create namespace

    oc create -f deploy/namespace.yaml

Create role, binding service_account

    oc create -f deploy/role.yaml
    oc create -f deploy/role_binding.yaml
    oc create -f deploy/service_account.yaml

Install the operator

    oc create -f deploy/operator.yaml

    POD=`oc get pods -l name=neutron-operator --field-selector=status.phase=Running -o name | head -1 -`; echo $POD
    oc logs $POD -f

Create custom resource for a compute node which specifies the container images and the label.

Note: use OpenStack train rhel-8 container images!

Update `deploy/crds/neutron_v1_neutronovsagent_cr.yaml` with the details of the `openvswitchImage` image and OpenStack environment details.

    apiVersion: neutron.openstack.org/v1
    kind: NeutronOvsAgent
    metadata:
      name: neutron-ovsagent
    spec:
      # Rabbit transport url
      rabbitTransportURL: rabbit://guest:eJNAlgHTTN8A6mclF6q6dBdL1@controller-0.internalapi.redhat.local:5672/?ssl=0
      # Debug
      debug: "True"
      openvswitchImage: docker.io/tripleotrain/rhel-binary-neutron-openvswitch-agent:current-tripleo
      label: compute


Update `deploy/crds/neutron_v1_neutronsriovagent_cr.yaml` with the details of the `openvswitchImage` image and OpenStack environment details.

    apiVersion: neutron.openstack.org/v1
    kind: NeutronSriovAgent
    metadata:
      name: neutron-sriov-agent
    spec:
      # Rabbit transport url
      rabbitTransportURL: rabbit://guest:eJNAlgHTTN8A6mclF6q6dBdL1@controller-0.internalapi.redhat.local:5672/?ssl=0
      # Debug
      debug: "True"
      neutronSriovImage: docker.io/tripleotrain/rhel-binary-neutron-sriov-agent:current-tripleo
      label: compute


### Create required configMaps
TODO: move passwords, connection urls, ... to Secret

Node: If already done for the nova-operator, this can be skipped!

Get the following configs from a compute node in the OSP env:
- /etc/hosts

Place it in a config dir like:
- common-conf

Add OSP environment controller-0 short hostname in common-conf/osp_controller_hostname

    echo "SHORT OSP CTRL-0 HOSTNAME"> /root/common-conf/osp_controller_hostname

Create the configMaps

    oc create configmap common-config --from-file=/root/common-conf/

Note: if a later update is needed do e.g.

    oc create configmap common-config --from-file=./common-conf/ --dry-run -o yaml | oc apply -f -

!! Make sure we have the OSP needed network configs on the worker nodes. The workers need to be able to reach the internalapi and tenant network !!

Apply `deploy/crds/neutron_v1_neutronovsagent_cr.yaml`

    oc apply -f deploy/crds/neutron_v1_neutronovsagent_cr.yaml

    oc get pods
    NAME                               READY   STATUS    RESTARTS   AGE
    neutron-operator-5df665ff-79swh    1/1     Running   0          3m35s
    neutron-ovsagent-2t8hs             1/1     Running   0          97s
    neutron-ovsagent-s72r8             1/1     Running   0          97s
    ...

    oc get ds
    NAME               DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR    AGE
    neutron-ovsagent   2         2         2       2            2           daemon=compute   116s
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

Optional apply the `deploy/crds/neutron_v1_neutronsriovagent_cr.yaml`

    oc apply -f deploy/crds/neutron_v1_neutronsriovagent_cr.yaml

Note: right now it just pulls the image, uses the same neutron.conf as the ovs agent and starts a sleep.


## Cleanup

    oc delete -f deploy/crds/neutron_v1_neutronovsagent_cr.yaml
    oc delete -f deploy/crds/neutron_v1_neutronsriovagent_cr.yaml
    oc delete -f deploy/crds/neutron.openstack.org_v1_ovsnodeosp_cr.yaml
    oc delete -f deploy/crds/neutron.openstack.org_v1_ovncontroller_cr.yaml 
    oc delete -f deploy/operator.yaml
    oc delete -f deploy/role.yaml
    oc delete -f deploy/role_binding.yaml
    oc delete -f deploy/service_account.yaml
    oc delete -f deploy/crds/neutron_v1_neutronovsagent_crd.yaml
    oc delete -f deploy/crds/neutron_v1_neutronsriovagent_crd.yaml
    oc delete -f deploy/crds/neutron.openstack.org_ovsnodeosps_crd.yaml
    oc delete -f deploy/crds/neutron.openstack.org_ovncontrollers_crd.yaml 

## Formatting

For code formatting we are using goimports. It based on go fmt but also adding missing imports and removing unreferenced ones.

    go get golang.org/x/tools/cmd/goimports
    export PATH=$PATH:$GOPATH/bin
    goimports -w -v ./
