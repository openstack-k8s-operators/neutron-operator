#!/bin/sh

pod=$(oc get pods -n $NAMESPACE -l service=neutron --field-selector=status.phase=Running -o name|head -1)

# Check if the neutron pod logs contain DEBUG messages
oc logs -n $NAMESPACE "$pod" | grep -q "DEBUG"
exit $?
