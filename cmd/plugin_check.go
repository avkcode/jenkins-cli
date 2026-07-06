package cmd

import (
	"context"
)

func isFirecrackerPluginInstalled(ctx context.Context) (bool, error) {
	client, err := getClient(ctx)
	if err != nil {
		return false, err
	}
	plugins, err := client.Client.GetPlugins(ctx, 1)
	if err != nil {
		return false, err
	}
	for _, p := range plugins.Raw.Plugins {
		if p.ShortName == "firecracker-cloud" || p.ShortName == "aero-compute-engine" {
			return true, nil
		}
	}
	return false, nil
}
