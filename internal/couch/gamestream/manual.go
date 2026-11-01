package gamestream

import (
	"context"
	"net"

	"github.com/wittywilla/wittymodule/service"
)

// CheckAddress resolves a cleaned address and says what kind of place it is. The lookup is the
// module's own, whatever's given here: a server never gives one.
func CheckAddress(ctx context.Context, host string, _ func(context.Context, string) ([]net.IPAddr, error)) (Reach, error) {
	var r Reach
	err := service.Call(ctx, "wittycouch", "gamestream.CheckAddress", nil, nil, []any{host}, []any{&r})
	return r, err
}
