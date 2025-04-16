// Package neutronapi contains Neutron API service management functionality.
package neutronapi

import (
	"github.com/openstack-k8s-operators/lib-common/modules/storage"
)

const (
	// ServiceName -
	ServiceName = "neutron"
	// ServiceType -
	ServiceType = "network"
	// Database - Name of the database used in CREATE DATABASE statement
	Database = "neutron"

	// DatabaseCRName - Name of the MariaDBDatabase CR
	DatabaseCRName = "neutron"

	// DatabaseUsernamePrefix - used by EnsureMariaDBAccount when a new username
	// is to be generated, e.g. "neutron_e5a4", "neutron_78bc", etc
	DatabaseUsernamePrefix = "neutron"

	// NeutronUID is the UID for the neutron user (neutron:neutron)
	NeutronUID int64 = 42435
	// NeutronGID is the GID for the neutron group
	NeutronGID int64 = 42435

	// NeutronPublicPort -
	NeutronPublicPort int32 = 9696
	// NeutronInternalPort -
	NeutronInternalPort int32 = 9696

	// NeutronExtraVolTypeUndefined can be used to label an extraMount which
	// is not associated with a specific backend
	NeutronExtraVolTypeUndefined storage.ExtraVolType = "Undefined"
	// NeutronAPI is the definition of the neutron-api group
	NeutronAPI storage.PropagationType = "NeutronAPI"
	// Neutron is the global ServiceType that refers to all the components deployed
	// by the neutron-operator
	Neutron storage.PropagationType = "Neutron"

	// NeutronOVNMetadataAgentSecretKey is the key in external Secret for Neutron OVN Metadata Agent with agent config
	NeutronOVNMetadataAgentSecretKey = "10-neutron-metadata.conf"

	// NeutronOVNAgentSecretKey is the key in external Secret for Neutron OVN Agent with agent config
	NeutronOVNAgentSecretKey = "10-neutron-ovn.conf" // #nosec

	// NeutronSriovAgentSecretKey is the key in external Secret for Neutron SR-IOV Agent with agent config
	NeutronSriovAgentSecretKey = "10-neutron-sriov.conf"

	// NeutronDhcpAgentSecretKey is the key in external Secret for Neutron DHCP Agent with agent config
	NeutronDhcpAgentSecretKey = "10-neutron-dhcp.conf"
)

// DbsyncPropagation keeps track of the DBSync Service Propagation Type
var DbsyncPropagation = []storage.PropagationType{storage.DBSync}

// NeutronAPIPropagation is the  definition of the NeutronAPI propagation group
// It allows the NeutronAPI pod to mount volumes destined to Neutron and NeutronAPI
// ServiceTypes
var NeutronAPIPropagation = []storage.PropagationType{Neutron, NeutronAPI}
