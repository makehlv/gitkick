package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/makehlv/kk/clients"
	"github.com/makehlv/kk/config"
	"github.com/makehlv/kk/repositories"
	"github.com/makehlv/kk/services"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: kk <command> [flags]")
		os.Exit(1)
	}

	clients := clients.NewClients()
	logger := slog.New(NewColorHandler(os.Stderr, slog.LevelInfo))
	config := config.NewConfig()
	repos := repositories.NewRepositories(logger, config)
	svc := services.NewServices(clients, logger, config, repos)

	command := os.Args[1]
	switch command {
	case "squash":
		comparableBranch := parseFlag(os.Args[2:], "--compare")
		if comparableBranch == "" {
			comparableBranch = "develop"
		}
		message := parseFlag(os.Args[2:], "--message")
		if err := svc.CodeFlowManage.Squash(comparableBranch, message); err != nil {
			logger.Error("squash failed", "error", err)
			os.Exit(1)
		}
	case "clean":
		if err := svc.CodeFlowManage.CleanFallbackBranches(); err != nil {
			logger.Error("clean failed", "error", err)
			os.Exit(1)
		}
	case "commit":
		if err := svc.CodeFlowManage.Commit(); err != nil {
			logger.Error("commit failed", "error", err)
			os.Exit(1)
		}
	case "push":
		if err := svc.CodeFlowManage.Push(); err != nil {
			logger.Error("push failed", "error", err)
			os.Exit(1)
		}
	case "swg":
		if len(os.Args) < 3 {
			names, err := svc.Swagger.ListServers()
			if err != nil {
				logger.Error("swg failed", "error", err)
				os.Exit(1)
			}
			for _, name := range names {
				fmt.Println(name)
			}
			break
		}
		serverName := os.Args[2]
		genOp := parseFlag(os.Args[3:], "--gen")
		specPath := parseFlag(os.Args[3:], "--spec")
		if genOp != "" && specPath != "" {
			fmt.Println("use either --gen or --spec, not both")
			os.Exit(1)
		}
		if genOp != "" {
			curlCmd, err := svc.Swagger.BuildCurl(serverName, genOp)
			if err != nil {
				logger.Error("swg failed", "error", err)
				os.Exit(1)
			}
			fmt.Println(curlCmd)
		} else if specPath != "" {
			if err := svc.Swagger.SaveServerSpec(serverName, specPath); err != nil {
				logger.Error("swg failed", "error", err)
				os.Exit(1)
			}
		} else {
			fmt.Println("usage: kk swg [<server_name> --gen <operationId> | --spec <absolute_path_to_swagger>]")
			os.Exit(1)
		}
	case "var":
		if len(os.Args) < 4 {
			fmt.Println("usage: kk var <key> <value>")
			os.Exit(1)
		}
		key := os.Args[2]
		value := strings.Join(os.Args[3:], " ")
		if err := svc.Variable.Add(key, value); err != nil {
			logger.Error("var failed", "error", err)
			os.Exit(1)
		}
	default:
		fmt.Printf("unknown command: %s\n", command)
		os.Exit(1)
	}
}

func parseFlag(args []string, flag string) string {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
