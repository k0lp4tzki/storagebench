package broker

import (
	"os/user"
	"strconv"
)

// lookupGIDByName resolves a group name to a gid via os/user. With the pure-Go
// resolver this reads /etc/group; with cgo it uses getgrnam. Either way it works
// for the local groups the broker cares about.
func lookupGIDByName(name string) (int, bool) {
	g, err := user.LookupGroup(name)
	if err != nil {
		return 0, false
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return 0, false
	}
	return gid, true
}
