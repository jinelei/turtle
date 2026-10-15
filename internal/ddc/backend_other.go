//go:build !windows && !darwin

package ddc

import (
	"context"
	"errors"

	"turtle/internal/monitor"
)

func NewBackend() (monitor.Backend, error) {
	return &unsupportedBackend{}, nil
}

type unsupportedBackend struct{}

var errPlatform = &monitor.Error{
	Kind: monitor.KindPlatformUnsupported,
	Op:   "backend",
	Err:  errors.New("DDC/CI is only implemented for Windows and macOS in this version"),
}

func (b *unsupportedBackend) Enumerate(context.Context) ([]monitor.Monitor, error) {
	return nil, errPlatform
}

func (b *unsupportedBackend) GetVCP(context.Context, string, uint8) (monitor.VCPValue, error) {
	return monitor.VCPValue{}, errPlatform
}

func (b *unsupportedBackend) SetVCP(context.Context, string, uint8, uint16) error {
	return errPlatform
}

func (b *unsupportedBackend) Capabilities(context.Context, string) (string, error) {
	return "", errPlatform
}

func (b *unsupportedBackend) Close() error { return nil }
