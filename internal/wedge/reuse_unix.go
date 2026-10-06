//go:build !windows

package wedge

import "syscall"

// reuseAddr sets SO_REUSEADDR so discovery can share UDP 4992 with other
// listeners on the same host.
func reuseAddr(_, _ string, c syscall.RawConn) error {
	var serr error
	err := c.Control(func(fd uintptr) {
		serr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	})
	if err != nil {
		return err
	}
	return serr
}
