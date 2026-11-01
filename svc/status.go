package svc

// Status is a state of the App lifecycle. The App moves forward only:
//
//	Created → Initializing → Starting → Running → Draining → Stopping → Stopped
//
// A failed or canceled startup goes from Initializing or Starting to Stopping.
// Shutdown before Launch goes from Created to Stopped.
type Status int

// Lifecycle states.
const (
	StatusCreated Status = iota
	StatusInitializing
	StatusStarting
	StatusRunning
	StatusDraining
	StatusStopping
	StatusStopped
)

func (s Status) String() string {
	switch s {
	case StatusCreated:
		return "Created"
	case StatusInitializing:
		return "Initializing"
	case StatusStarting:
		return "Starting"
	case StatusRunning:
		return "Running"
	case StatusDraining:
		return "Draining"
	case StatusStopping:
		return "Stopping"
	case StatusStopped:
		return "Stopped"
	default:
		return "Unknown"
	}
}
