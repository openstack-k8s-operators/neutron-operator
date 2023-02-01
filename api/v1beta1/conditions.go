/*
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

//
// Neutron Condition Types used by API objects.
//

const (
	// NeutronRabbitMqTransportURLReadyCondition Status=True condition which indicates if the RabbitMQ TransportURLUrl is ready
	NeutronRabbitMqTransportURLReadyCondition condition.Type = "NeutronRabbitMqTransportURLReady"
)

// Neutron Reasons used by API objects.
const ()

// Common Messages used by API objects.
const (
	//
	// NeutronRabbitMqTransportURLReady condition messages
	//
	// NeutronRabbitMqTransportURLReadyInitMessage
	NeutronRabbitMqTransportURLReadyInitMessage = "NeutronRabbitMqTransportURL not started"

	// NeutronRabbitMqTransportURLReadyRunningMessage
	NeutronRabbitMqTransportURLReadyRunningMessage = "NeutronRabbitMqTransportURL creation in progress"

	// NeutronRabbitMqTransportURLReadyMessage
	NeutronRabbitMqTransportURLReadyMessage = "NeutronRabbitMqTransportURL successfully created"

	// NeutronRabbitMqTransportURLReadyErrorMessage
	NeutronRabbitMqTransportURLReadyErrorMessage = "NeutronRabbitMqTransportURL error occured %s"
)
