package interfaces

import (
	"io"

	S "github.com/compozed/deployadactyl/structs"
)

// BlueGreener interface.
type BlueGreener interface {
	Push(
		environment S.Environment,
		appPath string,
		deploymentInfo S.DeploymentInfo,
		response io.ReadWriter,
	) DeploymentError
}
