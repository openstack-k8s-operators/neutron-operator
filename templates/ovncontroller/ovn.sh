#!/bin/bash
set -e
if [[ -f "/env/${K8S_NODE}" ]]; then
  set -o allexport
  source "/env/${K8S_NODE}"
  set +o allexport
fi
# Determine the ovn rundir.
if [[ -f /usr/bin/ovn-appctl ]] ; then
    # ovn-appctl is present. Use new ovn run dir path.
    OVNCTL_DIR=ovn
else
    # ovn-appctl is not present. Use openvswitch run dir path.
    OVNCTL_DIR=openvswitch
fi

exec ovn-controller -n ${HOSTNAME}-osp unix:/var/run/openvswitch/db.sock -vfile:off \
  --no-chdir --pidfile=/var/run/${OVNCTL_DIR}/ovn-controller.pid \
  -vconsole:"${OVN_LOG_LEVEL}"
