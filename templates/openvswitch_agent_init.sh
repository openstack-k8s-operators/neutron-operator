#!/bin//bash
#
# Copyright 2018 Red Hat Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may
# not use this file except in compliance with the License. You may obtain
# a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
# WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
# License for the specific language governing permissions and limitations
# under the License.

set -ex

# expect that the common.sh is in the same dir as the calling script
SCRIPTPATH="$( cd "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
. ${SCRIPTPATH}/common.sh --source-only

CONFIG_VOLUME=$1
if [ -z "${CONFIG_VOLUME}" ] ; then
  echo "No config volume specified!"
  exit 1
fi


# copy default configs to the tmp config volume
cp -a /etc/neutron/* ${CONFIG_VOLUME}/

LOCAL_IP=$(get_ip_address_from_network "tenant")
crudini --set ${CONFIG_VOLUME}/plugins/ml2/openvswitch_agent.ini ovs local_ip $LOCAL_IP

# create required bridges from bridge_mappings setting in openvswitch_agent.ini
IFS=',' read -r -a bridge_mappings <<< $(crudini --get ${CONFIG_VOLUME}/plugins/ml2/openvswitch_agent.ini ovs bridge_mappings)
for br in "${bridge_mappings[@]}"; do
  bridge=$(echo $br | awk -F ':' '{print $2}';)
  ovs-vsctl --may-exist add-br ${bridge} -- set Bridge ${bridge} fail-mode=secure
done
