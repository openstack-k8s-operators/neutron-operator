/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

// Neutron Condition Types used by API objects.
const (
	// NeutronAPIReadyCondition indicates if the NeutronAPI is operational
	NeutronAPIReadyCondition condition.Type = "NeutronAPIReady"
	// Neutron DHCP Agent ConfigReady indicates when the compute service config
	// is ready for <explain here>
	NeutronDhcpAgentConfigReady condition.Type = "Neutron DHCP Agent ConfigReady"
	// Neutron OVN Agent ConfigReady indicates when the compute service config
	// is ready for <explain here>
	NeutronOvnAgentConfigReady condition.Type = "Neutron OVN Agent ConfigReady"
	// Neutron OVN Metadata Agent ConfigReady indicates when the compute service config
	// is ready for <explain here>
	NeutronOvnMetadataConfigReady condition.Type = "Neutron OVN Metadata Agent ConfigReady"
	// Neutron SR-IOV Agent ConfigReady indicates when the compute service config
	// is ready for <explain here>
	NeutronSriovAgentConfigReady condition.Type = "Neutron SR-IOV Agent ConfigReady"
)

// Common Messages used by API objects.
const (
	// NeutronAPIReadyInitMessage
	NeutronAPIReadyInitMessage = "NeutronAPI not started"

	// NeutronAPIReadyErrorMessage
	NeutronAPIReadyErrorMessage = "NeutronAPI error occurred %s"

	//NeutronDhcpAgentConfigInitMessage
	NeutronDhcpAgentConfigInitMessage = " Neutron DHCP Agent config generation is not started"

	//NeutronDhcpAgentConfigErrorMessage
	NeutronDhcpAgentConfigErrorMessage = "Neutron DHCP Agent config generation error occurred %s"

	//NeutronOvnAgentConfigInitMessage
	NeutronOvnAgentConfigInitMessage = " Neutron OVN Agent config generation is not started"

	//NeutronOvnAgentConfigErrorMessage
	NeutronOvnAgentConfigErrorMessage = "Neutron OVN Agent config generation error occurred %s"

	//NeutronOvnMetadataConfigInitMessage
	NeutronOvnMetadataConfigInitMessage = " Neutron OVN Metadata Agent config generation is not started"

	//NeutronOvnMetadataConfigErrorMessage
	NeutronOvnMetadataConfigErrorMessage = "Neutron OVN Metadata Agent config generation error occurred %s"

	//NeutronSriovAgentConfigInitMessage
	NeutronSriovAgentConfigInitMessage = " Neutron SR-IOV Agent config generation is not started"

	//NeutronSriovAgentConfigErrorMessage
	NeutronSriovAgentConfigErrorMessage = "Neutron SR-IOV Agent config generation error occurred %s"
)
