package bluegreen

import (
	"errors"
	"fmt"
)

type LoginError struct {
	LoginErrors []error
}

func (e LoginError) Error() string {
	errs := makeErrorString(e.LoginErrors)
	return fmt.Sprintf("login failed: %s", errs)
}

func (e LoginError) Code() string {
	return "LoginError"
}

type PushError struct {
	PushErrors []error
}

func (e PushError) Error() string {
	errs := makeErrorString(e.PushErrors)
	return fmt.Sprintf("push failed: %s", errs)
}

func (e PushError) Code() string {
	return "PushError"
}

type RollbackError struct {
	PushErrors     []error
	RollbackErrors []error
}

func (e RollbackError) Error() string {
	var (
		pushErrs       = makeErrorString(e.PushErrors)
		rollbackErrors = makeErrorString(e.RollbackErrors)
	)

	return fmt.Sprintf("push failed: %s: rollback failed: %s", pushErrs, rollbackErrors)
}

func (e RollbackError) Code() string {
	return "RollbackError"
}

type FinishPushError struct {
	FinishPushError []error
}

func (e FinishPushError) Error() string {
	var (
		finishPushErrors = makeErrorString(e.FinishPushError)
	)

	return fmt.Sprintf("finish push failed: %s", finishPushErrors)
}

func (e FinishPushError) Code() string {
	return "FinishPushError"
}

type InitializationError struct {
	Err error
}

func (e InitializationError) Error() string {
	return e.Err.Error()
}

func (e InitializationError) Code() string {
	return "InitError"
}

type FinishDeployError struct {
	Err error
}

func (e FinishDeployError) Error() string {
	return e.Err.Error()
}

func (e FinishDeployError) Code() string {
	return "FinishDeployError"
}

func makeErrorString(manyErrors []error) error {
	var result string
	for i, e := range manyErrors {
		if len(e.Error()) != 0 {
			if i == 0 {
				result = e.Error()
			} else {
				result = fmt.Sprintf("%s: %s", result, e.Error())
			}
		}
	}

	return errors.New(result)
}
