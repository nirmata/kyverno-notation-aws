package main

import (
	"context"
	"crypto/tls"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/awslabs/amazon-ecr-credential-helper/ecr-login"
	"github.com/go-logr/zapr"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/kyverno/kyverno/pkg/leaderelection"
	"github.com/kyverno/pkg/certmanager"
	tlsMgr "github.com/kyverno/pkg/tls"
	"github.com/nirmata/kyverno-notation-verifier/kubenotation"
	knvSetup "github.com/nirmata/kyverno-notation-verifier/setup"
	knvVerifier "github.com/nirmata/kyverno-notation-verifier/verifier"
	_ "github.com/notaryproject/notation-core-go/signature/cose"
	_ "github.com/notaryproject/notation-core-go/signature/jws"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	Namespace      = os.Getenv("POD_NAMESPACE")
	PodName        = os.Getenv("POD_NAME")
	ServiceName    = getEnvWithFallback("SERVICE_NAME", "svc")
	DeploymentName = getEnvWithFallback("DEPLOYMENT_NAME", "kyverno-notation-aws")

	CertRenewalInterval = 12 * time.Hour
	CAValidityDuration  = 365 * 24 * time.Hour
	TLSValidityDuration = 150 * 24 * time.Hour

	resyncPeriod = 15 * time.Minute
)

