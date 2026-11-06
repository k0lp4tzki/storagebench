package broker

import (
	"fmt"
	"net"
	"os"
	"strconv"
)

// Listen creates a Unix domain socket listener at path with restrictive
// permissions. Any stale socket file is removed first. If groupName resolves to
// a gid, the socket is chowned to that group and made group-accessible (0660) so
// the (unprivileged) agent in that group can connect while the world cannot.
// Otherwise the socket is owner-only (0600).
func Listen(path, groupName string) (net.Listener, error) {
	// Remove a stale socket from a previous run.
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("remove stale socket: %w", err)
		}
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen unix %s: %w", path, err)
	}

	mode := os.FileMode(0o600)
	if gid, ok := lookupGID(groupName); ok {
		if err := os.Chown(path, -1, gid); err != nil {
			ln.Close()
			return nil, fmt.Errorf("chown socket to gid %d: %w", gid, err)
		}
		mode = 0o660
	}
	if err := os.Chmod(path, mode); err != nil {
		ln.Close()
		return nil, fmt.Errorf("chmod socket: %w", err)
	}
	return ln, nil
}

// lookupGID resolves a group name (or numeric gid string) to a gid. Returns
// ok=false if groupName is empty or cannot be resolved without cgo on this
// platform; callers then fall back to owner-only permissions.
func lookupGID(groupName string) (int, bool) {
	if groupName == "" {
		return 0, false
	}
	// Accept a numeric gid directly (works without cgo).
	if gid, err := strconv.Atoi(groupName); err == nil {
		return gid, true
	}
	return lookupGIDByName(groupName)
}
