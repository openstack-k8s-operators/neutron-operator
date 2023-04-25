package neutronapi

import (
	"github.com/openstack-k8s-operators/lib-common/modules/storage"
)

const (
	// ServiceName -
	ServiceName = "neutron"
	// ServiceType -
	ServiceType = "network"
	// KollaConfigAPI -
	KollaConfigAPI = "/var/lib/config-data/merged/neutron-api-config.json"
	// KollaConfigDbSync -
	KollaConfigDbSync = "/var/lib/config-data/merged/db-sync-config.json"
	// Database -
	Database = "neutron"

	// NeutronAdminPort -
	NeutronAdminPort int32 = 9696
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
)

// DbsyncPropagation keeps track of the DBSync Service Propagation Type
var DbsyncPropagation = []storage.PropagationType{storage.DBSync}

// NeutronAPIPropagation is the  definition of the NeutronAPI propagation group
// It allows the NeutronAPI pod to mount volumes destined to Neutron and NeutronAPI
// ServiceTypes
var NeutronAPIPropagation = []storage.PropagationType{Neutron, NeutronAPI}
