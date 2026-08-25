package platform

type ServiceManager interface {
	Register(name, displayName, description string) error
	Unregister(name string) error
	IsRegistered(name string) (bool, error)
	Start(name string) error
	Stop(name string) error
	Run(name string, handler func() error) error
	SetConfigPath(name, path string) error
	GetConfigPath(name string) (string, error)
}
