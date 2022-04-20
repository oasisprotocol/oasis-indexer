// Package api implements the api sub-command.
package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/oasislabs/oasis-block-indexer/go/api"
	"github.com/oasislabs/oasis-block-indexer/go/log"
	"github.com/oasislabs/oasis-block-indexer/go/oasis-indexer/cmd/common"
	target "github.com/oasislabs/oasis-block-indexer/go/storage/cockroach"
)

const (
	// CfgServiceEndpoint is the service endpoint at which the Oasis Indexer API
	// can be reached.
	CfgServiceEndpoint = "api.service_endpoint"

	// CfgStorageEndpoint is the flag for setting the connection string to
	// the backing storage.
	CfgStorageEndpoint = "storage.endpoint"

	moduleName = "api"
)

var (
	cfgServiceEndpoint string
	cfgStorageEndpoint string

	apiCmd = &cobra.Command{
		Use:   "serve",
		Short: "Serve Oasis Indexer API",
		Run:   runServer,
	}
)

func runServer(cmd *cobra.Command, args []string) {
	if err := common.Init(); err != nil {
		os.Exit(1)
	}

	service, err := NewAPIService()
	switch {
	case err == nil:
		service.Start()
	case errors.Is(err, context.Canceled):
		// Shutdown requested during startup.
		return
	default:
		common.Logger().Error("service failed to start",
			"error", err,
		)
		os.Exit(1)
	}
}

// APIService is the Oasis Indexer's API service.
type APIService struct {
	server  string
	handler *api.Handler
	logger  *log.Logger
}

// NewAPIService creates a new API service.
func NewAPIService() (*APIService, error) {
	cockroachClient, err := target.NewCockroachClient(cfgStorageEndpoint)
	if err != nil {
		return nil, err
	}

	return &APIService{
		server:  cfgServiceEndpoint,
		handler: api.NewHandler(cockroachClient),
		logger:  common.Logger().WithModule(moduleName),
	}, nil
}

// Start starts the API service.
func (s *APIService) Start() {
	s.logger.Info("starting api service")

	server := &http.Server{
		Addr:           s.server,
		Handler:        s.handler.Router(),
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
	apiCmd.Flags().StringVar(&cfgStorageEndpoint, CfgStorageEndpoint, "", "a postgresql-compliant connection url")
	apiCmd.Flags().StringVar(&cfgServiceEndpoint, CfgServiceEndpoint, "localhost:8008", "service endpoint from which to serve indexer api")
	parentCmd.AddCommand(apiCmd)
}
