package email

import (
	"os"

	"github.com/spf13/cobra"
)

var RootCmd = &cobra.Command{
	Use:   "pmail",
	Short: "Command line interface of PAID mail",
	Long: "A demo of using PAID mail. In production one should build " +
		"native or web applications acting as the user agent and manage" +
		"the states instead.",
}

func Execute() {
	err := RootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {

}
