#!/bin/bash
set -e
if [[ -f "/env/${K8S_NODE}" ]]; then
  set -o allexport
  source "/env/${K8S_NODE}"
  set +o allexport
fi
chown -R openvswitch:openvswitch /run/openvswitch
chown -R openvswitch:openvswitch /etc/openvswitch
function quit {
    # Don't allow ovs-vswitchd to clear datapath flows on exit
    kill -9 $(cat /var/run/openvswitch/ovs-vswitchd.pid 2>/dev/null) 2>/dev/null || true
    kill $(cat /var/run/openvswitch/ovsdb-server.pid 2>/dev/null) 2>/dev/null || true
    exit 0
}
trap quit SIGTERM
/usr/share/openvswitch/scripts/ovs-ctl start --ovs-user=openvswitch:openvswitch --system-id=random
ovs-appctl vlog/set "file:${OVS_LOG_LEVEL}"
/usr/share/openvswitch/scripts/ovs-ctl --protocol=udp --dport=6081 enable-protocol

sleep 5
export OVN_NODE_IP=`ip -4 -o addr show "${NIC}" | awk '{print $4}' | cut -d"/" -f1`

ovs-vsctl --may-exist add-br br-ex -- set Bridge br-ex fail-mode=secure

ovs-vsctl set open . external-ids:ovn-bridge-${HOSTNAME}-osp=br-int-osp
ovs-vsctl set open . external-ids:ovn-remote-${HOSTNAME}-osp=${OVN_SB_REMOTE}
ovs-vsctl set open . external-ids:ovn-encap-type-${HOSTNAME}-osp=geneve
ovs-vsctl set open . external-ids:ovn-encap-ip-${HOSTNAME}-osp="${OVN_NODE_IP}"
ovs-vsctl set open . external_ids:hostname="${HOSTNAME}"

tail -F --pid=$(cat /var/run/openvswitch/ovs-vswitchd.pid) /var/log/openvswitch/ovs-vswitchd.log &
tail -F --pid=$(cat /var/run/openvswitch/ovsdb-server.pid) /var/log/openvswitch/ovsdb-server.log &
wait
