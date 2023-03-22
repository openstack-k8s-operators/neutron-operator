#!/bin/bash
set -ex

oc delete validatingwebhookconfiguration vneutronapi.kb.io --ignore-not-found
oc delete mutatingwebhookconfiguration mneutronapi.kb.io --ignore-not-found
