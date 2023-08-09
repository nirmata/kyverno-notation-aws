package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-logr/zapr"
	"github.com/nirmata/kyverno-notation-verifier/kubenotation"
	knvSetup "github.com/nirmata/kyverno-notation-verifier/setup"
	knvTypes "github.com/nirmata/kyverno-notation-verifier/types"
	knvVerifier "github.com/nirmata/kyverno-notation-verifier/verifier"
	_ "github.com/notaryproject/notation-core-go/signature/cose"
	_ "github.com/notaryproject/notation-core-go/signature/jws"
	"go.uber.org/zap"
	ctrl "sigs.k8s.io/controller-runtime"
)

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

	var metricsAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")

	var probeAddr string
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")

	var enableLeaderElection bool
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	var cacheEnabled bool
	flag.BoolVar(&cacheEnabled, "cacheEnabled", true, "Whether to use a TTL cache for storing verified images, default is true")

	var cacheMaxSize int64
	flag.Int64Var(&cacheMaxSize, "cacheMaxSize", 1000, "Max size limit for the TTL cache, default is 1000.")

	var cacheTTLDuration int64
	flag.Int64Var(&cacheTTLDuration, "cacheTTLDurationSeconds", int64(1*time.Hour), "Max TTL value for a cache in seconds, default is 1 hour.")

	flag.Parse()
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("failed to initialize logger: %v", err)
	}

	slog := logger.Sugar().WithOptions(zap.AddStacktrace(zap.DPanicLevel))

	crdSetup, err := kubenotation.Setup(zapr.NewLogger(logger), metricsAddr, probeAddr, enableLeaderElection)
	if err != nil {
		log.Fatalf("failed to initialize crds: %v", err)
	}

	crdManager := *crdSetup.CRDManager
	crdChangeChan := *crdSetup.CRDChangeInformer

	slog.Info("Starting CRD Manager")
	errsMgr := make(chan error, 1)
	go func() {
		errsMgr <- crdManager.Start(ctrl.SetupSignalHandler())
	}()
	slog.Info("CRD Manager Started")

	if !flagLocal {
		knvSetup.SetupLocal(slog)
	}

	verifier := knvVerifier.NewVerifier(slog,
		knvVerifier.WithImagePullSecrets(flagImagePullSecrets),
		knvVerifier.WithInsecureRegistry(flagAllowInsecureRegistry),
		knvVerifier.WithPluginConfig(flagNotationPluginConfigMap),
		knvVerifier.WithMaxSignatureAttempts(flagMaxSignatureAtempts),
		knvVerifier.WithEnableDebug(flagEnableDebug),
		knvVerifier.WithProviderAuthConfigResolver(getAuthFromIRSA),
		knvVerifier.WithCacheEnabled(cacheEnabled),
		knvVerifier.WithMaxCacheSize(cacheMaxSize),
		knvVerifier.WithMaxCacheTTL(time.Duration(cacheTTLDuration*int64(time.Second))))

	mux := http.NewServeMux()
	mux.HandleFunc("/checkimages", verifier.HandleCheckImages)
	errsHTTP := make(chan error, 1)
	go func() {
		errsHTTP <- http.ListenAndServe(":9080", mux)
	}()

	errsTLS := make(chan error, 1)
	if !flagNoTLS {
		go func() {
			errsTLS <- http.ListenAndServeTLS(":9443", knvTypes.CertFile, knvTypes.KeyFile, mux)
		}()
	}

	slog.Info("Listening for requests...")
	for {
		select {
		case crdChanged := <-crdChangeChan:
			slog.Infof("CRD Changed, updating notation verifier %v", crdChanged)
			err := verifier.UpdateNotationVerfier()
			if err != nil {
				slog.Infof("failed to update verifier, reverting update err: %v", err)
			}
			slog.Infof("Notation verifier updated %v", crdChanged)
		case err := <-errsHTTP:
			slog.Infof("HTTP server error: %v", err)
			verifier.Stop()
			os.Exit(-1)

		case err := <-errsTLS:
			slog.Infof("TLS server error: %v", err)
			verifier.Stop()
			os.Exit(-1)

		case err := <-errsMgr:
			slog.Infof("problem running manager: %v", err)
			verifier.Stop()
			os.Exit(-1)
		}
	}
}
