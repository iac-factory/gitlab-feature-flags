package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

import "github.com/Unleash/unleash-client-go/v3"

func init() {
	e := unleash.Initialize(
		unleash.WithUrl("https://gitlab.com/api/v4/feature_flags/unleash/53169971"),
		unleash.WithInstanceId("9oBgyhUPSdr5GT2Favwb"),
		unleash.WithAppName("Production"), // Set to the running environment of your application
		unleash.WithListener(&unleash.DebugListener{}),
		unleash.WithDisableMetrics(true),
		unleash.WithRefreshInterval(time.Second*15),
	)

	if e != nil {
		exception := fmt.Errorf("unable to initialize unleash feature-flags: %w", e)

		panic(exception)
	}

	unleash.WaitForReady()
}

func flags() map[string]interface{} {
	return map[string]interface{}{
		"user": map[string]interface{}{
			"metadata": unleash.IsEnabled("user-metadata"),
		},
	}
}

// server initializes a http.Server with application-specific configuration.
func server(ctx context.Context, handler http.Handler, port string) *http.Server {
	return &http.Server{
		Addr:                         fmt.Sprintf(":%s", port),
		Handler:                      handler,
		DisableGeneralOptionsHandler: false,
		TLSConfig:                    nil,
		ReadTimeout:                  15 * time.Second,
		WriteTimeout:                 60 * time.Second,
		IdleTimeout:                  30 * time.Second,
		MaxHeaderBytes:               http.DefaultMaxHeaderBytes,
		TLSNextProto:                 nil,
		ConnState:                    nil,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
		ConnContext: nil,
	}
}

// interrupt is a graceful interrupt + signal handler for an HTTP server.
func interrupt(ctx context.Context, cancel context.CancelFunc, server *http.Server) {
	// Listen for syscall signals for process to interrupt/quit
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-interrupt

		slog.InfoContext(ctx, "Initializing Server Shutdown ...")

		// Shutdown signal with grace period of 30 seconds
		shutdown, timeout := context.WithTimeout(ctx, 30*time.Second)
		defer timeout()
		go func() {
			<-shutdown.Done()
			if errors.Is(shutdown.Err(), context.DeadlineExceeded) {
				slog.Log(ctx, slog.LevelError, "Graceful Server Shutdown Timeout - Forcing an Exit ...")

				os.Exit(99)
			}
		}()

		// Trigger graceful shutdown
		if e := server.Shutdown(shutdown); e != nil {
			slog.Log(ctx, slog.LevelError, e.Error())
		}

		cancel()
	}()
}

// router contains the http endpoints used to serve http request(s).
func router() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		content, e := json.MarshalIndent(flags(), "", "    ")
		if e != nil {
			code := http.StatusInternalServerError
			message := http.StatusText(code)
			http.Error(w, message, code)

			return
		}

		w.Header().Set("Content-Type", "Application/JSON")
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	})

	return mux
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	slog.SetLogLoggerLevel(slog.LevelDebug)

	api := server(ctx, router(), "3000")

	interrupt(ctx, cancel, api)

	// <-- blocking
	if e := api.ListenAndServe(); e != nil && !(errors.Is(e, http.ErrServerClosed)) {
		slog.ErrorContext(ctx, "Error During Server's Listen & Serve Call ...", slog.String("error", e.Error()))

		os.Exit(100)
	}

	// --> exit
	{
		slog.InfoContext(ctx, "Graceful Shutdown Complete")

		// Waiter
		<-ctx.Done()
	}

	os.Exit(0)
}
