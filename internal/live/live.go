package live

import "m31labs.dev/gosx/hub"

var Dashboard = hub.New("programma-dashboard")

func init() {
	Dashboard.SetState("status", "ready")
}

func Broadcast(event string, payload any) {
	Dashboard.Broadcast(event, payload)
}
