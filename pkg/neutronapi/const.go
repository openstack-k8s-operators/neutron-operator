package neutronapi

const (
	// ServiceName -
	ServiceName = "neutron"
	// ServiceType -
	ServiceType = "network"
	// KollaConfigAPI -
	KollaConfigAPI = "/var/lib/config-data/merged/neutron-api-config.json"
	// KollaConfigDbSync -
	KollaConfigDbSync = "/var/lib/config-data/merged/db-sync-config.json"
	// ServiceAccountName -
	ServiceAccountName = "neutron-operator-neutron"
	// Database -
	Database = "neutron"

	// NeutronAdminPort -
	NeutronAdminPort int32 = 9696
	// NeutronPublicPort -
	NeutronPublicPort int32 = 9696
	// NeutronInternalPort -
	NeutronInternalPort int32 = 9696
)
