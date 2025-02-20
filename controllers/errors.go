package controllers

import (
	"errors"
)

var (
	ErrCreateFailed    = errors.New("failed to create")
	ErrFetchFailed     = errors.New("failed to fetch")
	ErrDeleteFailed    = errors.New("failed to delete")
	ErrEnsureFailed    = errors.New("failed to ensure")
	ErrReconcileFailed = errors.New("failed to reconcile")
	ErrKeyNotFound     = errors.New("key not found in")
)
