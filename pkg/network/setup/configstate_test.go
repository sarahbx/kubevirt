/*
 * This file is part of the KubeVirt project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright 2023 Red Hat, Inc.
 *
 */

package network

import (
	"errors"
	"fmt"

	v1 "kubevirt.io/api/core/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"kubevirt.io/kubevirt/pkg/network/cache"
	neterrors "kubevirt.io/kubevirt/pkg/network/errors"
)

const (
	uid = "123"

	testNet0 = "testnet0"
	testNet1 = "testnet1"
	testNet2 = "testnet2"
)

var _ = Describe("config state", func() {
	var (
		configState      ConfigState
		configStateCache configStateCacheStub
		nics             []podNIC
		ns               nsExecutorStub
	)

	Context("Run", func() {
		BeforeEach(func() {
			configStateCache = newConfigStateCacheStub()
			ns = nsExecutorStub{}
			configState = NewConfigState(&configStateCache, ns)
			nics = []podNIC{{
				vmiSpecNetwork: &v1.Network{Name: testNet0},
			}}
		})

		It("runs with no current state (cache is empty)", func() {
			discover, config := &plugFuncStub{}, &plugFuncStub{}

			Expect(configState.Run(nics, noopCSPreRun, discover.f, config.f)).To(Succeed())

			Expect(discover.executedNetworks).To(Equal([]string{testNet0}), "the discover step should execute")
			Expect(config.executedNetworks).To(Equal([]string{testNet0}), "the config step should execute")

			state, err := configStateCache.Read(testNet0)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(Equal(cache.PodIfaceNetworkPreparationFinished))
		})

		It("runs with current pending state", func() {
			Expect(configStateCache.Write(testNet0, cache.PodIfaceNetworkPreparationPending)).To(Succeed())
			discover, config := &plugFuncStub{}, &plugFuncStub{}

			Expect(configState.Run(nics, noopCSPreRun, discover.f, config.f)).To(Succeed())

			Expect(discover.executedNetworks).To(Equal([]string{testNet0}), "the discover step should execute")
			Expect(config.executedNetworks).To(Equal([]string{testNet0}), "the config step should execute")

			state, err := configStateCache.Read(testNet0)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(Equal(cache.PodIfaceNetworkPreparationFinished))
		})

		It("runs with current started state", func() {
			Expect(configStateCache.Write(testNet0, cache.PodIfaceNetworkPreparationStarted)).To(Succeed())
			discover, config := &plugFuncStub{}, &plugFuncStub{}

			ns.shouldNotBeExecuted = true
			err := configState.Run(nics, noopCSPreRun, discover.f, config.f)
			Expect(err).To(HaveOccurred())
			var criticalNetErr *neterrors.CriticalNetworkError
			Expect(errors.As(err, &criticalNetErr)).To(BeTrue())

			Expect(discover.executedNetworks).To(BeEmpty(), "the discover step should not be execute")
			Expect(config.executedNetworks).To(BeEmpty(), "the config step should not be execute")

			state, err := configStateCache.Read(testNet0)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(Equal(cache.PodIfaceNetworkPreparationStarted))
		})

		It("runs with current finished state", func() {
			Expect(configStateCache.Write(testNet0, cache.PodIfaceNetworkPreparationFinished)).To(Succeed())
			discover, config := &plugFuncStub{}, &plugFuncStub{}

			ns.shouldNotBeExecuted = true
			Expect(configState.Run(nics, noopCSPreRun, discover.f, config.f)).To(Succeed())

			Expect(discover.executedNetworks).To(BeEmpty(), "the discover step should not be execute")
			Expect(config.executedNetworks).To(BeEmpty(), "the config step should not be execute")

			state, err := configStateCache.Read(testNet0)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(Equal(cache.PodIfaceNetworkPreparationFinished))
		})

		It("runs and fails at the discover step", func() {
			injectedErr := fmt.Errorf("fail discovery")
			discover, config := &plugFuncStub{errRun: injectedErr}, &plugFuncStub{}

			Expect(configState.Run(nics, noopCSPreRun, discover.f, config.f)).To(MatchError(injectedErr))

			Expect(discover.executedNetworks).To(Equal([]string{testNet0}), "the discover step should execute")
			Expect(config.executedNetworks).To(BeEmpty(), "the config step should not execute")

			state, err := configStateCache.Read(testNet0)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(Equal(cache.PodIfaceNetworkPreparationPending))
		})

		It("runs and fails at the config step", func() {
			injectedErr := fmt.Errorf("fail config")
			discover, config := &plugFuncStub{}, &plugFuncStub{errRun: injectedErr}

			Expect(configState.Run(nics, noopCSPreRun, discover.f, config.f)).To(MatchError(injectedErr))

			Expect(discover.executedNetworks).To(Equal([]string{testNet0}), "the discover step should execute")
			Expect(config.executedNetworks).To(Equal([]string{testNet0}), "the config step should execute")

			state, err := configStateCache.Read(testNet0)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(Equal(cache.PodIfaceNetworkPreparationStarted))
		})

		It("runs and fails reading the cache", func() {
			injectedErr := fmt.Errorf("fail read cache")
			configStateCache.readErr = injectedErr
			configState = NewConfigState(&configStateCache, ns)

			discover, config := &plugFuncStub{}, &plugFuncStub{}

			ns.shouldNotBeExecuted = true
			Expect(configState.Run(nics, noopCSPreRun, discover.f, config.f)).To(MatchError(injectedErr))

			Expect(discover.executedNetworks).To(BeEmpty(), "the discover step shouldn't execute")
			Expect(config.executedNetworks).To(BeEmpty(), "the config step shouldn't execute")
		})

		It("runs and fails writing the cache", func() {
			injectedErr := fmt.Errorf("fail write cache")
			configStateCache.writeErr = injectedErr
			configState = NewConfigState(&configStateCache, ns)

			discover, config := &plugFuncStub{}, &plugFuncStub{}

			Expect(configState.Run(nics, noopCSPreRun, discover.f, config.f)).To(MatchError(injectedErr))

			Expect(discover.executedNetworks).To(Equal([]string{testNet0}), "the discover step should execute")
			Expect(config.executedNetworks).To(BeEmpty(), "the config step shouldn't execute")
		})

		When("with multiple interfaces", func() {
			BeforeEach(func() {
				nics = append(nics,
					podNIC{vmiSpecNetwork: &v1.Network{Name: testNet1}},
					podNIC{vmiSpecNetwork: &v1.Network{Name: testNet2}},
				)
			})

			It("runs with no current state (cache is empty)", func() {
				discover, config := &plugFuncStub{}, &plugFuncStub{}

				Expect(configState.Run(nics, noopCSPreRun, discover.f, config.f)).To(Succeed())

				Expect(discover.executedNetworks).To(Equal([]string{testNet0, testNet1, testNet2}))
				Expect(config.executedNetworks).To(Equal([]string{testNet0, testNet1, testNet2}))

				for _, testNet := range []string{testNet0, testNet1, testNet2} {
					state, err := configStateCache.Read(testNet)
					Expect(err).NotTo(HaveOccurred())
					Expect(state).To(Equal(cache.PodIfaceNetworkPreparationFinished))
				}
			})

			It("runs and fails at the config step, 2nd network", func() {
				injectedErr := fmt.Errorf("fail config")
				discover, config := &plugFuncStub{}, &plugFuncStub{errRun: injectedErr, errRunForPodIfaceName: testNet1}

				Expect(configState.Run(nics, noopCSPreRun, discover.f, config.f)).To(MatchError(injectedErr))

				Expect(discover.executedNetworks).To(Equal([]string{testNet0, testNet1, testNet2}))
				Expect(config.executedNetworks).To(Equal([]string{testNet0, testNet1}))

				for _, testNet := range []string{testNet0, testNet1, testNet2} {
					state, err := configStateCache.Read(testNet)
					Expect(err).NotTo(HaveOccurred())
					Expect(state).To(Equal(cache.PodIfaceNetworkPreparationStarted))
				}
			})

			It("runs with filter that removes all networks", func() {
				discover, config := &plugFuncStub{}, &plugFuncStub{}
				preRunFilterOutAll := func(_ []podNIC) ([]podNIC, error) {
					return nil, nil
				}
				Expect(configState.Run(nics, preRunFilterOutAll, discover.f, config.f)).To(Succeed())

				Expect(discover.executedNetworks).To(BeEmpty())
				Expect(config.executedNetworks).To(BeEmpty())
			})
		})
	})

	Context("UnplugNetworks", func() {
		var specInterfaces []v1.Interface

		BeforeEach(func() {
			configStateCache = newConfigStateCacheStub()
			ns = nsExecutorStub{}
			configState = NewConfigState(&configStateCache, ns)
			specInterfaces = []v1.Interface{{Name: testNet0}, {Name: testNet1}, {Name: testNet2}}
			Expect(configStateCache.Write(testNet0, cache.PodIfaceNetworkPreparationFinished)).To(Succeed())
			Expect(configStateCache.Write(testNet1, cache.PodIfaceNetworkPreparationStarted)).To(Succeed())
			Expect(configStateCache.Write(testNet2, cache.PodIfaceNetworkPreparationFinished)).To(Succeed())
		})
		It("There are no networks to unplug", func() {
			ns.shouldNotBeExecuted = true
			unplugFunc := &unplugFuncStub{}
			err := configState.UnplugNetworks(specInterfaces, unplugFunc.f)
			Expect(err).NotTo(HaveOccurred())
			Expect(unplugFunc.executedNetworks).To(BeEmpty(), "the unplug step shouldn't execute")
		})
		It("There is one network to unplug", func() {
			specInterfaces[0].State = v1.InterfaceStateAbsent

			ns.shouldNotBeExecuted = false
			unplugFunc := &unplugFuncStub{}
			err := configState.UnplugNetworks(specInterfaces, unplugFunc.f)
			Expect(err).NotTo(HaveOccurred())
			Expect(unplugFunc.executedNetworks).To(ConsistOf([]string{testNet0}))
		})
		It("There are multiple networks to unplug", func() {
			specInterfaces[0].State = v1.InterfaceStateAbsent
			specInterfaces[1].State = v1.InterfaceStateAbsent

			ns.shouldNotBeExecuted = false
			unplugFunc := &unplugFuncStub{}
			err := configState.UnplugNetworks(specInterfaces, unplugFunc.f)
			Expect(err).NotTo(HaveOccurred())
			Expect(unplugFunc.executedNetworks).To(ConsistOf([]string{testNet0, testNet1}))
		})
		It("There are multiple networks to unplug and some have errors on cleanup", func() {
			specInterfaces[0].State = v1.InterfaceStateAbsent
			specInterfaces[1].State = v1.InterfaceStateAbsent
			specInterfaces[2].State = v1.InterfaceStateAbsent

			ns.shouldNotBeExecuted = false
			injectedErr := fmt.Errorf("fails unplug")
			injectedErr3 := fmt.Errorf("fails unplug3")
			unplugFunc := &unplugFuncStub{errRunForPodIfaces: map[string]error{testNet0: injectedErr, testNet2: injectedErr3}}
			err := configState.UnplugNetworks(specInterfaces, unplugFunc.f)
			Expect(err.Error()).To(ContainSubstring(injectedErr.Error()))
			Expect(err.Error()).To(ContainSubstring(injectedErr3.Error()))
			Expect(unplugFunc.executedNetworks).To(ConsistOf([]string{testNet0, testNet1, testNet2}))
		})
	})
})

type plugFuncStub struct {
	executedNetworks      []string
	errRun                error
	errRunForPodIfaceName string
}

func (f *plugFuncStub) f(nic *podNIC) error {
	f.executedNetworks = append(f.executedNetworks, nic.vmiSpecNetwork.Name)

	// If an error is specified, return it if there is no filter at all, or if the filter is specified and matches.
	// The filter is the network name.
	var err error
	if f.errRunForPodIfaceName == "" || f.errRunForPodIfaceName == nic.vmiSpecNetwork.Name {
		err = f.errRun
	}
	return err
}

type unplugFuncStub struct {
	executedNetworks   []string
	errRunForPodIfaces map[string]error
}

func (f *unplugFuncStub) f(name string) error {
	f.executedNetworks = append(f.executedNetworks, name)
	return f.errRunForPodIfaces[name]
}

func noopCSPreRun(nics []podNIC) ([]podNIC, error) {
	return nics, nil
}
