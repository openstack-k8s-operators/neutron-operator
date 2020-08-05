#!/bin/bash
if [ -z ${BUILD_IMAGE+x} ]; then echo "BUILD_IMAGE is unset"; exit 1; fi
if [ -z ${TAG+x} ]; then echo "TAG is unset, Please set value for TAG of image"; exit 1; fi
if [ -z ${REGISTRY_AUTH+x} ]; then echo "REGISTRY_AUTH is unset, set path to yours auth file"; exit 1; fi

oc create -f deploy/crds/neutron_v1_neutronsriovagent_crd.yaml
oc create -f deploy/crds/neutron.openstack.org_ovsnodeosps_crd.yaml
oc create -f deploy/crds/neutron.openstack.org_ovncontrollers_crd.yaml
make
operator-sdk build --image-builder buildah "${BUILD_IMAGE}:${TAG}"
sed -i 's|REPLACE_IMAGE|"${BUILD_IMAGE}:${TAG}' deploy/operator.yaml
podman push --authfile "${REGISTRY_AUTH}" "${BUILD_IMAGE}:${TAG}"
