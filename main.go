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
	"time"
)

import "github.com/Unleash/unleash-client-go/v3"

func init() {
	e := unleash.Initialize(
		unleash.WithUrl("https://gitlab.com/api/v4/feature_flags/unleash/53169971"),
		unleash.WithInstanceId("9oBgyhUPSdr5GT2Favwb"),
		unleash.WithAppName("Production"), // Set to the running environment of your application
		unleash.WithDisableMetrics(true),
		unleash.WithRefreshInterval(time.Second*15),
	)

	if e != nil {
		exception := fmt.Errorf("unable to initialize unleash feature-flags: %w", e)

		panic(exception)
	}

	unleash.WaitForReady()
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

// router contains the http endpoints used to serve http request(s).
func router() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		slog.DebugContext(ctx, "Serving HTTP(s) Request", slog.String("url", r.URL.String()))

		flags := map[string]interface{}{
			"user": map[string]interface{}{
				"metadata": unleash.IsEnabled("user-metadata"),
			},
		}

		content, e := json.MarshalIndent(flags, "", "    ")
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

// logger represents a json implementation extending slog.
func logger() *slog.Logger {
	slog.SetLogLoggerLevel(slog.LevelDebug)
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))
}

func main() {
	ctx := context.Background()

	slog.SetDefault(logger())

	api := server(ctx, router(), "8080")
	slog.InfoContext(ctx, "Starting HTTP Server", slog.String("hostname", "localhost"), slog.String("port", "8080"))
	if e := api.ListenAndServe(); e != nil && !(errors.Is(e, http.ErrServerClosed)) {
		slog.ErrorContext(ctx, "Error During Server's Listen & Serve Call ...", slog.String("error", e.Error()))

		os.Exit(100)
	}

	os.Exit(0)
}