func main() {
	var (
		flagLocal                   bool
		flagNoTLS                   bool
		flagImagePullSecrets        string
		flagAllowInsecureRegistry   bool
		flagNotationPluginConfigMap string
		flagEnableDebug             bool
		flagMaxSignatureAtempts     int
		metricsAddr                 string
		probeAddr                   string
		enableLeaderElection        bool
		cacheEnabled                bool
		cacheMaxSize                int64
		cacheTTLDuration            int64
		allowedUsers                string
		reviewKyvernoToken          bool
	)

	flag.BoolVar(&flagLocal, "local", false, "Use local system notation configuration")
	flag.BoolVar(&flagNoTLS, "notls", false, "Do not start the TLS server")
	flag.StringVar(&flagImagePullSecrets, "imagePullSecrets", "", "Secret resource names for image registry access credentials.")
	flag.BoolVar(&flagAllowInsecureRegistry, "allowInsecureRegistry", false, "Whether to allow insecure connections to registries. Not recommended.")
	flag.StringVar(&flagNotationPluginConfigMap, "pluginConfigMap", "notation-plugin-config", "ConfigMap with notation plugin configuration")
	flag.BoolVar(&flagEnableDebug, "debug", false, "Enable debug logging")
	flag.IntVar(&flagMaxSignatureAtempts, "maxSignatureAttempts", 30, "Maximum number of signature envelopes that will be processed for verification")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&cacheEnabled, "cacheEnabled", true, "Whether to use a TTL cache for storing verified images, default is true")
	flag.Int64Var(&cacheMaxSize, "cacheMaxSize", 1000, "Max size limit for the TTL cache, default is 1000.")
	flag.Int64Var(&cacheTTLDuration, "cacheTTLDurationSeconds", int64(1*time.Hour), "Max TTL value for a cache in seconds, default is 1 hour.")
	flag.BoolVar(&reviewKyvernoToken, "reviewKyvernoToken", true, "Checks if the Auth token in the request is a token from kyverno controllers or other allowed users, default is true.")
	flag.StringVar(&allowedUsers, "allowedUsers", "system:serviceaccount:kyverno:kyverno-admission-controller,system:serviceaccount:kyverno:kyverno-reports-controller", "Comma-seperated list of all the allowed users and service accounts.")

	flag.Parse()
	zc := zap.NewDevelopmentConfig()
	zc.Level = zap.NewAtomicLevelAt(zapcore.Level(-2))
	logger, err := zc.Build()
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

	signalCtx, sdown := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer sdown()

	tlsMgrConfig := &tlsMgr.Config{
		ServiceName: ServiceName,
		Namespace:   Namespace,
	}

	caStopCh := make(chan struct{}, 1)
	caInformer := NewSecretInformer(kubeClient, Namespace, tlsMgr.GenerateRootCASecretName(tlsMgrConfig), resyncPeriod)
	go caInformer.Informer().Run(caStopCh)

	tlsStopCh := make(chan struct{}, 1)
	tlsInformer := NewSecretInformer(kubeClient, Namespace, tlsMgr.GenerateTLSPairSecretName(tlsMgrConfig), resyncPeriod)
	go tlsInformer.Informer().Run(tlsStopCh)

	le, err := leaderelection.New(
		zapr.NewLogger(logger).WithName("leader-election"),
		DeploymentName,
		Namespace,
		kubeClient,
		PodName,
		2*time.Second,
		func(ctx context.Context) {

			certRenewer := tlsMgr.NewCertRenewer(
				zapr.NewLogger(logger).WithName("tls").WithValues("pod", PodName),
				kubeClient.CoreV1().Secrets(Namespace),
				CertRenewalInterval,
				CAValidityDuration,
				TLSValidityDuration,
				"",
				tlsMgrConfig,
			)

			certManager := certmanager.NewController(
				zapr.NewLogger(logger).WithName("certmanager").WithValues("pod", PodName),
				caInformer,
				tlsInformer,
				certRenewer,
				tlsMgrConfig,
			)

			leaderControllers := []Controller{NewController("cert-manager", certManager, 1)}

			// start leader controllers
			var wg sync.WaitGroup
			for _, controller := range leaderControllers {
				controller.Run(signalCtx, zapr.NewLogger(logger).WithName("controllers"), &wg)
			}
			// wait all controllers shut down
			wg.Wait()
		},
		nil,
	)
	if err != nil {
		log.Fatalf("failed to initialize leader election: %v", err)
		os.Exit(1)
	}

	// start leader election
	go func() {
		select {
		case <-signalCtx.Done():
			return
		default:
			le.Run(signalCtx)
		}
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
		knvVerifier.WithProviderKeychain(authn.NewKeychainFromHelper(ecr.NewECRHelper(ecr.WithLogger(io.Discard)))),
		knvVerifier.WithTokenReviewEnabled(reviewKyvernoToken),
		knvVerifier.WithCacheEnabled(cacheEnabled),
		knvVerifier.WithMaxCacheSize(cacheMaxSize),
		knvVerifier.WithMaxCacheTTL(time.Duration(cacheTTLDuration*int64(time.Second))),
		knvVerifier.WithAllowedUsers(strings.Split(allowedUsers, ",")))

	mux := http.NewServeMux()
	mux.HandleFunc("/checkimages", verifier.HandleCheckImages)
	errsHTTP := make(chan error, 1)
	go func() {
		errsHTTP <- http.ListenAndServe(":9080", mux)
	}()

	errsTLS := make(chan error, 1)
	if !flagNoTLS {
		tlsConf := &tls.Config{
			GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
				secret, err := tlsInformer.Lister().Secrets(tlsMgrConfig.Namespace).Get(tlsMgr.GenerateTLSPairSecretName(tlsMgrConfig))
				if err != nil {
					return nil, err
				} else if secret == nil {
					return nil, errors.New("tls secret not found")
				} else if secret.Type != corev1.SecretTypeTLS {
					return nil, errors.New("secret is not a TLS secret")
				}

				cert, err := tls.X509KeyPair(secret.Data[corev1.TLSCertKey], secret.Data[corev1.TLSPrivateKeyKey])
				if err != nil {
					return nil, err
				}

				return &cert, nil
			},
		}
		srv := &http.Server{
			Addr:              ":9443",
			Handler:           mux,
			TLSConfig:         tlsConf,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			ReadHeaderTimeout: 30 * time.Second,
			IdleTimeout:       1 * time.Minute,
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
