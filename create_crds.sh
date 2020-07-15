#!/bin/bash
oc create -f deploy/crds/neutron_v1_neutronovsagent_crd.yaml
oc create -f deploy/crds/neutron_v1_neutronsriovagent_crd.yaml
oc create -f deploy/crds/neutron.openstack.org_ovsnodeosps_crd.yaml
oc create -f deploy/crds/neutron.openstack.org_ovncontrollers_crd.yaml
oc create -f deploy/namespace.yaml
oc create -f deploy/role.yaml
oc create -f deploy/cluster_role.yaml
oc create -f deploy/role_binding.yaml
oc create -f deploy/cluster_role_binding.yaml
oc create -f deploy/service_account.yaml
oc create -f deploy/scc.yaml
