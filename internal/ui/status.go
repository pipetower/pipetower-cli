package ui

// Connection renders the connection state of a server.
func Connection(connected bool) string {
	if connected {
		return ConnectionMark(true) + Success.Render(" Connected")
	}
	return ConnectionMark(false) + Muted.Render(" Disconnected")
}

// ConnectionMark renders the connection state of a server as a dot, green or gray like in the web app.
func ConnectionMark(connected bool) string {
	if connected {
		return Success.Render("●")
	}
	return Muted.Render("●")
}
