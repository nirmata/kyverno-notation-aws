package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/cenkalti/backoff/v4"
	_ "github.com/notaryproject/notation-core-go/signature/cose"
	_ "github.com/notaryproject/notation-core-go/signature/jws"
	"github.com/notaryproject/notation-go/dir"
	"go.uber.org/zap"
)

var certFile = "/certs/tls.crt"
var keyFile = "/certs/tls.key"

func main() {
	var flagLocal bool
	flag.BoolVar(&flagLocal, "local", false, "Use local system notation configuration")

	var flagNoTLS bool
	flag.BoolVar(&flagNoTLS, "notls", false, "Do not start the TLS server")

	var flagImagePullSecrets string
	flag.StringVar(&flagImagePullSecrets, "imagePullSecrets", "", "Secret resource names for image registry access credentials.")

	var flagAllowInsecureRegistry bool
	flag.BoolVar(&flagAllowInsecureRegistry, "allowInsecureRegistry", false, "Whether to allow insecure connections to registries. Not recommended.")

	var flagNotationPluginConfigMap string
	flag.StringVar(&flagNotationPluginConfigMap, "pluginConfigMap", "notation-plugin-config", "ConfigMap with notation plugin configuration")

	var flagEnableDebug bool
	flag.BoolVar(&flagEnableDebug, "debug", false, "Enable debug logging")

	var flagMaxSignatureAtempts int
	flag.IntVar(&flagMaxSignatureAtempts, "maxSignatureAttempts", 30, "Maximum number of signature envelopes that will be processed for verification")

	flag.Parse()
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("failed to initialize logger: %v", err)
	}

	slog := logger.Sugar().WithOptions(zap.AddStacktrace(zap.DPanicLevel))
	if !flagLocal {
		if err := installPlugins(); err != nil {
			log.Fatalf("failed to install plugins: %v", err)
		}

		installDir := os.Getenv("NOTATION_DIR")
		dir.UserConfigDir = installDir
		dir.UserLibexecDir = installDir
		slog.Infow("configuring notation", "dir.UserConfigDir", dir.UserConfigDir, "dir.UserLibexecDir", dir.UserLibexecDir)
	}

	var verifier *verifier
	initVerifier := func() error {
		verifier, err = newVerifier(slog,
			withImagePullSecrets(flagImagePullSecrets),
			withInsecureRegistry(flagAllowInsecureRegistry),
			withPluginConfig(flagNotationPluginConfigMap),
			withMaxSignatureAttempts(flagMaxSignatureAtempts),
			withEnableDebug(flagEnableDebug))
		return err
	}

	if err := backoff.Retry(initVerifier, backoff.NewExponentialBackOff()); err != nil {
		slog.Fatalf("initialization error: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/checkimages", verifier.handleCheckImages)
	errsHTTP := make(chan error, 1)
	go func() {
		errsHTTP <- http.ListenAndServe(":9080", mux)
	}()

	errsTLS := make(chan error, 1)
	if !flagNoTLS {
		go func() {
			errsTLS <- http.ListenAndServeTLS(":9443", certFile, keyFile, mux)
		}()
	}

	slog.Info("Listening...")
	select {
	case err := <-errsHTTP:
		slog.Infof("HTTP server error: %v", err)
	case err := <-errsTLS:
		slog.Infof("TLS server error: %v", err)
	}

	verifier.stop()
	os.Exit(-1)
}
