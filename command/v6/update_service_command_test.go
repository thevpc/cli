package v6_test

import (
	"errors"
	"fmt"

	"code.cloudfoundry.org/cli/actor/v2action"
	"code.cloudfoundry.org/cli/command/commandfakes"
	"code.cloudfoundry.org/cli/command/flag"
	"code.cloudfoundry.org/cli/command/translatableerror"
	. "code.cloudfoundry.org/cli/command/v6"
	"code.cloudfoundry.org/cli/command/v6/v6fakes"
	"code.cloudfoundry.org/cli/util/configv3"
	"code.cloudfoundry.org/cli/util/ui"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
)

var _ = Describe("update-service Command", func() {

	const (
		serviceInstanceName = "my-service"
		spaceGUID           = "space-guid"
		instanceGUID        = "instance-guid"
		planGUID            = "plan-guid"
	)

	var (
		cmd             UpdateServiceCommand
		fakeActor       *v6fakes.FakeUpdateServiceActor
		fakeSharedActor *commandfakes.FakeSharedActor
		fakeConfig      *commandfakes.FakeConfig
		testUI          *ui.UI
		input           *Buffer
		executeErr      error
		extraArgs       []string

		space = configv3.Space{Name: "space-a", GUID: spaceGUID}
	)

	BeforeEach(func() {
		input = NewBuffer()
		testUI = ui.NewTestUI(input, NewBuffer(), NewBuffer())
		fakeActor = new(v6fakes.FakeUpdateServiceActor)
		fakeSharedActor = new(commandfakes.FakeSharedActor)
		fakeConfig = new(commandfakes.FakeConfig)

		fakeConfig.TargetedSpaceReturns(space)

		extraArgs = []string{}

		cmd = UpdateServiceCommand{
			UI:           testUI,
			Actor:        fakeActor,
			SharedActor:  fakeSharedActor,
			Config:       fakeConfig,
			RequiredArgs: flag.ServiceInstance{ServiceInstance: serviceInstanceName},
		}
	})

	JustBeforeEach(func() {
		executeErr = cmd.Execute(extraArgs)
	})

	When("not upgrading", func() {
		It("returns UnrefactoredCommandError", func() {
			// delegates non-upgrades to legacy code
			Expect(executeErr).To(MatchError(translatableerror.UnrefactoredCommandError{}))
		})
	})

	When("combining upgrade with other flags", func() {
		BeforeEach(func() {
			cmd.Upgrade = true
		})

		When("tags provided", func() {
			BeforeEach(func() {
				cmd.Tags = "tags"
			})

			It("returns UpgradeArgumentCombinationError", func() {
				Expect(executeErr).To(MatchError(translatableerror.ArgumentCombinationError{
					Args: []string{"--upgrade", "-t", "-c", "-p"},
				}))
			})
		})

		When("parameters provided", func() {
			BeforeEach(func() {
				cmd.ParametersAsJSON = "{\"some\": \"stuff\"}"
			})

			It("returns UpgradeArgumentCombinationError", func() {
				Expect(executeErr).To(MatchError(translatableerror.ArgumentCombinationError{
					Args: []string{"--upgrade", "-t", "-c", "-p"},
				}))
			})
		})

		When("plan provided", func() {
			BeforeEach(func() {
				cmd.Plan = "new-plan"
			})

			It("returns UpgradeArgumentCombinationError", func() {
				Expect(executeErr).To(MatchError(translatableerror.ArgumentCombinationError{
					Args: []string{"--upgrade", "-t", "-c", "-p"},
				}))
			})
		})
	})

	When("upgrading", func() {
		BeforeEach(func() {
			cmd.Upgrade = true
		})

		It("checks the user is logged in, and targeting an org and space", func() {
			Expect(fakeSharedActor.CheckTargetCallCount()).To(Equal(1))
			orgChecked, spaceChecked := fakeSharedActor.CheckTargetArgsForCall(0)
			Expect(orgChecked).To(BeTrue())
			Expect(spaceChecked).To(BeTrue())
		})

		When("checking the target succeeds", func() {
			When("getting the service instance succeeds", func() {
				BeforeEach(func() {
					fakeActor.GetServiceInstanceByNameAndSpaceReturns(
						v2action.ServiceInstance{GUID: instanceGUID, ServicePlanGUID: planGUID},
						v2action.Warnings{"warning"},
						nil)
				})

				It("displays any warnings", func() {
					Expect(testUI.Err).To(Say("warning"))
				})

				It("mentions that the command is experimental", func() {
					Expect(testUI.Out).To(Say("This command is in EXPERIMENTAL stage and may change without notice\\."))
				})

				It("prompts the user about the upgrade", func() {
					Expect(testUI.Out).To(Say("You are about to update %s\\.", serviceInstanceName))
					Expect(testUI.Out).To(Say("Warning: This operation may be long running and will block further operations on the service until complete\\."))
					Expect(testUI.Out).To(Say("Really update service %s\\? \\[yN\\]:", serviceInstanceName))
				})

				When("user refuses to proceed with the upgrade", func() {
					BeforeEach(func() {
						input.Write([]byte("n\n"))
					})

					It("does not send an upgrade request", func() {
						Expect(fakeActor.UpgradeServiceInstanceCallCount()).To(Equal(0))
					})

					It("cancels the update", func() {
						Expect(executeErr).NotTo(HaveOccurred())
						Expect(testUI.Out).To(Say("Update cancelled"))
					})
				})

				When("user goes ahead with the upgrade", func() {
					BeforeEach(func() {
						input.Write([]byte("y\n"))
					})

					It("sends an upgrade request", func() {
						Expect(fakeActor.UpgradeServiceInstanceCallCount()).To(Equal(1), "upgrade should be requested")

						serviceInstanceGUID, servicePlanGUID := fakeActor.UpgradeServiceInstanceArgsForCall(0)
						Expect(serviceInstanceGUID).To(Equal(instanceGUID))
						Expect(servicePlanGUID).To(Equal(planGUID))
					})

					When("the update request succeeds", func() {
						It("says that the update was successful", func() {
							Expect(executeErr).NotTo(HaveOccurred())
							Expect(testUI.Out).To(Say("OK"))
						})
					})

					When("the update request fails", func() {
						BeforeEach(func() {
							fakeActor.UpgradeServiceInstanceReturns(
								v2action.Warnings{},
								fmt.Errorf("bad things happened"),
							)
						})

						It("says that the update has failed", func() {
							Expect(executeErr).To(MatchError("bad things happened"))
						})
					})

					When("there are warnings", func() {
						BeforeEach(func() {
							fakeActor.UpgradeServiceInstanceReturns(
								v2action.Warnings{"fake upgrade warning 1", "fake upgrade warning 2"},
								nil,
							)
						})

						It("outputs the warnings", func() {
							Expect(testUI.Err).To(Say("fake upgrade warning 1"))
							Expect(testUI.Err).To(Say("fake upgrade warning 2"))
						})

						It("can still output OK", func() {
							Expect(testUI.Out).To(Say("OK"))
						})
					})
				})

				When("user presses return", func() {
					BeforeEach(func() {
						input.Write([]byte("\n"))
					})

					It("cancels the update", func() {
						Expect(testUI.Out).To(Say("Update cancelled"))
						Expect(executeErr).NotTo(HaveOccurred())
					})
				})

				When("user does not answer", func() {
					It("fails", func() {
						Expect(executeErr).To(MatchError("EOF"))
					})
				})
			})

			When("getting the service instance fails", func() {
				BeforeEach(func() {
					fakeActor.GetServiceInstanceByNameAndSpaceReturns(v2action.ServiceInstance{}, v2action.Warnings{"warning"}, errors.New("explode"))
				})

				It("propagates the error", func() {
					Expect(executeErr).To(MatchError("explode"))
				})

				It("displays any warnings", func() {
					Expect(testUI.Err).To(Say("warning"))
				})
			})
		})

		When("too many arguments are provided", func() {
			BeforeEach(func() {
				extraArgs = []string{"extra"}
			})

			It("returns a TooManyArgumentsError", func() {
				Expect(executeErr).To(MatchError(translatableerror.TooManyArgumentsError{
					ExtraArgument: "extra",
				}))
			})
		})

		When("checking the target returns an error", func() {
			BeforeEach(func() {
				fakeSharedActor.CheckTargetReturns(errors.New("explode"))
			})

			It("returns an error", func() {
				Expect(executeErr).To(MatchError("explode"))
			})
		})
	})
})
