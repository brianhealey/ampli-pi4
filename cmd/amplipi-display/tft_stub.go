//go:build !linux

package main

import "errors"

// TFT is a stub for non-Linux platforms.
type TFT struct{}

// NewTFT returns an error on non-Linux platforms.
func NewTFT() (*TFT, error) {
	return nil, errors.New("TFT display only supported on Linux")
}

// RenderStatus is a no-op stub.
func (t *TFT) RenderStatus(status *Status) error {
	return errors.New("TFT display only supported on Linux")
}
