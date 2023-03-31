//go:build noswss
// +build noswss

package client

import (
	"errors"
	"unsafe"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
)

/* stubs to bypass unsupported features when swss is not available */

func NewEventClient(paths []*gnmipb.Path, prefix *gnmipb.Path, logLevel int) (Client, error) {
	return nil, errors.New("EventClient not supported")
}

func NewMixedDbClient(paths []*gnmipb.Path, prefix *gnmipb.Path, origin string, encoding gnmipb.Encoding, zmqAddress string) (Client, error) {
	return nil, errors.New("MixedDbClient not supported")
}

/* Mock vars/funcs used by gnmi_server gotests */

type Evt_rcvd struct {
	Event_str        string
	Missed_cnt       uint32
	Publish_epoch_ms int64
}

const SUBSCRIBER_TIMEOUT = (2 * 1000)

var PyCodeForYang = ""

func C_init_subs(use_cache bool) unsafe.Pointer { return nil }

func C_deinit_subs(h unsafe.Pointer) {}

func C_recv_evt(h unsafe.Pointer) (int, Evt_rcvd) { return 0, Evt_rcvd{} }

func Set_heartbeat(val int) {}
