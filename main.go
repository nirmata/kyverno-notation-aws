package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-logr/zapr"
	"github.com/kyverno/pkg/certmanager"
	tlsMgr "github.com/kyverno/pkg/tls"
	"github.com/nirmata/kyverno-notation-verifier/kubenotation"
	knvSetup "github.com/nirmata/kyverno-notation-verifier/setup"
	knvVerifier "github.com/nirmata/kyverno-notation-verifier/verifier"
	_ "github.com/notaryproject/notation-core-go/signature/cose"
	_ "github.com/notaryproject/notation-core-go/signature/jws"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	namespace           = "kyverno-notation-aws"
	CertRenewalInterval = 12 * time.Hour
	CAValidityDuration  = 365 * 24 * time.Hour
	TLSValidityDuration = 150 * 24 * time.Hour
	resyncPeriod        = 15 * time.Minute
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

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("failed to get kubernetes cluster config: %v", err)
	}
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("failed to initialize kube client: %v", err)
	}

	tlsMgrConfig := &tlsMgr.Config{
		ServiceName: "kyverno-notation-aws",
		Namespace:   namespace,
	}

	certRenewer := tlsMgr.NewCertRenewer(
		zapr.NewLogger(logger),
		kubeClient.CoreV1().Secrets(namespace),
		CertRenewalInterval,
		CAValidityDuration,
		TLSValidityDuration,
		"",
		tlsMgrConfig,
	)

	caStopCh := make(chan struct{}, 1)
	caInformer := NewSecretInformer(kubeClient, namespace, tlsMgr.GenerateRootCASecretName(tlsMgrConfig), resyncPeriod)
	go caInformer.Informer().Run(caStopCh)

	tlsStopCh := make(chan struct{}, 1)
	tlsInformer := NewSecretInformer(kubeClient, namespace, tlsMgr.GenerateTLSPairSecretName(tlsMgrConfig), resyncPeriod)
	go tlsInformer.Informer().Run(tlsStopCh)

	certManager := certmanager.NewController(
		zapr.NewLogger(logger),
		caInformer,
		tlsInformer,
		certRenewer,
		tlsMgrConfig,
	)

	go func() {
		certManager.Run(context.TODO(), 1)
	}()

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
		tlsConf := &tls.Config{
			GetCertificate: certManager.GetCertificate,
		}
		srv := &http.Server{
			Addr:      ":9443",
			Handler:   mux,
			TLSConfig: tlsConf,
		}

		go func() {
			errsTLS <- srv.ListenAndServeTLS("", "")
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
			Shutdown(slog, &caStopCh, &tlsStopCh)
			os.Exit(-1)

		case err := <-errsTLS:
			slog.Infof("TLS server error: %v", err)
			verifier.Stop()
			Shutdown(slog, &caStopCh, &tlsStopCh)
			os.Exit(-1)

		case err := <-errsMgr:
			slog.Infof("problem running manager: %v", err)
			verifier.Stop()
			Shutdown(slog, &caStopCh, &tlsStopCh)
			os.Exit(-1)
		}
	}
}

func Shutdown(slog *zap.SugaredLogger, caStopCh *chan struct{}, tlsStopCh *chan struct{}) {
	slog.Sync()
	*caStopCh <- struct{}{}
	*tlsStopCh <- struct{}{}
}
