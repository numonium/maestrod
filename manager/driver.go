package manager

import "github.com/cpg1111/maestrod/config"

// Driver is an interface for running maestro in
type Driver interface {
	Run(name, confTarget, hostVolume string, args []string) error
}

// PluginDriver is a function type that returns a driver
type PluginDriver func(maestroVersion string, conf *config.Server) *Driver
