package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "panago",
	Short: "Panago is a tool for device discovery and management",
}

func Execute() error {
	return rootCmd.Execute()
}
