// Package runtime wires a module's gRPC service to the agent-wow sockets.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"

	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// Register attaches the module's service to the gRPC server.
type Register func(s grpc.ServiceRegistrar, session modv1.SessionClient)

// Run serves the module until ctx is canceled. Logs go to /logs/<module>.log when mounted.
func Run(ctx context.Context, register Register) error {
	name := os.Getenv("AGENT_WOW_MODULE_NAME")
	setupLog(name)
	moduleSocket := os.Getenv("AGENT_WOW_MODULE_SOCKET")
	sessionSocket := os.Getenv("AGENT_WOW_SESSION_SOCKET")
	if moduleSocket == "" || sessionSocket == "" {
		return fmt.Errorf("AGENT_WOW_MODULE_SOCKET and AGENT_WOW_SESSION_SOCKET must be set by agent-wow")
	}
	listener, err := net.Listen("unix", moduleSocket)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := os.Chmod(moduleSocket, 0666); err != nil {
		return err
	}
	conn, err := grpc.NewClient("unix://"+sessionSocket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDisableRetry(),
		grpc.WithDefaultCallOptions(grpc.MaxRetryRPCBufferSize(0)),
	)
	if err != nil {
		return err
	}
	defer conn.Close()

	server := grpc.NewServer()
	defer server.Stop()
	register(server, modv1.NewSessionClient(conn))
	healthy := health.NewServer()
	grpc_health_v1.RegisterHealthServer(server, healthy)
	healthy.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	stop := context.AfterFunc(ctx, server.Stop)
	defer stop()
	slog.Info("module started", "name", name)
	if err := server.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return err
	}
	return nil
}

func setupLog(name string) {
	var w io.Writer = os.Stderr
	if st, err := os.Stat("/logs"); err == nil && st.IsDir() {
		if f, err := os.OpenFile(filepath.Join("/logs", name+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			w = f
		}
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(w, nil)))
}
