package controller_test

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"

	"io/ioutil"

	"os"

	. "github.com/compozed/deployadactyl/controller"
	"github.com/compozed/deployadactyl/logger"
	"github.com/compozed/deployadactyl/mocks"
	"github.com/compozed/deployadactyl/randomizer"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/op/go-logging"
	//"github.com/compozed/deployadactyl/constants"
	//"github.com/compozed/deployadactyl/interfaces"
	"github.com/compozed/deployadactyl/constants"
)

const (
	deployerNotEnoughCalls = "deployer didn't have the right number of calls"
)

var _ = Describe("Controller", func() {

	var (
		deployer   *mocks.Deployer
		silentDeployer *mocks.Deployer
		controller *Controller
		router     *gin.Engine
		resp       *httptest.ResponseRecorder
		jsonBuffer *bytes.Buffer

		foundationURL string
		appName       string
		environment   string
		org           string
		space         string
		contentType   string
		byteBody      []byte
		server        *httptest.Server
	)

	BeforeEach(func() {
		deployer = &mocks.Deployer{}
		silentDeployer = &mocks.Deployer{}

		controller = &Controller{
			Deployer: deployer,
			SilentDeployer: silentDeployer,
			Log:      logger.DefaultLogger(GinkgoWriter, logging.DEBUG, "api_test"),
		}

		router = gin.New()
		resp = httptest.NewRecorder()
		jsonBuffer = &bytes.Buffer{}

		appName = "appName-" + randomizer.StringRunes(10)
		environment = "environment-" + randomizer.StringRunes(10)
		org = "org-" + randomizer.StringRunes(10)
		space = "non-prod"
		contentType = "application/json"
		
		router.POST("/v2/deploy/:environment/:org/:space/:appName", controller.RunDeploymentViaHttp)


		server = httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
			byteBody, _ = ioutil.ReadAll(req.Body)
		}))

		silentDeployUrl := server.URL + "/v1/apps/" + os.Getenv("SILENT_DEPLOY_ENVIRONMENT") + "/%s/%s/%s"
		os.Setenv("SILENT_DEPLOY_URL", silentDeployUrl)
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("RunDeploymentViaHttp handler", func() {
		Context("when deployer succeeds", func() {
			It("deploys and returns http.StatusOK", func() {
				foundationURL = fmt.Sprintf("/v2/deploy/%s/%s/%s/%s", environment, org, space, appName)

				req, err := http.NewRequest("POST", foundationURL, jsonBuffer)
				Expect(err).ToNot(HaveOccurred())

				deployer.DeployCall.Returns.Error = nil
				deployer.DeployCall.Returns.StatusCode = http.StatusOK
				deployer.DeployCall.Write.Output = "deploy success"

				router.ServeHTTP(resp, req)

				Eventually(resp.Code).Should(Equal(http.StatusOK))
				Eventually(resp.Body).Should(ContainSubstring("deploy success"))

				Eventually(deployer.DeployCall.Received.Environment).Should(Equal(environment))
				Eventually(deployer.DeployCall.Received.Org).Should(Equal(org))
				Eventually(deployer.DeployCall.Received.Space).Should(Equal(space))
				Eventually(deployer.DeployCall.Received.AppName).Should(Equal(appName))
			})

			It("does not run silent deploy when environment other than non-prop", func() {
				foundationURL = fmt.Sprintf("/v2/deploy/%s/%s/%s/%s", environment, org, "not-non-prod", appName)

				req, err := http.NewRequest("POST", foundationURL, jsonBuffer)
				Expect(err).ToNot(HaveOccurred())

				deployer.DeployCall.Returns.Error = nil
				deployer.DeployCall.Returns.StatusCode = http.StatusOK
				deployer.DeployCall.Write.Output = "deploy success"

				router.ServeHTTP(resp, req)

				Eventually(resp.Code).Should(Equal(http.StatusOK))
				Eventually(resp.Body).Should(ContainSubstring("deploy success"))

				Eventually(len(byteBody)).Should(Equal(0))
			})
		})

		Context("when deployer fails", func() {
			It("doesn't deploy and gives http.StatusInternalServerError", func() {
				foundationURL = fmt.Sprintf("/v2/deploy/%s/%s/%s/%s", environment, org, space, appName)

				req, err := http.NewRequest("POST", foundationURL, jsonBuffer)
				Expect(err).ToNot(HaveOccurred())

				deployer.DeployCall.Returns.Error = errors.New("bork")
				deployer.DeployCall.Returns.StatusCode = http.StatusInternalServerError

				router.ServeHTTP(resp, req)

				Eventually(resp.Code).Should(Equal(http.StatusInternalServerError))
				Eventually(resp.Body).Should(ContainSubstring("bork"))
			})
		})

		Context("when parameters are added to the url", func() {
			It("does not return an error", func() {
				foundationURL = fmt.Sprintf("/v2/deploy/%s/%s/%s/%s?broken=false", environment, org, space, appName)

				req, err := http.NewRequest("POST", foundationURL, jsonBuffer)
				Expect(err).ToNot(HaveOccurred())

				deployer.DeployCall.Write.Output = "deploy success"
				deployer.DeployCall.Returns.StatusCode = http.StatusOK

				router.ServeHTTP(resp, req)

				Eventually(resp.Code).Should(Equal(http.StatusOK))
				Eventually(resp.Body).Should(ContainSubstring("deploy success"))
			})
		})
	})

	Describe("RunDeployment", func() {
		Context("when verbose deployer is called", func () {
			It("channel resolves when no errors occur", func() {

				deployer.DeployCall.Returns.Error = nil
				deployer.DeployCall.Returns.StatusCode = http.StatusOK
				deployer.DeployCall.Write.Output = "little-timmy-env.zip"

				response := &bytes.Buffer{}

				deployment:= &Deployment{
					Body: &[]byte{},
					Type: constants.DeploymentType{ JSON: true },
					CFContext: CFContext{
						Environment: environment,
						Organization: org,
						Space: space,
						Application: appName,
					},
				}
				response, statusCode, _ := controller.RunDeployment(deployment, response)

				Eventually(deployer.DeployCall.Called).Should(Equal(1))
				Eventually(silentDeployer.DeployCall.Called).Should(Equal(0))

				Eventually(statusCode).Should(Equal(http.StatusOK))

				Eventually(deployer.DeployCall.Received.Environment).Should(Equal(environment))
				Eventually(deployer.DeployCall.Received.ContentType).Should(Equal(constants.DeploymentType{ JSON: true }))
				Eventually(deployer.DeployCall.Received.Org).Should(Equal(org))
				Eventually(deployer.DeployCall.Received.Space).Should(Equal(space))
				Eventually(deployer.DeployCall.Received.AppName).Should(Equal(appName))

				ret, _ := ioutil.ReadAll(response)
				Eventually(string(ret)).Should(Equal("little-timmy-env.zip"))
			})
			It("channel resolves when errors occur", func() {

				deployer.DeployCall.Returns.Error = errors.New("bork")
				deployer.DeployCall.Returns.StatusCode = http.StatusInternalServerError
				deployer.DeployCall.Write.Output = "little-timmy-env.zip"

				response := &bytes.Buffer{}

				deployment:= &Deployment{
					Body: &[]byte{},
					Type: constants.DeploymentType{ JSON: true },
					CFContext: CFContext{
						Environment: environment,
						Organization: org,
						Space: space,
						Application: appName,
					},
				}
				response, statusCode, err := controller.RunDeployment(deployment, response)

				Eventually(deployer.DeployCall.Called).Should(Equal(1))
				Eventually(silentDeployer.DeployCall.Called).Should(Equal(0))

				Eventually(statusCode).Should(Equal(http.StatusInternalServerError))
				Eventually(err.Error()).Should(Equal("bork"))

				Eventually(deployer.DeployCall.Received.Environment).Should(Equal(environment))
				Eventually(deployer.DeployCall.Received.ContentType).Should(Equal(constants.DeploymentType{ JSON: true }))
				Eventually(deployer.DeployCall.Received.Org).Should(Equal(org))
				Eventually(deployer.DeployCall.Received.Space).Should(Equal(space))
				Eventually(deployer.DeployCall.Received.AppName).Should(Equal(appName))

				ret, _ := ioutil.ReadAll(response)
				Eventually(string(ret)).Should(Equal("little-timmy-env.zip"))
			})
		})
		Context("when SILENT_DEPLOY_ENVIRONMENT is true", func() {
			It("channel resolves true when no errors occur", func() {

				os.Setenv("SILENT_DEPLOY_ENVIRONMENT", environment)
				deployer.DeployCall.Returns.Error = nil
				deployer.DeployCall.Returns.StatusCode = http.StatusOK
				deployer.DeployCall.Write.Output = "little-timmy-env.zip"

				response := &bytes.Buffer{}

				deployment:= &Deployment{
					Body: &[]byte{},
					Type: constants.DeploymentType{ JSON: true },
					CFContext: CFContext{
						Environment: environment,
						Organization: org,
						Space: space,
						Application: appName,
					},
				}
				response, statusCode, _ := controller.RunDeployment(deployment, response)

				Eventually(deployer.DeployCall.Called).Should(Equal(1))
				Eventually(silentDeployer.DeployCall.Called).Should(Equal(1))

				Eventually(statusCode).Should(Equal(http.StatusOK))

				Eventually(deployer.DeployCall.Received.Environment).Should(Equal(environment))
				Eventually(deployer.DeployCall.Received.ContentType).Should(Equal(constants.DeploymentType{ JSON: true }))
				Eventually(deployer.DeployCall.Received.Org).Should(Equal(org))
				Eventually(deployer.DeployCall.Received.Space).Should(Equal(space))
				Eventually(deployer.DeployCall.Received.AppName).Should(Equal(appName))


				ret, _ := ioutil.ReadAll(response)
				Eventually(string(ret)).Should(Equal("little-timmy-env.zip"))
			})
			It("channel resolves when no errors occur", func() {

				os.Setenv("SILENT_DEPLOY_ENVIRONMENT", environment)
				deployer.DeployCall.Returns.Error = nil
				deployer.DeployCall.Returns.StatusCode = http.StatusOK
				deployer.DeployCall.Write.Output = "little-timmy-env.zip"

				silentDeployer.DeployCall.Returns.Error = errors.New("bork")
				silentDeployer.DeployCall.Returns.StatusCode = http.StatusInternalServerError

				response := &bytes.Buffer{}

				deployment:= &Deployment{
					Body: &[]byte{},
					Type: constants.DeploymentType{ JSON: true },
					CFContext: CFContext{
						Environment: environment,
						Organization: org,
						Space: space,
						Application: appName,
					},
				}
				response, statusCode, _ := controller.RunDeployment(deployment, response)

				Eventually(deployer.DeployCall.Called).Should(Equal(1))
				Eventually(silentDeployer.DeployCall.Called).Should(Equal(1))

				Eventually(statusCode).Should(Equal(http.StatusOK))

				Eventually(deployer.DeployCall.Received.Environment).Should(Equal(environment))
				Eventually(deployer.DeployCall.Received.ContentType).Should(Equal(constants.DeploymentType{ JSON: true }))
				Eventually(deployer.DeployCall.Received.Org).Should(Equal(org))
				Eventually(deployer.DeployCall.Received.Space).Should(Equal(space))
				Eventually(deployer.DeployCall.Received.AppName).Should(Equal(appName))

				ret, _ := ioutil.ReadAll(response)
				Eventually(string(ret)).Should(Equal("little-timmy-env.zip"))
			})
		})
	})
})
