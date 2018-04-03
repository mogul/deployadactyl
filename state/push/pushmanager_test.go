package push_test

import (
	"bytes"
	"encoding/base64"
	"github.com/compozed/deployadactyl/constants"
	"github.com/compozed/deployadactyl/controller/deployer"
	"github.com/compozed/deployadactyl/interfaces"
	"github.com/compozed/deployadactyl/logger"
	"github.com/compozed/deployadactyl/mocks"
	"github.com/compozed/deployadactyl/randomizer"
	. "github.com/compozed/deployadactyl/state/push"
	"github.com/compozed/deployadactyl/structs"
	"github.com/go-errors/errors"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
	"github.com/op/go-logging"
	"github.com/spf13/afero"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
)

var _ = Describe("Actioncreator", func() {
	var (
		logBuffer         *bytes.Buffer
		log               interfaces.Logger
		fetcher           *mocks.Fetcher
		eventManager      *mocks.EventManager
		pusherCreator     *PushManager
		fileSystemCleaner *mocks.FileSystemCleaner
		response          io.ReadWriter
	)
	BeforeEach(func() {
		logBuffer = bytes.NewBuffer([]byte{})
		log = logger.DefaultLogger(logBuffer, logging.DEBUG, "deployer tests")

		fetcher = &mocks.Fetcher{}
		eventManager = &mocks.EventManager{}
		fileSystemCleaner = &mocks.FileSystemCleaner{}

		response = NewBuffer()
		pusherCreator = &PushManager{
			Fetcher:      fetcher,
			Logger:       logger.DeploymentLogger{log, randomizer.StringRunes(10)},
			EventManager: eventManager,
			DeployEventData: structs.DeployEventData{
				DeploymentInfo: &structs.DeploymentInfo{},
				Response:       response,
			},
			FileSystemCleaner: fileSystemCleaner,
		}
	})
	Describe("Setup", func() {
		Context("content-type is JSON", func() {

			manifest := `---
applications:
- instances: 2`
			encodedManifest := base64.StdEncoding.EncodeToString([]byte(manifest))

			It("should extract manifest from the request", func() {
				fetcher.FetchCall.Returns.AppPath = "newAppPath"
				environment := structs.Environment{Instances: 0}

				deploymentInfo := structs.DeploymentInfo{
					Manifest:    encodedManifest,
					ContentType: "JSON",
				}
				pusherCreator.DeployEventData.DeploymentInfo = &deploymentInfo

				pusherCreator.SetUp(environment)

				Expect(pusherCreator.DeployEventData.DeploymentInfo.Manifest).To(Equal(manifest))
				logBytes, _ := ioutil.ReadAll(logBuffer)
				Eventually(string(logBytes)).Should(ContainSubstring("deploying from json request"))
			})
			It("should fetch and return app path", func() {
				fetcher.FetchCall.Returns.AppPath = "newAppPath"
				environment := structs.Environment{Instances: 0}

				deploymentInfo := structs.DeploymentInfo{
					Manifest:    encodedManifest,
					ArtifactURL: "https://artifacturl.com",
					ContentType: "JSON",
				}
				pusherCreator.DeployEventData.DeploymentInfo = &deploymentInfo

				pusherCreator.SetUp(environment)

				Expect(pusherCreator.DeployEventData.DeploymentInfo.AppPath).To(Equal("newAppPath"))
				Expect(fetcher.FetchCall.Received.ArtifactURL).To(Equal(deploymentInfo.ArtifactURL))
				Expect(fetcher.FetchCall.Received.Manifest).To(Equal(manifest))

			})
			It("should error when artifact cannot be fetched", func() {
				fetcher.FetchCall.Returns.Error = errors.New("fetch error")
				environment := structs.Environment{Instances: 0}

				deploymentInfo := structs.DeploymentInfo{
					Manifest:    encodedManifest,
					ArtifactURL: "https://artifacturl.com",
					ContentType: "JSON",
				}
				pusherCreator.DeployEventData.DeploymentInfo = &deploymentInfo

				err := pusherCreator.SetUp(environment)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("unzipped app path failed: fetch error"))
			})
			It("should retrieve instances from manifest", func() {
				fetcher.FetchCall.Returns.AppPath = "newAppPath"
				environment := structs.Environment{Instances: 0}

				deploymentInfo := structs.DeploymentInfo{
					Manifest:    encodedManifest,
					ContentType: "JSON",
				}
				pusherCreator.DeployEventData.DeploymentInfo = &deploymentInfo

				pusherCreator.SetUp(environment)

				Expect(pusherCreator.DeployEventData.DeploymentInfo.Instances).To(Equal(uint16(2)))
			})
			It("should emit artifact retrieval events", func() {
				environment := structs.Environment{Instances: 0}

				deploymentInfo := structs.DeploymentInfo{
					Manifest:    encodedManifest,
					ArtifactURL: "https://artifacturl.com",
					ContentType: "JSON",
				}

				pusherCreator.DeployEventData.DeploymentInfo = &deploymentInfo

				pusherCreator.SetUp(environment)

				Expect(eventManager.EmitCall.Received.Events[0].Type).Should(Equal(constants.ArtifactRetrievalStart))
				Expect(eventManager.EmitCall.Received.Events[1].Type).Should(Equal(constants.ArtifactRetrievalSuccess))

			})
			It("should log an artifact retrieval event", func() {
				environment := structs.Environment{Instances: 0}

				deploymentInfo := structs.DeploymentInfo{
					Manifest:    encodedManifest,
					ArtifactURL: "https://artifacturl.com",
					ContentType: "JSON",
				}
				pusherCreator.DeployEventData.DeploymentInfo = &deploymentInfo

				pusherCreator.SetUp(environment)

				logBytes, _ := ioutil.ReadAll(logBuffer)
				Eventually(string(logBytes)).Should(ContainSubstring("emitting a artifact.retrieval.start event"))
			})
			It("should return error if start emit fails", func() {
				eventManager.EmitCall.Returns.Error = []error{errors.New("error")}
				environment := structs.Environment{Instances: 0}

				deploymentInfo := structs.DeploymentInfo{
					Manifest:    encodedManifest,
					ArtifactURL: "https://artifacturl.com",
					ContentType: "JSON",
				}
				pusherCreator.DeployEventData.DeploymentInfo = &deploymentInfo

				err := pusherCreator.SetUp(environment)

				Expect(reflect.TypeOf(err)).Should(Equal(reflect.TypeOf(deployer.EventError{})))

			})
			It("should return error if emit success fails", func() {
				eventManager.EmitCall.Returns.Error = []error{nil, errors.New("error")}
				environment := structs.Environment{Instances: 0}

				deploymentInfo := structs.DeploymentInfo{
					Manifest:    encodedManifest,
					ArtifactURL: "https://artifacturl.com",
					ContentType: "JSON",
				}
				pusherCreator.DeployEventData.DeploymentInfo = &deploymentInfo

				err := pusherCreator.SetUp(environment)

				Expect(reflect.TypeOf(err)).Should(Equal(reflect.TypeOf(deployer.EventError{})))

			})
			It("should emit failure if fetch fails", func() {
				fetcher.FetchCall.Returns.Error = errors.New("a test error")
				environment := structs.Environment{Instances: 0}

				eventManager.EmitCall.Returns.Error = []error{nil, errors.New("error")}

				deploymentInfo := structs.DeploymentInfo{
					Manifest:    encodedManifest,
					ArtifactURL: "https://artifacturl.com",
					ContentType: "JSON",
				}
				pusherCreator.DeployEventData.DeploymentInfo = &deploymentInfo

				pusherCreator.SetUp(environment)

				Expect(eventManager.EmitCall.Received.Events[1].Type).Should(Equal(constants.ArtifactRetrievalFailure))
			})
		})

		Context("when instances is nil", func() {
			It("assigns environmental instances as the instance", func() {
				manifest := `---
applications:
- name: long-running-spring-app`
				encodedManifest := base64.StdEncoding.EncodeToString([]byte(manifest))
				environment := structs.Environment{Instances: 22}

				fetcher.FetchCall.Returns.AppPath = "newAppPath"

				deploymentInfo := structs.DeploymentInfo{
					Manifest:    encodedManifest,
					ArtifactURL: "https://artifacturl.com",
					ContentType: "JSON",
				}
				pusherCreator.DeployEventData.DeploymentInfo = &deploymentInfo

				pusherCreator.SetUp(environment)

				Expect(pusherCreator.DeployEventData.DeploymentInfo.Instances).To(Equal(uint16(22)))
			})
		})

		Context("contentType is ZIP", func() {

			It("should extract manifest from the zip file", func() {
				fetcher.FetchFromZipCall.Returns.AppPath = "newAppPath"
				environment := structs.Environment{Instances: 0}

				deploymentInfo := structs.DeploymentInfo{ContentType: "ZIP"}
				pusherCreator.DeployEventData.DeploymentInfo = &deploymentInfo

				pusherCreator.SetUp(environment)

				Expect(pusherCreator.DeployEventData.DeploymentInfo.AppPath).To(Equal("newAppPath"))
				logBytes, _ := ioutil.ReadAll(logBuffer)
				Eventually(string(logBytes)).Should(ContainSubstring("deploying from zip request"))
			})
			It("should error when artifact cannot be fetched", func() {
				fetcher.FetchFromZipCall.Returns.Error = errors.New("a test error")
				environment := structs.Environment{Instances: 0}

				deploymentInfo := structs.DeploymentInfo{
					ContentType: "ZIP",
				}
				pusherCreator.DeployEventData.DeploymentInfo = &deploymentInfo

				err := pusherCreator.SetUp(environment)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("unzipping request body error: a test error"))
			})
		})

	})

	Describe("OnStart", func() {
		It("emits a push started event", func() {
			pusherCreator.OnStart()

			Expect(eventManager.EmitCall.Received.Events[0].Type).Should(Equal(constants.PushStartedEvent))
		})
		It("logs the parameters", func() {
			deployInfo := pusherCreator.DeployEventData.DeploymentInfo
			deployInfo.ArtifactURL = randomizer.StringRunes(10)
			deployInfo.Username = randomizer.StringRunes(10)
			deployInfo.Environment = randomizer.StringRunes(10)
			deployInfo.Org = randomizer.StringRunes(10)
			deployInfo.Space = randomizer.StringRunes(10)
			deployInfo.AppName = randomizer.StringRunes(10)

			pusherCreator.OnStart()

			logBytes, _ := ioutil.ReadAll(logBuffer)
			Eventually(string(logBytes)).Should(ContainSubstring("Artifact URL: " + pusherCreator.DeployEventData.DeploymentInfo.ArtifactURL))
			Eventually(string(logBytes)).Should(ContainSubstring("Username:     " + pusherCreator.DeployEventData.DeploymentInfo.Username))
			Eventually(string(logBytes)).Should(ContainSubstring("Environment:  " + pusherCreator.DeployEventData.DeploymentInfo.Environment))
			Eventually(string(logBytes)).Should(ContainSubstring("Org:          " + pusherCreator.DeployEventData.DeploymentInfo.Org))
			Eventually(string(logBytes)).Should(ContainSubstring("Space:        " + pusherCreator.DeployEventData.DeploymentInfo.Space))
			Eventually(string(logBytes)).Should(ContainSubstring("AppName:      " + pusherCreator.DeployEventData.DeploymentInfo.AppName))
		})
		It("prints the parameters to the response", func() {
			deployInfo := pusherCreator.DeployEventData.DeploymentInfo
			deployInfo.ArtifactURL = randomizer.StringRunes(10)
			deployInfo.Username = randomizer.StringRunes(10)
			deployInfo.Environment = randomizer.StringRunes(10)
			deployInfo.Org = randomizer.StringRunes(10)
			deployInfo.Space = randomizer.StringRunes(10)
			deployInfo.AppName = randomizer.StringRunes(10)

			pusherCreator.OnStart()

			Eventually(response).Should(Say("Artifact URL: " + pusherCreator.DeployEventData.DeploymentInfo.ArtifactURL))
			Eventually(response).Should(Say("Username:     " + pusherCreator.DeployEventData.DeploymentInfo.Username))
			Eventually(response).Should(Say("Environment:  " + pusherCreator.DeployEventData.DeploymentInfo.Environment))
			Eventually(response).Should(Say("Org:          " + pusherCreator.DeployEventData.DeploymentInfo.Org))
			Eventually(response).Should(Say("Space:        " + pusherCreator.DeployEventData.DeploymentInfo.Space))
			Eventually(response).Should(Say("AppName:      " + pusherCreator.DeployEventData.DeploymentInfo.AppName))
		})
		Context("if push started event fails", func() {
			It("returns an error", func() {
				eventManager.EmitCall.Returns.Error = []error{errors.New("a test error")}

				err := pusherCreator.OnStart()

				Expect(reflect.TypeOf(err)).Should(Equal(reflect.TypeOf(deployer.EventError{})))
			})
			It("logs the error", func() {
				eventManager.EmitCall.Returns.Error = []error{errors.New("a test error")}

				pusherCreator.OnStart()

				logBytes, _ := ioutil.ReadAll(logBuffer)
				Eventually(string(logBytes)).Should(ContainSubstring("a test error"))
			})
		})
	})

	Describe("CleanUp", func() {
		It("deletes all temp artifacts", func() {
			path := randomizer.StringRunes(10)
			pusherCreator.DeployEventData.DeploymentInfo.AppPath = path

			pusherCreator.CleanUp()

			Expect(fileSystemCleaner.RemoveAllCall.Received.Path).To(Equal(path))
		})
		It("really deletes all temp artifacts", func() {
			af := &afero.Afero{Fs: afero.NewMemMapFs()}
			pusherCreator.FileSystemCleaner = af

			directoryName, _ := af.TempDir("", "deployadactyl-")

			pusherCreator.CleanUp()

			exists, err := af.DirExists(directoryName)
			Expect(err).ToNot(HaveOccurred())

			Expect(exists).ToNot(BeTrue())
		})
	})

	Describe("OnFinish", func() {
		Context("when error occurs", func() {
			Context("and EnableRollback is false", func() {
				It("returns StatusOK", func() {
					env := structs.Environment{EnableRollback: false}
					err := errors.New("a test error")

					resp := pusherCreator.OnFinish(env, response, err)

					Expect(resp.StatusCode).To(Equal(http.StatusOK))
				})
				It("logs the failure", func() {
					env := structs.Environment{EnableRollback: false}
					err := errors.New("a test error")

					pusherCreator.OnFinish(env, response, err)

					logBytes, _ := ioutil.ReadAll(logBuffer)
					Eventually(string(logBytes)).Should(ContainSubstring("EnableRollback false, returning status"))
				})
			})
			Context("and EnableRollback is true", func() {
				Context("and error is a login failure", func() {
					It("returns StatusBadRequest", func() {
						env := structs.Environment{EnableRollback: true}
						err := errors.New("the login failed")

						resp := pusherCreator.OnFinish(env, response, err)

						Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
					})
				})
				It("returns StatusInternalServerError", func() {
					env := structs.Environment{EnableRollback: true}
					err := errors.New("a test error")

					resp := pusherCreator.OnFinish(env, response, err)

					Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
				})
			})
		})
		Context("when no error occurs", func() {
			It("returns StatusOK", func() {
				env := structs.Environment{EnableRollback: true}

				resp := pusherCreator.OnFinish(env, response, nil)

				Expect(resp.StatusCode).To(Equal(http.StatusOK))
			})
			It("logs a successful deployment message", func() {
				env := structs.Environment{EnableRollback: true}

				pusherCreator.OnFinish(env, response, nil)
				logBytes, _ := ioutil.ReadAll(logBuffer)
				Eventually(string(logBytes)).Should(ContainSubstring("successfully deployed application"))
			})
			It("writes success to the output", func() {
				env := structs.Environment{EnableRollback: true}

				pusherCreator.OnFinish(env, response, nil)
				logBytes, _ := ioutil.ReadAll(response)
				Eventually(string(logBytes)).Should(ContainSubstring("Your deploy was successful!"))
			})
		})
	})
})