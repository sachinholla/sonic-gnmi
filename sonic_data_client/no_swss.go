//go:build noswss
// +build noswss

package client

import (
	"errors"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
)

/* stubs to bypass unsupported features when swss is not available */

func NewEventClient(paths []*gnmipb.Path, prefix *gnmipb.Path, logLevel int) (Client, error) {
	return nil, errors.New("EventClient not supported")
}

func NewMixedDbClient(paths []*gnmipb.Path, prefix *gnmipb.Path, origin string) (Client, error) {
	return nil, errors.New("MixedDbClient not supported")
}
