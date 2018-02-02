package mocks

import "github.com/compozed/deployadactyl/interfaces"

type ErrorFinder struct {
	FindErrorsCall struct {
		Received struct {
			Response string
		}
		Returns struct {
			Errors []interfaces.DeploymentError
		}
	}
}

func (e *ErrorFinder) FindErrors(responseString string) []interfaces.DeploymentError {
	e.FindErrorsCall.Received.Response = responseString
	return e.FindErrorsCall.Returns.Errors
}
