package main

// InstallOptions defines the configuration for environment deployment.
type InstallOptions struct {
	UseNightly bool   `json:"useNightly"`
	HFToken    string `json:"hfToken"`
}

// LaunchOptions defines the parameters for engine initialization.
type LaunchOptions struct {
	ModelID  string `json:"modelId"`
	TaskType string `json:"taskType"`
}
