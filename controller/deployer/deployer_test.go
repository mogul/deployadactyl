package deployer_test

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"math/rand"
	"net/http"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
	"github.com/op/go-logging"
	"github.com/spf13/afero"

	"github.com/compozed/deployadactyl/config"
	C "github.com/compozed/deployadactyl/constants"
	. "github.com/compozed/deployadactyl/controller/deployer"
	"github.com/compozed/deployadactyl/controller/deployer/bluegreen"
	"github.com/compozed/deployadactyl/controller/deployer/error_finder"
	"github.com/compozed/deployadactyl/interfaces"
	"github.com/compozed/deployadactyl/logger"
	"github.com/compozed/deployadactyl/mocks"
	"github.com/compozed/deployadactyl/randomizer"
	S "github.com/compozed/deployadactyl/structs"
)

const (
	testManifest = `---
applications:
- name: deployadactyl
  memory: 256M
  disk_quota: 256M
`
	eventManagerNotEnoughCalls = "event manager didn't have the right number of calls"
)

var _ = Describe("Deployer", func() {
	var (
		deployer Deployer

		c              config.Config
		blueGreener    *mocks.BlueGreener
		fetcher        *mocks.Fetcher
		prechecker     *mocks.Prechecker
		eventManager   *mocks.EventManager
		randomizerMock *mocks.Randomizer
		errorFinder    *mocks.ErrorFinder

		req                          *http.Request
		requestBody                  *bytes.Buffer
		appName                      string
		appPath                      string
		artifactURL                  string
		domain                       string
		environment                  string
		org                          string
		space                        string
		username                     string
		uuid                         string
		manifest                     string
		instances                    uint16
		password                     string
		testManifestLocation         string
		response                     *bytes.Buffer
		logBuffer                    *Buffer
		log                          interfaces.Logger
		deploymentInfo               S.DeploymentInfo
		deploymentInfoNoCustomParams S.DeploymentInfo
		foundations                  []string
		enableRollback               bool
		environments                 = map[string]S.Environment{}
		environmentsNoCustomParams   = map[string]S.Environment{}
		af                           *afero.Afero
	)

	BeforeEach(func() {
		blueGreener = &mocks.BlueGreener{}
		fetcher = &mocks.Fetcher{}
		prechecker = &mocks.Prechecker{}
		eventManager = &mocks.EventManager{}
		randomizerMock = &mocks.Randomizer{}
		errorFinder = &mocks.ErrorFinder{}

		appName = "appName-" + randomizer.StringRunes(10)
		appPath = "appPath-" + randomizer.StringRunes(10)
		artifactURL = "artifactURL-" + randomizer.StringRunes(10)
		domain = "domain-" + randomizer.StringRunes(10)
		environment = "environment-" + randomizer.StringRunes(10)
		org = "org-" + randomizer.StringRunes(10)
		password = "password-" + randomizer.StringRunes(10)
		space = "space-" + randomizer.StringRunes(10)
		username = "username-" + randomizer.StringRunes(10)
		uuid = "uuid-" + randomizer.StringRunes(10)
		manifest = "manifest-" + randomizer.StringRunes(10)
		instances = uint16(rand.Uint32())
		enableRollback = true

		base64Manifest := base64.StdEncoding.EncodeToString([]byte(manifest))

		randomizerMock.RandomizeCall.Returns.Runes = uuid
		eventManager.EmitCall.Returns.Error = append(eventManager.EmitCall.Returns.Error, nil)
		eventManager.EmitCall.Returns.Error = append(eventManager.EmitCall.Returns.Error, nil)
		eventManager.EmitCall.Returns.Error = append(eventManager.EmitCall.Returns.Error, nil)
		eventManager.EmitCall.Returns.Error = append(eventManager.EmitCall.Returns.Error, nil)

		requestBody = bytes.NewBufferString(fmt.Sprintf(`{
				"artifact_url": "%s",
				"manifest": "%s"
			}`,
			artifactURL,
			base64Manifest,
		))

		req, _ = http.NewRequest("POST", "", requestBody)

		customParams := make(map[string]interface{})
		customParams["service_now_column_name"] = "u_change"
		customParams["service_now_table_name"] = "u_table"

		deploymentInfo = S.DeploymentInfo{
			ArtifactURL:  artifactURL,
			Username:     username,
			Password:     password,
			Environment:  environment,
			Org:          org,
			Space:        space,
			AppName:      appName,
			UUID:         uuid,
			Instances:    instances,
			Manifest:     manifest,
			Domain:       domain,
			AppPath:      appPath,
			CustomParams: customParams,
		}

		deploymentInfoNoCustomParams = S.DeploymentInfo{
			ArtifactURL: artifactURL,
			Username:    username,
			Password:    password,
			Environment: environment,
			Org:         org,
			Space:       space,
			AppName:     appName,
			UUID:        uuid,
			Instances:   instances,
			Manifest:    manifest,
			Domain:      domain,
			AppPath:     appPath,
		}

		foundations = []string{randomizer.StringRunes(10)}
		response = &bytes.Buffer{}

		environments[environment] = S.Environment{
			Name:           environment,
			Domain:         domain,
			Foundations:    foundations,
			Instances:      instances,
			CustomParams:   customParams,
			EnableRollback: enableRollback,
		}

		c = config.Config{
			Username:     username,
			Password:     password,
			Environments: environments,
		}

		af = &afero.Afero{Fs: afero.NewMemMapFs()}

		testManifestLocation, _ = af.TempDir("", "")

		logBuffer = NewBuffer()
		log = logger.DefaultLogger(logBuffer, logging.DEBUG, "deployer tests")

		deployer = Deployer{
			c,
			blueGreener,
			fetcher,
			prechecker,
			eventManager,
			randomizerMock,
			errorFinder,
			log,
			af,
		}
	})

	Describe("prechecking the environments", func() {
		Context("when Prechecker fails", func() {
			It("rejects the request with a http.StatusInternalServerError", func() {
				prechecker.AssertAllFoundationsUpCall.Returns.Error = errors.New("prechecker failed")

				reqChannel1 := make(chan interfaces.DeployResponse)
				go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
				deployResponse := <-reqChannel1

				Expect(deployResponse.Error).To(MatchError("prechecker failed"))

				Expect(deployResponse.StatusCode).To(Equal(http.StatusInternalServerError))
				Expect(prechecker.AssertAllFoundationsUpCall.Received.Environment).To(Equal(environments[environment]))
			})
		})
	})

	Describe("authentication", func() {
		Context("a username and password are not provided", func() {
			Context("when authenticate in the config is not true", func() {
				It("uses the config username and password and accepts the request with a http.StatusOK", func() {
					By("setting authenticate to false")
					deployer.Config.Environments[environment] = S.Environment{Authenticate: false}

					By("not setting basic auth")

					reqChannel1 := make(chan interfaces.DeployResponse)
					go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
					deployResponse := <-reqChannel1

					Expect(deployResponse.Error).ToNot(HaveOccurred())
					Expect(deployResponse.StatusCode).To(Equal(http.StatusOK))

					Expect(response.String()).To(ContainSubstring("deploy was successful"))
					Expect(eventManager.EmitCall.TimesCalled).To(Equal(3), eventManagerNotEnoughCalls)
					Expect(response.String()).To(ContainSubstring(username))
				})
			})

			Context("when authenticate in the config is true", func() {
				It("rejects the request with a http.StatusUnauthorized", func() {
					deployer.Config.Environments[environment] = S.Environment{Authenticate: true}

					By("not setting basic auth")

					reqChannel1 := make(chan interfaces.DeployResponse)
					go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
					deployResponse := <-reqChannel1

					Expect(deployResponse.Error).To(MatchError("basic auth header not found"))

					Expect(deployResponse.StatusCode).To(Equal(http.StatusUnauthorized))
					Expect(eventManager.EmitCall.TimesCalled).To(Equal(0), eventManagerNotEnoughCalls)
				})
			})
		})
	})

	Describe("deploying with JSON in the request body", func() {
		Context("with missing properties in the JSON", func() {
			It("returns an error and http.StatusInternalServerError", func() {
				By("sending empty JSON")
				requestBody = bytes.NewBufferString("{}")

				req, _ = http.NewRequest("POST", "", requestBody)

				reqChannel1 := make(chan interfaces.DeployResponse)
				go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
				deployResponse := <-reqChannel1

				Expect(deployResponse.Error).To(MatchError("The following properties are missing: artifact_url"))

				Expect(deployResponse.StatusCode).To(Equal(http.StatusInternalServerError))
			})
		})

		Context("when manifest is given in the request body", func() {
			Context("if the provided manifest is base64 encoded", func() {
				It("decodes the manifest, does not return an error and returns http.StatusOK", func() {
					deploymentInfo.Manifest = "manifest-" + randomizer.StringRunes(10)

					By("base64 encoding the manifest")
					base64Manifest := base64.StdEncoding.EncodeToString([]byte(deploymentInfo.Manifest))

					By("including the manifest in the request body")
					requestBody = bytes.NewBufferString(fmt.Sprintf(`{"artifact_url": "%s", "manifest": "%s"}`,
						artifactURL,
						base64Manifest,
					))

					req, _ = http.NewRequest("POST", "", requestBody)

					reqChannel1 := make(chan interfaces.DeployResponse)
					go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
					deployResponse := <-reqChannel1

					Expect(deployResponse.Error).ToNot(HaveOccurred())

					Expect(deployResponse.StatusCode).To(Equal(http.StatusOK))
					Expect(fetcher.FetchCall.Received.Manifest).ToNot(Equal(base64Manifest), "manifest was not decoded")
				})
			})

			Context("if the provided manifest is not base64 encoded", func() {
				It("returns an error and http.StatusBadRequest", func() {
					deploymentInfo.Manifest = "manifest-" + randomizer.StringRunes(10)

					By("not base64 encoding the manifest")

					By("including the manifest in the JSON")
					requestBody = bytes.NewBufferString(fmt.Sprintf(`{"artifact_url": "%s", "manifest": "%s"}`,
						artifactURL,
						deploymentInfo.Manifest,
					))

					req, _ = http.NewRequest("POST", "", requestBody)

					reqChannel1 := make(chan interfaces.DeployResponse)
					go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
					deployResponse := <-reqChannel1

					Expect(deployResponse.Error.Error()).To(ContainSubstring("base64 encoded manifest could not be decoded"))

					Expect(deployResponse.StatusCode).To(Equal(http.StatusBadRequest))
				})
			})
		})

		Describe("fetching an artifact from an artifact url", func() {
			Context("when Fetcher fails", func() {
				It("returns an error and http.StatusInternalServerError", func() {
					fetcher.FetchCall.Returns.AppPath = ""
					fetcher.FetchCall.Returns.Error = errors.New("fetcher error")

					reqChannel1 := make(chan interfaces.DeployResponse)
					go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
					deployResponse := <-reqChannel1

					Expect(deployResponse.Error).To(MatchError("fetcher error"))

					Expect(deployResponse.StatusCode).To(Equal(http.StatusInternalServerError))
					Expect(fetcher.FetchCall.Received.ArtifactURL).To(Equal(artifactURL))
					Expect(fetcher.FetchCall.Received.Manifest).To(Equal(manifest))
				})
			})
		})

		Context("when a UUID is provided", func() {
			It("does not create a new UUID", func() {
				deploymentInfo.Manifest = "manifest-" + randomizer.StringRunes(10)
				base64Manifest := base64.StdEncoding.EncodeToString([]byte(deploymentInfo.Manifest))

				requestBody = bytes.NewBufferString(fmt.Sprintf(`{"artifact_url": "%s", "manifest": "%s"}`,
					artifactURL,
					base64Manifest,
				))

				req, _ = http.NewRequest("POST", "", requestBody)

				reqChannel1 := make(chan interfaces.DeployResponse)
				go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
				deployResponse := <-reqChannel1

				Expect(deployResponse.Error).ToNot(HaveOccurred())

				Expect(deployResponse.StatusCode).To(Equal(http.StatusOK))
				Expect(deployResponse.DeploymentInfo.UUID).To(Equal(uuid))

			})
		})

		Context("when no UUID is provided", func() {
			It("creates a new UUID", func() {
				deploymentInfo.Manifest = "manifest-" + randomizer.StringRunes(10)
				base64Manifest := base64.StdEncoding.EncodeToString([]byte(deploymentInfo.Manifest))

				requestBody = bytes.NewBufferString(fmt.Sprintf(`{"artifact_url": "%s", "manifest": "%s"}`,
					artifactURL,
					base64Manifest,
				))

				req, _ = http.NewRequest("POST", "", requestBody)
				uuid = ""
				reqChannel1 := make(chan interfaces.DeployResponse)
				go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
				deployResponse := <-reqChannel1

				Expect(deployResponse.Error).ToNot(HaveOccurred())

				Expect(deployResponse.StatusCode).To(Equal(http.StatusOK))
				Expect(deployResponse.DeploymentInfo.UUID).ToNot(Equal(uuid))

			})
		})
	})

	Describe("deploying with a zip file in the request body", func() {
		Context("when manifest file cannot be found in the extracted zip", func() {
			It("deploys successfully and returns http.StatusOK because manifest is optional", func() {

				reqChannel1 := make(chan interfaces.DeployResponse)
				go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{ZIP: true}, response, reqChannel1)
				deployResponse := <-reqChannel1

				Expect(deployResponse.Error).To(BeNil())

				Expect(deployResponse.StatusCode).To(Equal(http.StatusOK))
				Expect(response.String()).To(ContainSubstring("deploy was successful"))
			})
		})

		Describe("fetching an artifact from the request body", func() {
			Context("when Fetcher fails", func() {
				It("returns an error and http.StatusInternalServerError", func() {
					fetcher.FetchFromZipCall.Returns.AppPath = ""
					fetcher.FetchFromZipCall.Returns.Error = errors.New("fetcher error")

					reqChannel1 := make(chan interfaces.DeployResponse)
					go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{ZIP: true}, response, reqChannel1)
					deployResponse := <-reqChannel1

					Expect(deployResponse.Error).To(MatchError("fetcher error"))

					Expect(deployResponse.StatusCode).To(Equal(http.StatusInternalServerError))
				})
			})
		})
	})

	Describe("deploying with an unknown request type", func() {
		It("returns an http.StatusBadRequest and an error", func() {

			reqChannel1 := make(chan interfaces.DeployResponse)
			go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{}, response, reqChannel1)
			deployResponse := <-reqChannel1

			Expect(deployResponse.Error).To(MatchError(InvalidContentTypeError{}))

			Expect(deployResponse.StatusCode).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("setting the number of instances in the deployment", func() {
		Context("when a manifest with instances is provided", func() {
			It("uses the instances declared in the manifest", func() {
				deploymentInfo.Manifest = `---
applications:
- name: deployadactyl
  instances: 1337
`
				base64Manifest := base64.StdEncoding.EncodeToString([]byte(deploymentInfo.Manifest))

				requestBody = bytes.NewBufferString(fmt.Sprintf(`{
	 					"artifact_url": "%s",
	 					"manifest": "%s"
	 				}`,
					artifactURL,
					base64Manifest,
				))

				req, _ = http.NewRequest("POST", "", requestBody)

				reqChannel1 := make(chan interfaces.DeployResponse)
				go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
				deployResponse := <-reqChannel1

				Expect(deployResponse.Error).ToNot(HaveOccurred())

				Expect(blueGreener.PushCall.Received.DeploymentInfo.Instances).To(Equal(uint16(1337)))
			})
		})

		Context("when a manifest is not provided", func() {
			It("uses the instances declared in the deployadactyl config", func() {
				deployer.Config.Environments[environment] = S.Environment{Instances: 303}

				reqChannel1 := make(chan interfaces.DeployResponse)
				go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
				<-reqChannel1

				Eventually(blueGreener.PushCall.Received.DeploymentInfo.Instances).Should(Equal(uint16(303)))
			})
		})
	})

	Describe("not finding an environment in the config", func() {
		It("returns an error and an http.StatusInternalServerError", func() {

			reqChannel1 := make(chan interfaces.DeployResponse)
			go deployer.Deploy(req, "doesnt_exist", org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
			deployResponse := <-reqChannel1

			Eventually(deployResponse.Error).Should(MatchError(EnvironmentNotFoundError{"doesnt_exist"}))

			Eventually(deployResponse.StatusCode).Should(Equal(http.StatusInternalServerError))
			Eventually(response.String()).Should(ContainSubstring("environment not found: doesnt_exist"))
		})
	})

	Describe("deployment output", func() {
		It("shows the user deployment info properties", func() {

			reqChannel1 := make(chan interfaces.DeployResponse)
			go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
			deployResponse := <-reqChannel1

			Expect(deployResponse.StatusCode).To(Equal(http.StatusOK))
			Expect(response.String()).To(ContainSubstring(artifactURL))
			Expect(response.String()).To(ContainSubstring(username))
			Expect(response.String()).To(ContainSubstring(environment))
			Expect(response.String()).To(ContainSubstring(org))
			Expect(response.String()).To(ContainSubstring(space))
			Expect(response.String()).To(ContainSubstring(appName))
		})

		It("shows the user their deploy was successful", func() {

			reqChannel1 := make(chan interfaces.DeployResponse)
			go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
			deployResponse := <-reqChannel1

			Eventually(deployResponse.StatusCode).Should(Equal(http.StatusOK))
			Eventually(response.String()).Should(ContainSubstring("deploy was successful"))
		})
	})

	Describe("emitting events during a deployment", func() {
		BeforeEach(func() {
			eventManager.EmitCall.Returns.Error = nil
		})

		Context("when EventManager fails on "+C.DeployStartEvent, func() {
			It("returns an error and an http.StatusInternalServerError", func() {
				eventManager.EmitCall.Returns.Error = append(eventManager.EmitCall.Returns.Error, errors.New(C.DeployStartEvent+" error"))
				eventManager.EmitCall.Returns.Error = append(eventManager.EmitCall.Returns.Error, nil)

				reqChannel1 := make(chan interfaces.DeployResponse)
				go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
				deployResponse := <-reqChannel1

				expected := EventError{C.DeployStartEvent, bluegreen.InitializationError{errors.New(C.DeployStartEvent + " error")}}
				Expect(deployResponse.Error.Error()).To(Equal(expected.Error()))

				Expect(deployResponse.StatusCode).To(Equal(http.StatusInternalServerError))
				Expect(eventManager.EmitCall.TimesCalled).To(Equal(3), eventManagerNotEnoughCalls)
			})

			Context("when EventManager also fails on "+C.DeployFinishEvent, func() {
				It("outputs "+C.DeployFinishEvent+" error", func() {
					eventManager.EmitCall.Returns.Error = append(eventManager.EmitCall.Returns.Error, errors.New(C.DeployStartEvent+" error"))
					eventManager.EmitCall.Returns.Error = append(eventManager.EmitCall.Returns.Error, nil)
					eventManager.EmitCall.Returns.Error = append(eventManager.EmitCall.Returns.Error, errors.New(""+C.DeployFinishEvent+" error"))

					reqChannel1 := make(chan interfaces.DeployResponse)
					go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
					deployResponse := <-reqChannel1

					expectedErr := bluegreen.FinishDeployError{Err: errors.New("an error occurred in the " + C.DeployStartEvent + " event: " + C.DeployStartEvent + " error: an error occurred in the " + C.DeployFinishEvent + " event: " + C.DeployFinishEvent + " error")}
					Expect(deployResponse.Error.Error()).To(Equal(expectedErr.Error()))

					Expect(deployResponse.StatusCode).To(Equal(http.StatusInternalServerError))
					Expect(eventManager.EmitCall.TimesCalled).To(Equal(3), eventManagerNotEnoughCalls)
				})
			})
		})

		Context("when the blue greener fails", func() {
			It("returns an error and outputs "+C.DeployFailureEvent, func() {
				eventManager.EmitCall.Returns.Error = append(eventManager.EmitCall.Returns.Error, nil)
				eventManager.EmitCall.Returns.Error = append(eventManager.EmitCall.Returns.Error, nil)
				eventManager.EmitCall.Returns.Error = append(eventManager.EmitCall.Returns.Error, nil)

				expectedError := bluegreen.FinishPushError{[]error{errors.New("blue greener failed")}}
				blueGreener.PushCall.Returns.Error = expectedError

				reqChannel1 := make(chan interfaces.DeployResponse)
				go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
				deployResponse := <-reqChannel1

				Expect(deployResponse.Error).To(Equal(expectedError))

				Expect(deployResponse.StatusCode).To(Equal(http.StatusInternalServerError))
				Expect(eventManager.EmitCall.Received.Events[1].Type).To(Equal(C.DeployFailureEvent))
				Expect(eventManager.EmitCall.Received.Events[1].Error).To(Equal(expectedError))
			})

			It("passes the response string to FindErrors and writes the found errors to the output stream", func() {
				err := bluegreen.FinishPushError{[]error{errors.New("blue greener failed")}}
				blueGreener.PushCall.Returns.Error = err

				errors := make([]interfaces.LogMatchedError, 0, 0)
				errors = append(errors, error_finder.CreateLogMatchedError("an error description", []string{"error 1", "error 2", "error 3"}, "error solution", "TestCode"))
				errorFinder.FindErrorsCall.Returns.Errors = errors

				reqChannel1 := make(chan interfaces.DeployResponse)
				go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
				<-reqChannel1

				Expect(errorFinder.FindErrorsCall.Received.Response).ToNot(Equal(""))
				Expect(response.String()).To(ContainSubstring(errorFinder.FindErrorsCall.Received.Response))
				Expect(eventManager.EmitCall.Received.Events[1].Error).To(Equal(errors[0]))
				Expect(response.String()).To(ContainSubstring("an error description"))
				Expect(response.String()).To(ContainSubstring("error 1"))
				Expect(response.String()).To(ContainSubstring("error solution"))
			})

			It("returns a matched error with code", func() {
				err := bluegreen.FinishPushError{[]error{errors.New("blue greener failed")}}
				blueGreener.PushCall.Returns.Error = err

				errors := make([]interfaces.LogMatchedError, 0, 0)
				errors = append(errors, error_finder.CreateLogMatchedError("an error description", []string{"error 1", "error 2", "error 3"}, "error solution", "TestCode"))
				errorFinder.FindErrorsCall.Returns.Errors = errors

				reqChannel1 := make(chan interfaces.DeployResponse)
				go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
				deployResponse := <-reqChannel1

				Expect(errorFinder.FindErrorsCall.Received.Response).ToNot(Equal(""))
				Expect(response.String()).To(ContainSubstring(errorFinder.FindErrorsCall.Received.Response))
				Expect(eventManager.EmitCall.Received.Events[1].Error).To(Equal(errors[0]))
				Expect(response.String()).To(ContainSubstring("an error description"))
				Expect(response.String()).To(ContainSubstring("error 1"))
				Expect(response.String()).To(ContainSubstring("error solution"))
				Expect(deployResponse.Error.Error()).To(Equal("an error description"))
				Expect(deployResponse.Error.(interfaces.DeploymentError).Code()).To(Equal("TestCode"))

			})
		})

		Context("when blue greener succeeds", func() {
			It("does not return an error and outputs a "+C.DeploySuccessEvent+" and http.StatusOK", func() {
				eventManager.EmitCall.Returns.Error = append(eventManager.EmitCall.Returns.Error, nil)
				eventManager.EmitCall.Returns.Error = append(eventManager.EmitCall.Returns.Error, nil)
				eventManager.EmitCall.Returns.Error = append(eventManager.EmitCall.Returns.Error, nil)

				reqChannel1 := make(chan interfaces.DeployResponse)
				go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
				deployResponse := <-reqChannel1

				Expect(deployResponse.Error).To(BeNil())

				Expect(deployResponse.StatusCode).To(Equal(http.StatusOK))
				Expect(eventManager.EmitCall.Received.Events[1].Type).To(Equal(C.DeploySuccessEvent))
			})

			It("returns correct deployment info", func() {
				eventManager.EmitCall.Returns.Error = append(eventManager.EmitCall.Returns.Error, nil)
				eventManager.EmitCall.Returns.Error = append(eventManager.EmitCall.Returns.Error, nil)
				eventManager.EmitCall.Returns.Error = append(eventManager.EmitCall.Returns.Error, nil)

				reqChannel1 := make(chan interfaces.DeployResponse)
				go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
				deployResponse := <-reqChannel1

				Expect(deployResponse.DeploymentInfo.UUID).ToNot(Equal(""))
				Expect(deployResponse.DeploymentInfo.Manifest).To(ContainSubstring("manifest-"))
			})

			Context("when emitting a "+C.DeploySuccessEvent+" event fails", func() {
				It("return an error and outputs a "+C.DeploySuccessEvent+" and http.StatusOK", func() {
					eventManager.EmitCall.Returns.Error = append(eventManager.EmitCall.Returns.Error, nil)
					eventManager.EmitCall.Returns.Error = append(eventManager.EmitCall.Returns.Error, errors.New("event error"))
					eventManager.EmitCall.Returns.Error = append(eventManager.EmitCall.Returns.Error, nil)

					reqChannel1 := make(chan interfaces.DeployResponse)
					go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
					deployResponse := <-reqChannel1

					Expect(deployResponse.Error).To(BeNil())

					Expect(deployResponse.StatusCode).To(Equal(http.StatusOK))
					Expect(response.String()).To(ContainSubstring("event error"))
					Expect(eventManager.EmitCall.Received.Events[1].Type).To(Equal(C.DeploySuccessEvent))
				})
			})
		})
	})

	Describe("BlueGreener.Push", func() {
		Context("when BlueGreener fails with a login failed error", func() {
			It("returns an error and a http.StatusUnauthorized", func() {
				expectedError := bluegreen.LoginError{[]error{errors.New("login failed")}}
				blueGreener.PushCall.Returns.Error = expectedError

				reqChannel1 := make(chan interfaces.DeployResponse)
				go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
				deployResponse := <-reqChannel1

				Expect(deployResponse.Error).To(Equal(expectedError))

				Expect(deployResponse.StatusCode).To(Equal(http.StatusBadRequest))
			})
		})

		Context("when BlueGreener fails during a deploy with a zip file in the request body", func() {
			It("returns an error and a http.StatusInternalServerError", func() {
				Expect(af.WriteFile(testManifestLocation+"/manifest.yml", []byte(testManifest), 0644)).To(Succeed())

				fetcher.FetchFromZipCall.Returns.AppPath = testManifestLocation

				expectedError := bluegreen.InitializationError{errors.New("blue green error")}

				blueGreener.PushCall.Returns.Error = expectedError

				reqChannel1 := make(chan interfaces.DeployResponse)
				go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{ZIP: true}, response, reqChannel1)
				deployResponse := <-reqChannel1

				Expect(deployResponse.Error).To(Equal(expectedError))

				Expect(deployResponse.StatusCode).To(Equal(http.StatusInternalServerError))
				Expect(blueGreener.PushCall.Received.AppPath).To(Equal(testManifestLocation))
				Expect(blueGreener.PushCall.Received.DeploymentInfo.Manifest).To(Equal(fmt.Sprintf("---\napplications:\n- name: deployadactyl\n  memory: 256M\n  disk_quota: 256M\n")))
				Expect(blueGreener.PushCall.Received.DeploymentInfo.ArtifactURL).To(ContainSubstring(testManifestLocation))
			})
		})

		Context("when BlueGreener fails during a deploy with JSON in the request body", func() {
			It("returns an error and a http.StatusInternalServerError", func() {
				fetcher.FetchCall.Returns.AppPath = appPath

				blueGreener.PushCall.Returns.Error = bluegreen.InitializationError{Err: errors.New("blue green error")}

				reqChannel1 := make(chan interfaces.DeployResponse)
				go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
				deployResponse := <-reqChannel1

				Expect(deployResponse.Error).To(MatchError("blue green error"))

				Expect(deployResponse.StatusCode).To(Equal(http.StatusInternalServerError))
				Expect(blueGreener.PushCall.Received.AppPath).To(Equal(appPath))
				Expect(blueGreener.PushCall.Received.DeploymentInfo).To(Equal(deploymentInfo))
			})
		})

		Context("when BlueGreener fails during a deploy with EnableRollback set to false", func() {
			It("returns an error and a http.StatusInternalServerError", func() {
				fetcher.FetchCall.Returns.AppPath = appPath
				expectedError := bluegreen.PushError{[]error{errors.New("blue green error")}}
				blueGreener.PushCall.Returns.Error = expectedError

				reqChannel1 := make(chan interfaces.DeployResponse)
				go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
				deployResponse := <-reqChannel1

				Expect(deployResponse.Error).To(Equal(expectedError))

				Expect(deployResponse.StatusCode).To(Equal(http.StatusInternalServerError))
				Expect(blueGreener.PushCall.Received.AppPath).To(Equal(appPath))
				Expect(blueGreener.PushCall.Received.DeploymentInfo).To(Equal(deploymentInfo))
			})
		})
	})

	Describe("removing files after deploying", func() {
		It("deletes the unzipped folder from the fetcher", func() {
			af = &afero.Afero{Fs: afero.NewMemMapFs()}
			deployer = Deployer{
				c,
				blueGreener,
				fetcher,
				prechecker,
				eventManager,
				randomizerMock,
				errorFinder,
				log,
				af,
			}

			directoryName, err := af.TempDir("", "deployadactyl-")
			Expect(err).ToNot(HaveOccurred())

			fetcher.FetchCall.Returns.AppPath = directoryName

			reqChannel1 := make(chan interfaces.DeployResponse)
			go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
			<-reqChannel1

			exists, err := af.DirExists(directoryName)
			Expect(err).ToNot(HaveOccurred())

			Expect(exists).ToNot(BeTrue())
		})
	})

	Describe("happy path deploying with json in the request body", func() {
		Context("when no errors occur", func() {
			It("accepts the request and returns http.StatusOK", func() {
				fetcher.FetchCall.Returns.AppPath = appPath

				reqChannel1 := make(chan interfaces.DeployResponse)
				go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
				deployResponse := <-reqChannel1

				Expect(deployResponse.Error).To(BeNil())

				Expect(deployResponse.StatusCode).To(Equal(http.StatusOK))

				Expect(response.String()).To(ContainSubstring("Deployment Parameters"))
				Expect(response.String()).To(ContainSubstring("deploy was successful"))

				Eventually(logBuffer).Should(Say("prechecking the foundations"))
				Eventually(logBuffer).Should(Say("checking for basic auth"))
				Eventually(logBuffer).Should(Say("deploying from json request"))
				Eventually(logBuffer).Should(Say("building deploymentInfo"))
				Eventually(logBuffer).Should(Say("Deployment Parameters"))
				Eventually(logBuffer).Should(Say("emitting a " + C.DeployStartEvent + " event"))
				Eventually(logBuffer).Should(Say("emitting a " + C.DeploySuccessEvent + " event"))
				Eventually(logBuffer).Should(Say("emitting a " + C.DeployFinishEvent + " event"))

				Expect(prechecker.AssertAllFoundationsUpCall.Received.Environment).To(Equal(environments[environment]))
				Expect(fetcher.FetchCall.Received.ArtifactURL).To(Equal(artifactURL))
				Expect(fetcher.FetchCall.Received.Manifest).To(Equal(manifest))
				Expect(eventManager.EmitCall.Received.Events[0].Type).To(Equal(C.DeployStartEvent))
				Expect(eventManager.EmitCall.Received.Events[1].Type).To(Equal(C.DeploySuccessEvent))
				Expect(eventManager.EmitCall.Received.Events[2].Type).To(Equal(C.DeployFinishEvent))
				Expect(blueGreener.PushCall.Received.Environment).To(Equal(environments[environment]))
				Expect(blueGreener.PushCall.Received.AppPath).To(Equal(appPath))
				Expect(blueGreener.PushCall.Received.DeploymentInfo).To(Equal(deploymentInfo))
			})
		})
	})

	Describe("happy path deploying with a zip file in the request body", func() {
		Context("when no errors occur", func() {
			It("accepts the request and returns http.StatusOK", func() {
				Expect(af.WriteFile(testManifestLocation+"/manifest.yml", []byte(testManifest), 0644)).To(Succeed())

				fetcher.FetchFromZipCall.Returns.AppPath = testManifestLocation

				reqChannel1 := make(chan interfaces.DeployResponse)
				go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{ZIP: true}, response, reqChannel1)
				deployResponse := <-reqChannel1

				Expect(deployResponse.Error).To(BeNil())

				Expect(deployResponse.StatusCode).To(Equal(http.StatusOK))

				Expect(response.String()).To(ContainSubstring("Deployment Parameters"))
				Expect(response.String()).To(ContainSubstring("deploy was successful"))

				Eventually(logBuffer).Should(Say("prechecking the foundations"))
				Eventually(logBuffer).Should(Say("checking for basic auth"))
				Eventually(logBuffer).Should(Say("deploying from zip request"))
				Eventually(logBuffer).Should(Say("Deployment Parameters"))
				Eventually(logBuffer).Should(Say("emitting a " + C.DeployStartEvent + " event"))
				Eventually(logBuffer).Should(Say("emitting a " + C.DeploySuccessEvent + " event"))
				Eventually(logBuffer).Should(Say("emitting a " + C.DeployFinishEvent + " event"))

				Expect(prechecker.AssertAllFoundationsUpCall.Received.Environment).To(Equal(environments[environment]))
				Expect(fetcher.FetchFromZipCall.Received.Request).To(Equal(req))
				Expect(eventManager.EmitCall.Received.Events[0].Type).To(Equal(C.DeployStartEvent))
				Expect(eventManager.EmitCall.Received.Events[1].Type).To(Equal(C.DeploySuccessEvent))
				Expect(eventManager.EmitCall.Received.Events[2].Type).To(Equal(C.DeployFinishEvent))
				Expect(blueGreener.PushCall.Received.Environment).To(Equal(environments[environment]))
				Expect(blueGreener.PushCall.Received.AppPath).To(Equal(testManifestLocation))
				Expect(blueGreener.PushCall.Received.DeploymentInfo.Manifest).To(Equal(fmt.Sprintf("---\napplications:\n- name: deployadactyl\n  memory: 256M\n  disk_quota: 256M\n")))
				Expect(blueGreener.PushCall.Received.DeploymentInfo.ArtifactURL).To(ContainSubstring(testManifestLocation))
			})
		})
	})

	Describe("extract custom params from yaml", func() {
		Context("when custom params are provided", func() {
			It("should marshal params to deploymentInfo", func() {

				reqChannel1 := make(chan interfaces.DeployResponse)
				go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
				<-reqChannel1

				Expect(blueGreener.PushCall.Received.DeploymentInfo.CustomParams["service_now_column_name"].(string)).To(Equal("u_change"))
				Expect(blueGreener.PushCall.Received.DeploymentInfo.CustomParams["service_now_table_name"].(string)).To(Equal("u_table"))
			})
		})

		Context("when no custom params are provided", func() {
			BeforeEach(func() {
				environmentsNoCustomParams[environment] = S.Environment{
					Name:        environment,
					Domain:      domain,
					Foundations: foundations,
					Instances:   instances,
				}

				c := config.Config{
					Username:     username,
					Password:     password,
					Environments: environmentsNoCustomParams,
				}

				deployer = Deployer{
					c,
					blueGreener,
					fetcher,
					prechecker,
					eventManager,
					randomizerMock,
					errorFinder,
					log,
					af,
				}
			})

			It("doesn't return an error", func() {

				reqChannel1 := make(chan interfaces.DeployResponse)
				go deployer.Deploy(req, environment, org, space, appName, uuid, interfaces.DeploymentType{JSON: true}, response, reqChannel1)
				deployResponse := <-reqChannel1

				Expect(deployResponse.Error).ToNot(HaveOccurred())
				Expect(blueGreener.PushCall.Received.DeploymentInfo.CustomParams).To(BeNil())
			})
		})
	})
})
