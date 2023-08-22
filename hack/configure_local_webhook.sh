#!/bin/bash
set -ex

TMPDIR=${TMPDIR:-"/tmp/k8s-webhook-server/serving-certs"}
SKIP_CERT=${SKIP_CERT:-false}
CRC_IP=${CRC_IP:-$(/sbin/ip -o -4 addr list crc | awk '{print $4}' | cut -d/ -f1)}
FIREWALL_ZONE=${FIREWALL_ZONE:-"libvirt"}

#Open 9443
if command -v firewall-cmd &> /dev/null; then
    sudo firewall-cmd --zone=${FIREWALL_ZONE} --add-port=9443/tcp
    sudo firewall-cmd --runtime-to-permanent
fi

# Generate the certs and the ca bundle
if [ "$SKIP_CERT" = false ] ; then
    mkdir -p ${TMPDIR}
    rm -rf ${TMPDIR}/* || true

    openssl req -newkey rsa:2048 -days 3650 -nodes -x509 \
        -subj "/CN=${HOSTNAME}" \
        -addext "subjectAltName = IP:${CRC_IP}" \
        -keyout ${TMPDIR}/tls.key \
        -out ${TMPDIR}/tls.crt

    cat ${TMPDIR}/tls.crt ${TMPDIR}/tls.key | base64 -w 0 > ${TMPDIR}/bundle.pem

fi

CA_BUNDLE=`cat ${TMPDIR}/bundle.pem`

# Patch the webhook(s)
cat >> ${TMPDIR}/patch_webhook_configurations.yaml <<EOF_CAT
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: vneutronapi.kb.io
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    caBundle: ${CA_BUNDLE}
    url: https://${CRC_IP}:9443/validate-neutron-openstack-org-v1beta1-neutronapi
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: vneutronapi.kb.io
  objectSelector: {}
  rules:
  - apiGroups:
    - neutron.openstack.org
    apiVersions:
    - v1beta1
    operations:
    - CREATE
    - UPDATE
    resources:
    - neutronapis
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
---
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: mneutronapi.kb.io
webhooks:
- admissionReviewVersions:
  - v1
  clientConfig:
    caBundle: ${CA_BUNDLE}
    url: https://${CRC_IP}:9443/mutate-neutron-openstack-org-v1beta1-neutronapi
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: mneutronapi.kb.io
  objectSelector: {}
  rules:
  - apiGroups:
    - neutron.openstack.org
    apiVersions:
    - v1beta1
    operations:
    - CREATE
    - UPDATE
    resources:
    - neutronapis
    scope: '*'
  sideEffects: None
  timeoutSeconds: 10
EOF_CAT

oc apply -n openstack -f ${TMPDIR}/patch_webhook_configurations.yaml
