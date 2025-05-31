package model

import modelprovider "github.com/viant/agently/genai/llm/provider"

// Option defines a functional option for Finder
type Option func(dao *Finder)

// WithConfigLoader sets a custom configuration loader for the Finder instance.
func WithConfigLoader(loader modelprovider.ConfigLoader) Option {
	return func(dao *Finder) {
		dao.configLoader = loader
	}
}

// WithInitial adds model configurations to the Finder instance.
func WithInitial(configs ...*modelprovider.Config) Option {
	return func(dao *Finder) {
		for _, modelConfig := range configs {
			dao.configRegistry.Add(modelConfig.ID, modelConfig)
		}
	}
}
