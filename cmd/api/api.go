// Package api implements the api sub-command.
package api

import (
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/oasislabs/oasis-indexer/api"
	"github.com/oasislabs/oasis-indexer/cmd/common"
	"github.com/oasislabs/oasis-indexer/config"
	"github.com/oasislabs/oasis-indexer/log"
	"github.com/oasislabs/oasis-indexer/storage"
	"github.com/oasislabs/oasis-indexer/storage/cockroach"
	"github.com/oasislabs/oasis-indexer/storage/postgres"
)

const (
	moduleName = "api"
)

var (
	// Path to the configuration file.
	configFile string

	apiCmd = &cobra.Command{
		Use:   "serve",
		Short: "Serve Oasis Indexer API",
		Run:   runServer,
	}
)

func runServer(cmd *cobra.Command, args []string) {
	// Initialize config.
	cfg, err := config.InitConfig(configFile)
	if err != nil {
		os.Exit(1)
	}

	// Initialize common environment.
	if err := common.Init(cfg); err != nil {
		os.Exit(1)
	}
	logger := common.Logger()

	if cfg.Server == nil {
		logger.Error("server config not provided")
		os.Exit(1)
	}
	sCfg := cfg.Server

	service, err := NewService(sCfg)
	if err != nil {
		logger.Error("service failed to start",
			"error", err,
		)
		os.Exit(1)
	}

	service.Start()
}

// Service is the Oasis Indexer's API service.
type Service struct {
	server string
	api    *api.IndexerAPI
	logger *log.Logger
}

// NewService creates a new API service.
func NewService(cfg *config.ServerConfig) (*Service, error) {
	logger := common.Logger().WithModule(moduleName)

	// Initialize target storage.
	var backend config.StorageBackend
	if err := backend.Set(cfg.Storage.Backend); err != nil {
		return nil, err
	}

	var client storage.TargetStorage
	var err error
	switch backend {
	case config.BackendCockroach:
		client, err = cockroach.NewClient(cfg.Storage.Endpoint, logger)
	case config.BackendPostgres:
		client, err = postgres.NewClient(cfg.Storage.Endpoint, logger)
	}
	if err != nil {
		return nil, err
	}

	return &Service{
		server: cfg.Endpoint,
		api:    api.NewIndexerAPI(client, logger),
		logger: logger,
	}, nil
}

// Start starts the API service.
func (s *Service) Start() {
	s.logger.Info("starting api service")

	server := &http.Server{
		Addr:           s.server,
		Handler:        s.api.Router(),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	s.logger.Error("shutting down",
		"error", server.ListenAndServe(),
	)
}

// Register registers the process sub-command.
func Register(parentCmd *cobra.Command) {
	apiCmd.Flags().StringVar(&configFile, "config", "./config/local.yml", "path to the config.yml file")
	parentCmd.AddCommand(apiCmd)
}
