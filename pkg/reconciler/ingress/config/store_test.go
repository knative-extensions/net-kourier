/*
Copyright 2021 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"knative.dev/net-kourier/pkg/config"
	network "knative.dev/networking/pkg"
	logtesting "knative.dev/pkg/logging/testing"

	. "knative.dev/pkg/configmap/testing"
)

func TestStoreLoadWithContext(t *testing.T) {
	store := NewStore(logtesting.TestLogger(t))
	kourierConfig := ConfigMapFromTestFile(t, config.ConfigName)
	networkConfig := ConfigMapFromTestFile(t, network.ConfigName)
	store.OnConfigChanged(kourierConfig)
	store.OnConfigChanged(networkConfig)

	cfg := FromContext(store.ToContext(context.Background()))

	expected, _ := config.NewConfigFromConfigMap(kourierConfig)
	if diff := cmp.Diff(expected, cfg.Kourier); diff != "" {
		t.Errorf("Unexpected defaults config (-want, +got):\n%v", diff)
	}
}

func TestStoreLoadWithDefaults(t *testing.T) {
	cfg := FromContextOrDefaults(context.Background())

	if diff := cmp.Diff(config.DefaultConfig(), cfg.Kourier); diff != "" {
		t.Errorf("Unexpected defaults config (-want, +got):\n%v", diff)
	}
	if diff := cmp.Diff(defaultConfig(), cfg.Network); diff != "" {
		t.Errorf("Unexpected defaults config (-want, +got):\n%v", diff)
	}
}

func TestStoreImmutableConfig(t *testing.T) {
	store := NewStore(logtesting.TestLogger(t))
	store.OnConfigChanged(ConfigMapFromTestFile(t, config.ConfigName))
	store.OnConfigChanged(ConfigMapFromTestFile(t, network.ConfigName))
	config := store.Load()

	config.Kourier.EnableServiceAccessLogging = false

	newConfig := store.Load()
	if newConfig.Kourier.EnableServiceAccessLogging == false {
		t.Error("Kourier config is not immutable")
	}
}
