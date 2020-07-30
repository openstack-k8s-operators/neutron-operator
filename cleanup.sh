#!/bin/bash
oc delete -f deploy/crds/neutron_v1_neutronsriovagent_cr.yaml
oc delete -f deploy/crds/neutron.openstack.org_v1_ovsnodeosp_cr.yaml
oc delete -f deploy/crds/neutron.openstack.org_v1_ovncontroller_cr.yaml
oc delete -f deploy/operator.yaml
oc create -f deploy/role.yaml
oc create -f deploy/role_binding.yaml
oc create -f deploy/service_account.yaml
oc delete -f deploy/crds/neutron_v1_neutronsriovagent_crd.yaml
oc delete -f deploy/crds/neutron.openstack.org_ovsnodeosps_crd.yaml
oc delete -f deploy/crds/neutron.openstack.org_ovncontrollers_crd.yaml