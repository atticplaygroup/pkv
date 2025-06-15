package email

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to read private messages",
	Long:  "",
	Run:   login,
}

type AccountConfig struct {
	AuthToken string `yaml:"auth_token"`
}

type Config struct {
	Account AccountConfig `yaml:"account"`
}

func getDefaultConfigPath() string {
	defaultConfigPath, ok := os.LookupEnv("PMAIL_CONFIG_PATH")
	if ok {
		return defaultConfigPath
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		defaultConfigPath = ""
	} else {
		defaultConfigPath = path.Join(homeDir, ".prex", "pmail-config.yaml")
	}
	return defaultConfigPath
}

func init() {
	flags := loginCmd.PersistentFlags()
	flags.StringP("config", "c", getDefaultConfigPath(), "Config yaml path")
	RootCmd.AddCommand(loginCmd)
}

func persistToYaml(configPath string, authToken string) error {
	parentDir := filepath.Dir(configPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("cannot create default config file: %v", err)
	}
	defaultConfig := &Config{
		Account: AccountConfig{AuthToken: authToken},
	}
	yamlData, err := yaml.Marshal(defaultConfig)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, yamlData, 0644)
}

func login(cmd *cobra.Command, args []string) {
	configPath, err := cmd.Flags().GetString("config")
	if err != nil {
		log.Fatal("no config file provided")
	}

	loginUrl := "http://localhost:8080/v1/login"
	fmt.Printf("Please open %s and after login paste the access token here: ", loginUrl)

	var authToken string
	fmt.Scanln(&authToken)
	persistToYaml(configPath, authToken)
}
