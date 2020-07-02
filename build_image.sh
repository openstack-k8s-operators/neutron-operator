#!/bin/bash
oc create -f deploy/crds/neutron_v1_neutronovsagent_crd.yaml
oc create -f deploy/crds/neutron_v1_neutronsriovagent_crd.yaml
oc create -f deploy/crds/neutron.openstack.org_ovsnodeosps_crd.yaml
oc create -f deploy/crds/neutron.openstack.org_ovncontrollers_crd.yaml
oc create -f deploy/crds/neutron.openstack.org_ovnmetadataagents_crd.yaml
make
operator-sdk build --image-builder buildah "${REGISTRY:-quay.io/openstack-k8s-operators/neutron-operator:v0.0.3}"
sed -i 's|REPLACE_IMAGE|"${REGISTRY:-quay.io/openstack-k8s-operators/neutron-operator:v0.0.3}"g' deploy/operator.yaml
podman push --authfile "${REGISTRY_AUTH}" "${REGISTRY:-quay.io/openstack-k8s-operators/neutron-operator:v0.0.3}"
