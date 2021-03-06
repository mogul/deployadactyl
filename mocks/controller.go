package mocks

import (
	"bytes"
	"fmt"

	"github.com/gin-gonic/gin"

	I "github.com/compozed/deployadactyl/interfaces"
)

type Controller struct {
	RunDeploymentCall struct {
		Called   bool
		Received struct {
			Deployment *I.Deployment
			Response   *bytes.Buffer
			UUID       string
		}
		Write struct {
			Output string
		}
		Returns I.DeployResponse
	}
	RunDeploymentViaHttpCall struct {
		Called   bool
		Received struct {
			Context *gin.Context
		}
	}
}

func (c *Controller) RunDeployment(deployment *I.Deployment, response *bytes.Buffer) I.DeployResponse {
	c.RunDeploymentCall.Called = true

	c.RunDeploymentCall.Received.Deployment = deployment
	c.RunDeploymentCall.Received.Response = response
	c.RunDeploymentCall.Received.UUID = deployment.CFContext.UUID

	fmt.Fprint(response, c.RunDeploymentCall.Write.Output)

	return c.RunDeploymentCall.Returns
}

func (c *Controller) RunDeploymentViaHttp(g *gin.Context) {
	c.RunDeploymentViaHttpCall.Called = true

	c.RunDeploymentViaHttpCall.Received.Context = g
}
