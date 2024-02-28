#!/bin/sh

pod=$(oc get pods -n $NAMESPACE -l service=neutron -o name)

# Check if the neutron pod logs contain DEBUG messages
oc logs -n $NAMESPACE "$pod" | grep -q "DEBUG"
exit $?
