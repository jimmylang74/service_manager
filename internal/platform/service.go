package platform

// ServiceManager defines the platform-specific service registration interface.
type ServiceManager interface {
	// Register registers the current binary as a system service.
	Register(name, displayName, description string) error
	// Unregister removes the service registration.
	Unregister(name string) error
	// IsRegistered checks if the service is already registered.
	IsRegistered(name string) (bool, error)
	// Run starts the service and blocks. This is called when the OS starts the service.
	Run(name string, handler func() error) error
	// SetConfigPath stores the config path for the service to use on start.
	SetConfigPath(name, path string) error
	// GetConfigPath retrieves the stored config path.
	GetConfigPath(name string) (string, error)
}
