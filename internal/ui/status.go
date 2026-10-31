package ui

// Connection renders the connection state of a server.
func Connection(connected bool) string {
	if connected {
		return Success.Render("● Connected")
	}
	return Faint.Render("○ Disconnected")
}
