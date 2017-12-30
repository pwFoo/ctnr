// Copyright © 2017 Max Goltzsche
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"fmt"

	"github.com/mgoltzsche/cntnr/log"
	"github.com/spf13/cobra"
	//homedir "github.com/mitchellh/go-homedir"
	//"github.com/spf13/viper"
	"os"
	"os/user"
	"path/filepath"

	"github.com/containers/image/types"
	istore "github.com/mgoltzsche/cntnr/oci/image/store"
	storepkg "github.com/mgoltzsche/cntnr/oci/store"
)

var (
	flagVerbose     bool
	flagRootless    bool
	flagCfgFile     string
	flagStoreDir    string
	flagStateDir    string
	flagImagePolicy string

	store    storepkg.Store
	errorLog = log.NewStdLogger(os.Stderr)
	warnLog  = log.NewStdLogger(os.Stderr)
	debugLog = log.NewNopLogger()
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "cntnr",
	Short: "a lightweight container engine",
	Long: `cntnr is a lightweight OCI-compliant container engine.
It supports single image and container operations as well as high-level service composition.`,
	PersistentPreRun: preRun,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	RootCmd.AddCommand(runCmd)
	RootCmd.AddCommand(killCmd)
	RootCmd.AddCommand(listCmd)
	RootCmd.AddCommand(imageCmd)
	RootCmd.AddCommand(bundleCmd)
	RootCmd.AddCommand(composeCmd)
	RootCmd.AddCommand(netCmd)
	RootCmd.AddCommand(commitCmd)
	RootCmd.AddCommand(gcCmd)
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	//cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	//RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.cntnr.yaml)")

	currUser, err := user.Current()
	if err != nil {
		exitError(2, "cannot get current user: %s", err)
	}
	flagStoreDir = filepath.Join(currUser.HomeDir, ".cntnr")
	flagStateDir = "/run/cntnr"
	if currUser.Uid != "0" {
		flagStateDir = "/run/user/" + currUser.Uid + "/cntnr"
	}
	flagImagePolicy = "insecure"
	policyFile := "/etc/containers/policy.json"
	if _, err = os.Stat(policyFile); err == nil {
		flagImagePolicy = policyFile
	}
	f := RootCmd.PersistentFlags()
	f.BoolVar(&flagVerbose, "verbose", false, "enables verbose log output")
	f.BoolVar(&flagRootless, "rootless", currUser.Uid != "0", "enables image and container management as unprivileged user")
	f.StringVar(&flagStoreDir, "store-dir", flagStoreDir, "directory to store images and containers")
	f.StringVar(&flagStateDir, "state-dir", flagStateDir, "directory to store OCI container states (should be tmpfs)")
	f.StringVar(&flagImagePolicy, "image-policy", flagImagePolicy, "image trust policy configuration file or 'insecure'")
}

func preRun(cmd *cobra.Command, args []string) {
	if flagVerbose {
		debugLog = log.NewStdLogger(os.Stderr)
	}

	// init store
	// TODO: provide CLI options
	ctx := &types.SystemContext{
		RegistriesDirPath:           "",
		DockerCertPath:              "",
		DockerInsecureSkipTLSVerify: true,
		OSTreeTmpDirPath:            "ostree-tmp-dir",
		// TODO: add docker auth
		//DockerAuthConfig: dockerAuth,
	}
	var err error
	if flagRootless && ctx.DockerCertPath == "" {
		ctx.DockerCertPath = "./docker-certs"
	}

	var imagePolicy istore.TrustPolicyContext
	if flagImagePolicy == "" {
		exitError(2, "empty value for --image-policy option")
	} else if flagImagePolicy == "insecure" {
		imagePolicy = istore.TrustPolicyInsecure()
	} else {
		imagePolicy = istore.TrustPolicyFromFile(flagImagePolicy)
	}
	store, err = storepkg.NewStore(flagStoreDir, flagRootless, ctx, imagePolicy, errorLog, debugLog)
	exitOnError(cmd, err)
}

// initConfig reads in config file and ENV variables if set.
/*func initConfig() {
	if flagCfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(flagCfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			exitError(1, "%s", err)
		}

		// Search config in home directory with name ".cntnr" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".cntnr")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}*/
