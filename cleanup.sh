#!/bin/bash
oc delete -f deploy/crds/neutron_v1_neutronovsagent_cr.yaml
oc delete -f deploy/crds/neutron_v1_neutronsriovagent_cr.yaml
oc delete -f deploy/crds/neutron.openstack.org_v1_ovsnodeosp_cr.yaml
oc delete -f deploy/crds/neutron.openstack.org_v1_ovncontroller_cr.yaml
oc delete -f deploy/crds/neutron.openstack.org_v1_ovnmetadataagent_cr.yaml
oc delete -f deploy/operator.yaml
oc delete -f deploy/role.yaml
oc delete -f deploy/cluster_role.yaml
oc delete -f deploy/role_binding.yaml
oc delete -f deploy/cluster_role_binding.yaml
oc delete -f deploy/scc.yaml
oc delete -f deploy/service_account.yaml
oc delete -f deploy/crds/neutron_v1_neutronovsagent_crd.yaml
oc delete -f deploy/crds/neutron_v1_neutronsriovagent_crd.yaml
oc delete -f deploy/crds/neutron.openstack.org_ovsnodeosps_crd.yaml
oc delete -f deploy/crds/neutron.openstack.org_ovncontrollers_crd.yaml
oc delete -f deploy/crds/neutron.openstack.org_ovnmetadataagents_crd.yaml