package live

import "m31labs.dev/gosx/hub"

// maxClients bounds the number of concurrent WebSocket connections the
// dashboard hub accepts (SE-8/M9). Without a cap, an unbounded number of
// open connections can exhaust server memory or file descriptors; the hub
// refuses any connection past this count rather than accepting it.
const maxClients = 256

var Dashboard = hub.New("rostrum-dashboard")

func init() {
	Dashboard.SetState("status", "ready")
	Dashboard.MaxClients = maxClients
}

func Broadcast(event string, payload any) {
	Dashboard.Broadcast(event, payload)
}
