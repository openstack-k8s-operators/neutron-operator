package controller

import (
	"github.com/neutron-operator/pkg/controller/neutronovsagent"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, neutronovsagent.Add)
}
